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

	wlock sync.Mutex    // serializes writes to stdin
	sendq chan any      // outgoing messages, drained after initialization
	ready chan struct{} // closed when the initialize handshake finishes

	dead     chan struct{} // closed when the server is unusable
	deadOnce sync.Once
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

// Initialize performs the LSP initialization handshake.
func (s *LspServer) Initialize(dir string, onShow func(lsp.ShowMessageParams), onDiag func(lsp.PublishDiagnosticsParams)) {
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
				Definition:     &lsp.DefinitionTextDocumentClientCapabilities{},
				Implementation: &lsp.ImplementationTextDocumentClientCapabilities{},
				Formatting:     &lsp.DocumentFormattingClientCapabilities{},
			},
		},
	}

	go s.receiveLoop(onShow, onDiag)

	go func() {
		// Release the send queue when the handshake ends (successfully or
		// not) so queued notifications are never stuck forever.
		defer close(s.ready)

		id, ch := s.allocResponse()
		defer s.releaseResponse(id)
		// Write directly: the send loop is still parked on s.ready, so
		// initialize/initialized are guaranteed to precede queued messages.
		if err := s.writeMessage(rpcRequest{JSONRPC: "2.0", ID: id, Method: lsp.MethodInitialize, Params: params}); err != nil {
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
		s.lock.Lock()
		s.capabilities = init.Result.Capabilities
		s.lock.Unlock()

		s.writeMessage(rpcNotification{JSONRPC: "2.0", Method: lsp.MethodInitialized, Params: struct{}{}})
		log.Printf("[lsp] initialized")
	}()
}

// Shutdown sends shutdown + exit to the server. Uses a short timeout so a
// hung server can't stall editor exit.
func (s *LspServer) Shutdown() {
	s.request(lsp.MethodShutdown, nil, 2*time.Second)
	s.notify(lsp.MethodExit, nil)
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
	if s == nil || s.caps().HoverProvider == nil {
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
	if s == nil || s.caps().DefinitionProvider == nil {
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

func (s *LspServer) Format(filename string) ([]lsp.TextEdit, error) {
	if s == nil || s.caps().DocumentFormattingProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.DocumentFormattingParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Options: lsp.FormattingOptions{
			TabSize:      4,
			InsertSpaces: true,
		},
	}
	resp, err := s.request(lsp.MethodTextDocumentFormatting, params, lspRequestTimeout)
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

func (s *LspServer) receiveLoop(onShow func(lsp.ShowMessageParams), onDiag func(lsp.PublishDiagnosticsParams)) {
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
			if onShow != nil {
				var m struct {
					Params lsp.ShowMessageParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				onShow(m.Params)
			}
		case string(lsp.MethodTextDocumentPublishDiagnostics):
			if onDiag != nil {
				var m struct {
					Params lsp.PublishDiagnosticsParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				onDiag(m.Params)
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
		case "workspace/configuration":
			// Answer with one null per requested item so the server isn't
			// left waiting (we have no workspace configuration).
			if header.ID != nil {
				var m struct {
					Params lsp.ConfigurationParams `json:"params"`
				}
				json.Unmarshal(msg, &m)
				s.reply(*header.ID, make([]any, len(m.Params.Items)))
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

// --- Manager ---

// LspManager manages one language server per filetype.
type LspManager struct {
	servers map[string]*LspServer
	langs   map[string]LspLanguage
	onShow  func(lsp.ShowMessageParams)
	onDiag  func(lsp.PublishDiagnosticsParams)
}

func NewLspManager(langs map[string]LspLanguage, onShow func(lsp.ShowMessageParams), onDiag func(lsp.PublishDiagnosticsParams)) *LspManager {
	return &LspManager{
		servers: make(map[string]*LspServer),
		langs:   langs,
		onShow:  onShow,
		onDiag:  onDiag,
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
		wd, _ := os.Getwd()
		s.Initialize(wd, m.onShow, m.onDiag)
		m.servers[sft] = s
	}

	s := m.servers[sft]
	absPath, _ := filepath.Abs(filename)
	s.DidOpen(absPath, sft, contents, version)
	return s, nil
}

// ShutdownAll shuts down all running servers.
func (m *LspManager) ShutdownAll() {
	if m == nil {
		return
	}
	for _, s := range m.servers {
		s.Shutdown()
	}
}
