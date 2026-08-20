package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// --- JSON-RPC types ---

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcReply struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  any    `json:"result"`
}

// --- LspServer ---

// LspServer communicates with a language server subprocess via JSON-RPC 2.0
// over stdin/stdout.
//
// Concurrency model: all outgoing messages are serialized through sendq,
// which a background goroutine drains once the initialize handshake is done
// (the handshake itself writes directly while the queue is still parked, so
// initialize/initialized always precede queued didOpen/didChange). Callers
// therefore never block on the server. The response map and capabilities
// are guarded by lock; raw writes are serialized by wlock.
type LspServer struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	lock         sync.Mutex // guards nextID, responses, capabilities
	nextID       int
	responses    map[int]chan json.RawMessage
	capabilities lsp.ServerCapabilities
	// inlayHintProvider mirrors capabilities.InlayHintProvider, which the
	// vendored protocol package (predating LSP 3.17) has no field for; it is
	// probed from the raw initialize response instead. See lsp_inlayhints.go.
	inlayHintProvider bool

	wlock sync.Mutex    // serializes writes to stdin
	sendq chan any      // outgoing messages, drained after initialization
	ready chan struct{} // closed when the initialize handshake finishes

	dead     chan struct{} // closed when the server is unusable
	deadOnce sync.Once
	exited   chan struct{} // closed when the subprocess has been reaped (nil without one)

	// Set before Initialize; read-only afterwards.
	rootDir  string         // workspace root (for workspace/workspaceFolders)
	settings map[string]any // configuration served to the server
	initOpts map[string]any // initializationOptions for the handshake
}

// lspCallbacks bundles the editor callbacks a server invokes from its
// receive goroutine; the editor marshals them onto the main event loop.
type lspCallbacks struct {
	onShow      func(lsp.ShowMessageParams)
	onDiag      func(lsp.PublishDiagnosticsParams)
	onProgress  func(string) // display-ready $/progress status line
	onApplyEdit func(lspWorkspaceEdit) lsp.ApplyWorkspaceEditResponse
}

var ErrLspNotSupported = errors.New("lsp: operation not supported")

const lspRequestTimeout = 10 * time.Second

// newLspServerIO creates a server around raw pipes (factored out so tests
// can use in-process pipes instead of a subprocess).
func newLspServerIO(stdin io.WriteCloser, stdout io.Reader) *LspServer {
	s := &LspServer{
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		responses: make(map[int]chan json.RawMessage),
		sendq:     make(chan any, 4096),
		ready:     make(chan struct{}),
		dead:      make(chan struct{}),
	}
	go s.sendLoop()
	return s
}

// markDead flags the server as unusable so requests fail fast instead of
// waiting out their timeout.
func (s *LspServer) markDead() {
	s.deadOnce.Do(func() { close(s.dead) })
}

func (s *LspServer) isDead() bool {
	select {
	case <-s.dead:
		return true
	default:
		return false
	}
}

func startLspServer(lang LspLanguage) (*LspServer, error) {
	c := exec.Command(lang.Command, lang.Args...)
	c.Stderr = log.Writer()
	// On Linux, have the kernel kill the server if the editor dies without
	// running its shutdown path (crash, SIGKILL).
	c.SysProcAttr = lspSysProcAttr()

	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, err
	}

	s := newLspServerIO(stdin, stdout)
	s.cmd = c
	s.exited = make(chan struct{})
	go func() {
		// Reap the subprocess as soon as it exits (no zombies while the
		// editor keeps running) and fail pending requests fast.
		c.Wait()
		close(s.exited)
		s.markDead()
	}()
	return s, nil
}

// sendLoop drains the outgoing queue once initialization has finished.
func (s *LspServer) sendLoop() {
	<-s.ready
	for m := range s.sendq {
		if s.isDead() {
			continue // keep draining so enqueuers never block
		}
		if err := s.writeMessage(m); err != nil {
			log.Printf("[lsp] write: %v", err)
			s.markDead()
		}
	}
}

// allocResponse registers a response channel for a new request id.
func (s *LspServer) allocResponse() (int, chan json.RawMessage) {
	s.lock.Lock()
	defer s.lock.Unlock()
	id := s.nextID
	s.nextID++
	ch := make(chan json.RawMessage, 1)
	s.responses[id] = ch
	return id, ch
}

func (s *LspServer) releaseResponse(id int) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.responses, id)
}

// request enqueues a request and waits for its response.
func (s *LspServer) request(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	if s.isDead() {
		return nil, fmt.Errorf("lsp: server unavailable")
	}
	id, ch := s.allocResponse()
	defer s.releaseResponse(id)
	s.sendq <- rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	select {
	case resp := <-ch:
		// A JSON-RPC error reply carries no result member; surface it as
		// an error instead of letting callers decode a zero value and
		// mistake it for success (e.g. a rejected rename reporting
		// "Renamed").
		var errResp struct {
			Error *struct {
				Code    int64  `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(resp, &errResp) == nil && errResp.Error != nil {
			return nil, fmt.Errorf("lsp: %s: %s", method, errResp.Error.Message)
		}
		return resp, nil
	case <-s.dead:
		return nil, fmt.Errorf("lsp: server unavailable")
	case <-time.After(timeout):
		return nil, fmt.Errorf("lsp: %s timed out", method)
	}
}

// notify enqueues a notification without waiting.
func (s *LspServer) notify(method string, params any) {
	if s.isDead() {
		return
	}
	s.sendq <- rpcNotification{JSONRPC: "2.0", Method: method, Params: params}
}

// caps returns the server capabilities (safe against the init goroutine).
func (s *LspServer) caps() lsp.ServerCapabilities {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.capabilities
}

// capEnabled interprets an interface{}-typed server capability field: the
// spec allows a boolean or an options object there, so absent (nil) and
// JSON false both mean unsupported, while true or any options object means
// supported.
func capEnabled(v interface{}) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return true
}

// hasInlayHintProvider reports whether the server advertised inlay-hint
// support (see the inlayHintProvider field comment).
func (s *LspServer) hasInlayHintProvider() bool {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.inlayHintProvider
}

// Initialize performs the LSP initialization handshake.
func (s *LspServer) Initialize(dir string, cb lspCallbacks) {
	s.rootDir = dir
	params := lsp.InitializeParams{
		ProcessID: int32(os.Getpid()),
		RootURI:   uri.File(dir),
		Capabilities: lsp.ClientCapabilities{
			TextDocument: &lsp.TextDocumentClientCapabilities{
				Completion: &lsp.CompletionTextDocumentClientCapabilities{
					CompletionItem: &lsp.CompletionTextDocumentClientCapabilitiesItem{
						SnippetSupport:      false,
						DocumentationFormat: []lsp.MarkupKind{lsp.PlainText},
					},
				},
				Hover: &lsp.HoverTextDocumentClientCapabilities{
					ContentFormat: []lsp.MarkupKind{lsp.PlainText},
				},
				Definition:      &lsp.DefinitionTextDocumentClientCapabilities{},
				Implementation:  &lsp.ImplementationTextDocumentClientCapabilities{},
				Formatting:      &lsp.DocumentFormattingClientCapabilities{},
				RangeFormatting: &lsp.DocumentRangeFormattingClientCapabilities{},
				SignatureHelp:   &lsp.SignatureHelpTextDocumentClientCapabilities{},
				References:      &lsp.ReferencesTextDocumentClientCapabilities{},
				DocumentSymbol: &lsp.DocumentSymbolClientCapabilities{
					HierarchicalDocumentSymbolSupport: true,
				},
				Rename: &lsp.RenameClientCapabilities{
					PrepareSupport: true,
				},
				CallHierarchy: &lsp.CallHierarchyClientCapabilities{},
			},
			Window: &lsp.WindowClientCapabilities{
				WorkDoneProgress: true,
			},
			Workspace: &lsp.WorkspaceClientCapabilities{
				Configuration: true,
				Symbol:        &lsp.WorkspaceSymbolClientCapabilities{},
			},
		},
	}
	// CodeActionLiteralSupport tells modern servers they may return actions
	// carrying edits (rather than only legacy Command values).  Use a map for
	// this small capability fragment because protocol v0.12 predates several
	// of the generated convenience types used by newer protocol packages.
	var initParams map[string]any
	if raw, err := json.Marshal(params); err == nil && json.Unmarshal(raw, &initParams) == nil {
		caps := initParams["capabilities"].(map[string]any)
		textDocument := caps["textDocument"].(map[string]any)
		textDocument["codeAction"] = map[string]any{
			"codeActionLiteralSupport": map[string]any{
				"codeActionKind": map[string]any{
					"valueSet": []string{"", "quickfix", "refactor", "refactor.extract", "refactor.inline", "refactor.rewrite", "source", "source.organizeImports"},
				},
			},
		}
		// InlayHint has no typed capability in the vendored protocol package
		// (it predates LSP 3.17); advertise minimal support via raw JSON.
		textDocument["inlayHint"] = map[string]any{}
		workspace := caps["workspace"].(map[string]any)
		workspace["applyEdit"] = true
		workspace["workspaceEdit"] = map[string]any{"documentChanges": true}
	}
	if s.initOpts != nil {
		initParams["initializationOptions"] = s.initOpts
	}

	go s.receiveLoop(cb)

	go func() {
		// Release the send queue when the handshake ends (successfully or
		// not) so queued notifications are never stuck forever.
		defer close(s.ready)

		id, ch := s.allocResponse()
		defer s.releaseResponse(id)
		// Write directly: the send loop is still parked on s.ready, so
		// initialize/initialized are guaranteed to precede queued messages.
		if err := s.writeMessage(rpcRequest{JSONRPC: "2.0", ID: id, Method: lsp.MethodInitialize, Params: initParams}); err != nil {
			log.Printf("[lsp] init error: %v", err)
			return
		}
		var resp json.RawMessage
		select {
		case resp = <-ch:
		case <-s.dead:
			log.Printf("[lsp] server died during initialization")
			return
		case <-time.After(30 * time.Second):
			log.Printf("[lsp] initialize timed out")
			return
		}

		var init struct {
			Result lsp.InitializeResult `json:"result"`
		}
		json.Unmarshal(resp, &init)
		// Also probe for inlayHintProvider, which lsp.ServerCapabilities has
		// no field for (see the inlayHintProvider field comment); any
		// non-false value (bool true or an options object) counts as support.
		var inlayProbe struct {
			Result struct {
				Capabilities struct {
					InlayHintProvider json.RawMessage `json:"inlayHintProvider"`
				} `json:"capabilities"`
			} `json:"result"`
		}
		json.Unmarshal(resp, &inlayProbe)
		rawInlay := strings.TrimSpace(string(inlayProbe.Result.Capabilities.InlayHintProvider))

		s.lock.Lock()
		s.capabilities = init.Result.Capabilities
		s.inlayHintProvider = rawInlay != "" && rawInlay != "false" && rawInlay != "null"
		s.lock.Unlock()

		s.writeMessage(rpcNotification{JSONRPC: "2.0", Method: lsp.MethodInitialized, Params: struct{}{}})
		// Push the configured settings (still before the queue is released,
		// so they precede any didOpen).
		if s.settings != nil {
			s.writeMessage(rpcNotification{JSONRPC: "2.0", Method: "workspace/didChangeConfiguration",
				Params: lsp.DidChangeConfigurationParams{Settings: s.settings}})
		}
		log.Printf("[lsp] initialized")
	}()
}

// How long Shutdown waits for the server to exit on its own after the exit
// notification, then again after closing its stdin, before killing it.
const (
	lspExitGrace = 100 * time.Millisecond
	lspEOFGrace  = 100 * time.Millisecond
)

// Shutdown asks the server to exit (shutdown request + exit notification)
// and escalates if it doesn't comply: close stdin (the EOF cue), then kill
// the process. A hung server must not survive the editor, and every step is
// bounded so it can't stall editor exit for more than a few seconds either.
func (s *LspServer) Shutdown() {
	if !s.isDead() {
		// The polite handshake is only worth attempting once
		// initialization has finished; before that the send queue has
		// never started, so the request would just burn its timeout.
		select {
		case <-s.ready:
			s.request(lsp.MethodShutdown, nil, 2*time.Second)
			s.notify(lsp.MethodExit, nil)
		default:
		}
	}

	if s.cmd == nil || s.cmd.Process == nil {
		return // in-process server (tests): nothing to reap
	}
	select {
	case <-s.exited:
		return
	case <-time.After(lspExitGrace):
	}
	// The exit notification was ignored (or never delivered): closing
	// stdin gives servers that watch for EOF a second cue, and unblocks a
	// send loop stuck writing to a full pipe.
	s.stdin.Close()
	select {
	case <-s.exited:
		return
	case <-time.After(lspEOFGrace):
	}
	log.Printf("[lsp] server unresponsive on shutdown; killing pid %d", s.cmd.Process.Pid)
	s.cmd.Process.Kill()
	select {
	case <-s.exited:
	case <-time.After(2 * time.Second):
	}
}

// --- Notifications (client → server) ---

func (s *LspServer) DidOpen(filename, language, text string, version int32) {
	if s == nil {
		return
	}
	params := lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:        uri.File(filename),
			LanguageID: lsp.LanguageIdentifier(language),
			Version:    version,
			Text:       text,
		},
	}
	s.notify(lsp.MethodTextDocumentDidOpen, params)
}

func (s *LspServer) DidChange(filename string, version int32, changes []lsp.TextDocumentContentChangeEvent) {
	if s == nil {
		return
	}
	params := lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Version:                version,
		},
		ContentChanges: changes,
	}
	s.notify(lsp.MethodTextDocumentDidChange, params)
}

func (s *LspServer) DidSave(filename string) {
	if s == nil {
		return
	}
	params := lsp.DidSaveTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
	}
	s.notify(lsp.MethodTextDocumentDidSave, params)
}

func (s *LspServer) DidClose(filename string) {
	if s == nil {
		return
	}
	params := lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
	}
	s.notify(lsp.MethodTextDocumentDidClose, params)
}

// --- Requests (client → server → response) ---

func (s *LspServer) Completion(filename string, pos lsp.Position) ([]lsp.CompletionItem, error) {
	if s == nil || s.caps().CompletionProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
		Context: &lsp.CompletionContext{TriggerKind: lsp.CompletionTriggerKindInvoked},
	}
	resp, err := s.request(lsp.MethodTextDocumentCompletion, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	// Server may return CompletionList or []CompletionItem.
	var list struct {
		Result lsp.CompletionList `json:"result"`
	}
	if err := json.Unmarshal(resp, &list); err == nil && len(list.Result.Items) > 0 {
		return list.Result.Items, nil
	}
	var items struct {
		Result []lsp.CompletionItem `json:"result"`
	}
	if err := json.Unmarshal(resp, &items); err != nil {
		return nil, err
	}
	return items.Result, nil
}

func (s *LspServer) Hover(filename string, pos lsp.Position) (string, error) {
	if s == nil || !capEnabled(s.caps().HoverProvider) {
		return "", ErrLspNotSupported
	}
	params := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Position:     pos,
	}
	resp, err := s.request(lsp.MethodTextDocumentHover, params, lspRequestTimeout)
	if err != nil {
		return "", err
	}
	var hover struct {
		Result lsp.Hover `json:"result"`
	}
	if err := json.Unmarshal(resp, &hover); err == nil && hover.Result.Contents.Value != "" {
		return hover.Result.Contents.Value, nil
	}
	// Alternate format: contents as array.
	var alt struct {
		Result struct {
			Contents []any `json:"contents"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &alt); err == nil {
		for _, c := range alt.Result.Contents {
			switch v := c.(type) {
			case string:
				return v, nil
			case map[string]any:
				if s, ok := v["value"].(string); ok {
					return s, nil
				}
			}
		}
	}
	return "", nil
}

func (s *LspServer) Definition(filename string, pos lsp.Position) ([]lsp.Location, error) {
	if s == nil || !capEnabled(s.caps().DefinitionProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.DefinitionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
	}
	resp, err := s.request(lsp.MethodTextDocumentDefinition, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var locs struct {
		Result []lsp.Location `json:"result"`
	}
	if err := json.Unmarshal(resp, &locs); err != nil {
		return nil, err
	}
	return locs.Result, nil
}

func (s *LspServer) Format(filename string, opts lsp.FormattingOptions, timeout time.Duration) ([]lsp.TextEdit, error) {
	if s == nil || !capEnabled(s.caps().DocumentFormattingProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.DocumentFormattingParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Options:      opts,
	}
	resp, err := s.request(lsp.MethodTextDocumentFormatting, params, timeout)
	if err != nil {
		return nil, err
	}
	var edits struct {
		Result []lsp.TextEdit `json:"result"`
	}
	if err := json.Unmarshal(resp, &edits); err != nil {
		return nil, err
	}
	return edits.Result, nil
}

// RangeFormatting requests formatting for just the given range, for the =
// operator (see registerLspFormatOperator).
func (s *LspServer) RangeFormatting(filename string, rng lsp.Range, opts lsp.FormattingOptions) ([]lsp.TextEdit, error) {
	if s == nil || !capEnabled(s.caps().DocumentRangeFormattingProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.DocumentRangeFormattingParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Range:        rng,
		Options:      opts,
	}
	resp, err := s.request(lsp.MethodTextDocumentRangeFormatting, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var edits struct {
		Result []lsp.TextEdit `json:"result"`
	}
	if err := json.Unmarshal(resp, &edits); err != nil {
		return nil, err
	}
	return edits.Result, nil
}

func (s *LspServer) References(filename string, pos lsp.Position) ([]lsp.Location, error) {
	if s == nil || !capEnabled(s.caps().ReferencesProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.ReferenceParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
		Context: lsp.ReferenceContext{IncludeDeclaration: true},
	}
	resp, err := s.request(lsp.MethodTextDocumentReferences, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var locs struct {
		Result []lsp.Location `json:"result"`
	}
	if err := json.Unmarshal(resp, &locs); err != nil {
		return nil, err
	}
	return locs.Result, nil
}

// DocumentSymbols requests the symbol outline of filename. Servers may
// answer with the hierarchical DocumentSymbol form or the flat legacy
// SymbolInformation form; both are normalized to DocumentSymbol, using each
// SymbolInformation's location as both range and selection range.
func (s *LspServer) DocumentSymbols(filename string) ([]lsp.DocumentSymbol, error) {
	if s == nil || !capEnabled(s.caps().DocumentSymbolProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
	}
	resp, err := s.request(lsp.MethodTextDocumentDocumentSymbol, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Result []json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, err
	}
	if len(raw.Result) == 0 {
		return nil, nil
	}
	// Distinguish the two shapes by a field only one of them has.
	var probe struct {
		Location *lsp.Location `json:"location"`
	}
	json.Unmarshal(raw.Result[0], &probe)
	out := make([]lsp.DocumentSymbol, len(raw.Result))
	if probe.Location != nil {
		for i, r := range raw.Result {
			var si lsp.SymbolInformation
			if err := json.Unmarshal(r, &si); err != nil {
				return nil, err
			}
			out[i] = lsp.DocumentSymbol{
				Name:           si.Name,
				Kind:           si.Kind,
				Range:          si.Location.Range,
				SelectionRange: si.Location.Range,
			}
		}
		return out, nil
	}
	for i, r := range raw.Result {
		if err := json.Unmarshal(r, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WorkspaceSymbols requests symbols matching query across every file in the
// workspace, not just the currently open document.
func (s *LspServer) WorkspaceSymbols(query string) ([]lsp.SymbolInformation, error) {
	if s == nil || !capEnabled(s.caps().WorkspaceSymbolProvider) {
		return nil, ErrLspNotSupported
	}
	params := lsp.WorkspaceSymbolParams{Query: query}
	resp, err := s.request(lsp.MethodWorkspaceSymbol, params, lspRequestTimeout)
	if err != nil {
		return nil, err
	}
	var syms struct {
		Result []lsp.SymbolInformation `json:"result"`
	}
	if err := json.Unmarshal(resp, &syms); err != nil {
		return nil, err
	}
	return syms.Result, nil
}

// --- JSON-RPC transport ---

func (s *LspServer) writeMessage(m any) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	body = append(body, '\r', '\n')
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	s.wlock.Lock()
	defer s.wlock.Unlock()
	_, err = s.stdin.Write(append([]byte(header), body...))
	return err
}

// reply sends a response to a server → client request.
func (s *LspServer) reply(id int, result any) {
	if err := s.writeMessage(rpcReply{JSONRPC: "2.0", ID: id, Result: result}); err != nil {
		log.Printf("[lsp] reply: %v", err)
	}
}

func (s *LspServer) readMessage() (json.RawMessage, error) {
	n := -1
	for {
		line, err := s.stdout.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		h := strings.TrimSpace(string(line))
		if h == "" {
			break
		}
		if strings.HasPrefix(h, "Content-Length:") {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) == 2 {
				n, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		}
	}
	if n <= 0 {
		return json.RawMessage{}, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.stdout, buf); err != nil {
		return nil, err
	}
	return json.RawMessage(buf), nil
}

func (s *LspServer) receiveLoop(cb lspCallbacks) {
	// Progress tokens → titles, from window/workDoneProgress/create begin
	// events. Only this goroutine touches it.
	progress := make(map[string]string)

	for {
		msg, err := s.readMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("[lsp] receive error: %v", err)
			}
			s.markDead()
			return
		}
		if len(msg) == 0 {
			continue
		}

		var header struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg, &header); err != nil {
			continue
		}

		switch header.Method {
		case string(lsp.MethodWindowShowMessage):
			if cb.onShow != nil {
				var m struct {
					Params lsp.ShowMessageParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				cb.onShow(m.Params)
			}
		case string(lsp.MethodWindowLogMessage):
			// Server debug output: route to the log file, not the UI.
			var m struct {
				Params lsp.LogMessageParams `json:"params"`
			}
			json.Unmarshal(msg, &m)
			log.Printf("[lsp] server: %s", m.Params.Message)
		case string(lsp.MethodTextDocumentPublishDiagnostics):
			if cb.onDiag != nil {
				var m struct {
					Params lsp.PublishDiagnosticsParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				cb.onDiag(m.Params)
			}
		case "$/progress":
			if status, ok := decodeProgress(msg, progress); ok && cb.onProgress != nil {
				cb.onProgress(status)
			}
		case "window/workDoneProgress/create":
			// Void result; the $/progress notifications follow.
			if header.ID != nil {
				s.reply(*header.ID, nil)
			}
		case "client/registerCapability", "client/unregisterCapability":
			// Void result. We do not advertise dynamic registration, so
			// servers that register anyway get an acknowledgement and the
			// static capabilities keep applying.
			if header.ID != nil {
				s.reply(*header.ID, nil)
			}
		case "workspace/applyEdit":
			if header.ID != nil {
				var m struct {
					Params struct {
						Edit lspWorkspaceEdit `json:"edit"`
					} `json:"params"`
				}
				json.Unmarshal(msg, &m)
				if cb.onApplyEdit == nil {
					s.reply(*header.ID, lsp.ApplyWorkspaceEditResponse{Applied: false, FailureReason: "client cannot apply workspace edits"})
				} else {
					// Reply from a goroutine: onApplyEdit blocks on the
					// main loop, and blocking the receive loop here
					// deadlocks against any synchronous request the main
					// loop is waiting on (format-on-save). Out-of-order
					// replies are legal in JSON-RPC.
					id := *header.ID
					edit := m.Params.Edit
					go func() {
						s.reply(id, cb.onApplyEdit(edit))
					}()
				}
			}
		case "workspace/workspaceFolders":
			if header.ID != nil {
				s.reply(*header.ID, []lsp.WorkspaceFolder{{
					URI:  string(uri.File(s.rootDir)),
					Name: filepath.Base(s.rootDir),
				}})
			}
		case "workspace/configuration":
			// Answer each requested item from the configured settings
			// (null for sections we have nothing for).
			if header.ID != nil {
				var m struct {
					Params lsp.ConfigurationParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				out := make([]any, len(m.Params.Items))
				for i, item := range m.Params.Items {
					out[i] = lookupSettings(s.settings, item.Section)
				}
				s.reply(*header.ID, out)
			}
		case "":
			// Response to a request.
			if header.ID == nil {
				continue
			}
			s.lock.Lock()
			ch, ok := s.responses[*header.ID]
			s.lock.Unlock()
			if ok {
				ch <- msg
			}
		default:
			if header.ID != nil {
				// Unknown server → client request: reply with a null
				// result so the server doesn't block on us.
				s.reply(*header.ID, nil)
			} else {
				log.Printf("[lsp] unhandled: %s", header.Method)
			}
		}
	}
}

// decodeProgress turns a $/progress notification into a display string,
// tracking begin-event titles per token in titles.
func decodeProgress(msg json.RawMessage, titles map[string]string) (string, bool) {
	var m struct {
		Params struct {
			Token json.RawMessage `json:"token"`
			Value struct {
				Kind       string   `json:"kind"`
				Title      string   `json:"title"`
				Message    string   `json:"message"`
				Percentage *float64 `json:"percentage"`
			} `json:"value"`
		} `json:"params"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		return "", false
	}
	tok := string(m.Params.Token)
	v := m.Params.Value

	var title string
	switch v.Kind {
	case "begin":
		titles[tok] = v.Title
		title = v.Title
	case "report":
		title = titles[tok]
	case "end":
		title = titles[tok]
		delete(titles, tok)
	default:
		return "", false
	}
	if title == "" {
		title = "lsp"
	}

	status := v.Message
	if v.Kind == "end" && status == "" {
		status = "done"
	}
	out := title
	if status != "" {
		out += ": " + status
	}
	if v.Percentage != nil && v.Kind != "end" {
		out += fmt.Sprintf(" (%d%%)", int(*v.Percentage))
	}
	return out, true
}

// lookupSettings resolves a workspace/configuration section (dotted path)
// against the configured settings map; nil when absent.
func lookupSettings(settings map[string]any, section string) any {
	if settings == nil {
		return nil
	}
	if section == "" {
		return settings
	}
	var cur any = settings
	for _, part := range strings.Split(section, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[part]
		if !ok {
			return nil
		}
	}
	return cur
}

// --- Manager ---

// LspManager manages one language server per filetype.
type LspManager struct {
	servers map[string]*LspServer
	langs   map[string]LspLanguage
	cb      lspCallbacks
}

func NewLspManager(langs map[string]LspLanguage, cb lspCallbacks) *LspManager {
	return &LspManager{
		servers: make(map[string]*LspServer),
		langs:   langs,
		cb:      cb,
	}
}

// Open starts the language server for the given filetype (if configured and
// not already running), sends didOpen, and returns the server.
func (m *LspManager) Open(ft, filename, contents string, version int32) (*LspServer, error) {
	if m == nil {
		return nil, nil
	}
	lang, ok := m.langs[ft]
	if !ok {
		return nil, nil // no LSP configured for this filetype
	}
	sft := lang.Ft
	if sft == "" {
		sft = ft
	}

	if _, ok := m.servers[sft]; !ok {
		s, err := startLspServer(lang)
		if err != nil {
			return nil, err
		}
		s.settings = lang.Settings
		s.initOpts = lang.InitOptions
		wd, _ := os.Getwd()
		s.Initialize(wd, m.cb)
		m.servers[sft] = s
	}

	s := m.servers[sft]
	absPath, _ := filepath.Abs(filename)
	s.DidOpen(absPath, sft, contents, version)
	return s, nil
}

// ShutdownUnused stops servers that no longer serve any buffer (e.g. after
// :set lsp false) so no server process lingers; a later attach starts a
// fresh one. Shutdown runs in the background since it can block briefly.
func (m *LspManager) ShutdownUnused(used map[*LspServer]bool) {
	if m == nil {
		return
	}
	for ft, s := range m.servers {
		if !used[s] {
			delete(m.servers, ft)
			go s.Shutdown()
		}
	}
}

// ShutdownAll shuts down all running servers in parallel, so several hung
// servers can't stack their shutdown timeouts.
func (m *LspManager) ShutdownAll() {
	if m == nil {
		return
	}
	var wg sync.WaitGroup
	for _, s := range m.servers {
		wg.Add(1)
		go func(s *LspServer) {
			defer wg.Done()
			s.Shutdown()
		}(s)
	}
	wg.Wait()
}
