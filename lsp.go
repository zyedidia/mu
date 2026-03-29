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

type rpcResult struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method,omitempty"`
}

// --- LspServer ---

// LspServer communicates with a language server subprocess via JSON-RPC 2.0
// over stdin/stdout.
type LspServer struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	capabilities lsp.ServerCapabilities
	lock         sync.Mutex
	nextID       int
	responses    map[int]chan json.RawMessage
}

var ErrLspNotSupported = errors.New("lsp: operation not supported")

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

	return &LspServer{
		cmd:       c,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		responses: make(map[int]chan json.RawMessage),
	}, nil
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

	s.lock.Lock()
	go func() {
		defer s.lock.Unlock()

		resp, err := s.sendRequestLocked(lsp.MethodInitialize, params)
		if err != nil {
			log.Printf("[lsp] init error: %v", err)
			return
		}

		var init struct {
			Result lsp.InitializeResult `json:"result"`
		}
		json.Unmarshal(resp, &init)
		s.capabilities = init.Result.Capabilities

		s.sendNotificationLocked(lsp.MethodInitialized, struct{}{})
		log.Printf("[lsp] initialized")
	}()
}

// Shutdown sends shutdown + exit to the server.
func (s *LspServer) Shutdown() {
	s.lock.Lock()
	s.sendRequestLocked(lsp.MethodShutdown, nil)
	s.sendNotificationLocked(lsp.MethodExit, nil)
	s.lock.Unlock()
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
	s.lock.Lock()
	go s.sendNotificationUnlock(lsp.MethodTextDocumentDidOpen, params)
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
	s.lock.Lock()
	go s.sendNotificationUnlock(lsp.MethodTextDocumentDidChange, params)
}

func (s *LspServer) DidSave(filename string) {
	if s == nil {
		return
	}
	params := lsp.DidSaveTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
	}
	s.lock.Lock()
	go s.sendNotificationUnlock(lsp.MethodTextDocumentDidSave, params)
}

func (s *LspServer) DidClose(filename string) {
	if s == nil {
		return
	}
	params := lsp.DidCloseTextDocumentParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
	}
	s.lock.Lock()
	go s.sendNotificationUnlock(lsp.MethodTextDocumentDidClose, params)
}

// --- Requests (client → server → response) ---

func (s *LspServer) Completion(filename string, pos lsp.Position) ([]lsp.CompletionItem, error) {
	if s == nil || s.capabilities.CompletionProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
		Context: &lsp.CompletionContext{TriggerKind: lsp.CompletionTriggerKindInvoked},
	}
	s.lock.Lock()
	resp, err := s.sendRequestUnlock(lsp.MethodTextDocumentCompletion, params)
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
	if s == nil || s.capabilities.HoverProvider == nil {
		return "", ErrLspNotSupported
	}
	params := lsp.TextDocumentPositionParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Position:     pos,
	}
	s.lock.Lock()
	resp, err := s.sendRequestUnlock(lsp.MethodTextDocumentHover, params)
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
	if s == nil || s.capabilities.DefinitionProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.DefinitionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
			Position:     pos,
		},
	}
	s.lock.Lock()
	resp, err := s.sendRequestUnlock(lsp.MethodTextDocumentDefinition, params)
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
	if s == nil || s.capabilities.DocumentFormattingProvider == nil {
		return nil, ErrLspNotSupported
	}
	params := lsp.DocumentFormattingParams{
		TextDocument: lsp.TextDocumentIdentifier{URI: uri.File(filename)},
		Options: lsp.FormattingOptions{
			TabSize:      4,
			InsertSpaces: true,
		},
	}
	s.lock.Lock()
	resp, err := s.sendRequestUnlock(lsp.MethodTextDocumentFormatting, params)
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

func (s *LspServer) sendRequestLocked(method string, params any) (json.RawMessage, error) {
	id := s.nextID
	s.nextID++
	ch := make(chan json.RawMessage, 1)
	s.responses[id] = ch

	if err := s.writeMessage(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		delete(s.responses, id)
		return nil, err
	}

	var resp json.RawMessage
	var err error
	select {
	case resp = <-ch:
	case <-time.After(10 * time.Second):
		err = fmt.Errorf("lsp: %s timed out", method)
	}
	delete(s.responses, id)
	return resp, err
}

func (s *LspServer) sendNotificationLocked(method string, params any) {
	s.writeMessage(rpcNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// sendRequestUnlock sends a request and unlocks the mutex (for use with
// Lock() before the call).
func (s *LspServer) sendRequestUnlock(method string, params any) (json.RawMessage, error) {
	defer s.lock.Unlock()
	return s.sendRequestLocked(method, params)
}

// sendNotificationUnlock sends a notification and unlocks the mutex.
func (s *LspServer) sendNotificationUnlock(method string, params any) {
	defer s.lock.Unlock()
	s.sendNotificationLocked(method, params)
}

func (s *LspServer) writeMessage(m any) error {
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	body = append(body, '\r', '\n')
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	_, err = s.stdin.Write(append([]byte(header), body...))
	return err
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
			return
		}
		if len(msg) == 0 {
			continue
		}

		var header rpcResult
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
			if ch, ok := s.responses[header.ID]; ok {
				ch <- msg
			}
		default:
			log.Printf("[lsp] unhandled: %s", header.Method)
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
