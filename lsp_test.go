package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	lsp "go.lsp.dev/protocol"
)

// fakeLspServer speaks just enough JSON-RPC to test the client transport.
type fakeLspServer struct {
	in  *bufio.Reader // client → server
	out io.Writer     // server → client

	initDelay   time.Duration
	hoverDelay  time.Duration
	complDelay  time.Duration
	symbolsFlat bool // reply to documentSymbol with the legacy SymbolInformation shape

	mu         sync.Mutex
	received   []string                   // method order as received
	replies    map[int]json.RawMessage    // client replies to server requests, by id
	lastParams map[string]json.RawMessage // last params received per method
}

// paramsFor returns the params of the most recent request for method, if
// one has arrived.
func (f *fakeLspServer) paramsFor(method string) (json.RawMessage, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.lastParams[method]
	return p, ok
}

func (f *fakeLspServer) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.received))
	copy(out, f.received)
	return out
}

// reply returns the client's reply to the server-initiated request id, if
// one has arrived.
func (f *fakeLspServer) reply(id int) (json.RawMessage, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.replies[id]
	return r, ok
}

// waitReply polls for the client's reply to request id.
func (f *fakeLspServer) waitReply(t *testing.T, id int) json.RawMessage {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := f.reply(id); ok {
			return r
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no reply to server request %d", id)
	return nil
}

func (f *fakeLspServer) write(m any) {
	body, _ := json.Marshal(m)
	fmt.Fprintf(f.out, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (f *fakeLspServer) run() {
	for {
		// Read one framed message.
		n := -1
		for {
			line, err := f.in.ReadBytes('\n')
			if err != nil {
				return
			}
			h := strings.TrimSpace(string(line))
			if h == "" {
				break
			}
			if strings.HasPrefix(h, "Content-Length:") {
				n, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(h, "Content-Length:")))
			}
		}
		if n <= 0 {
			continue
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(f.in, buf); err != nil {
			return
		}

		var msg struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		json.Unmarshal(buf, &msg)
		if msg.Method == "" && msg.ID != nil {
			// A client reply to one of our server → client requests.
			f.mu.Lock()
			if f.replies == nil {
				f.replies = make(map[int]json.RawMessage)
			}
			f.replies[*msg.ID] = json.RawMessage(buf)
			f.mu.Unlock()
			continue
		}
		var withParams struct {
			Params json.RawMessage `json:"params"`
		}
		json.Unmarshal(buf, &withParams)

		f.mu.Lock()
		f.received = append(f.received, msg.Method)
		if withParams.Params != nil {
			if f.lastParams == nil {
				f.lastParams = make(map[string]json.RawMessage)
			}
			f.lastParams[msg.Method] = withParams.Params
		}
		f.mu.Unlock()

		switch msg.Method {
		case "initialize":
			time.Sleep(f.initDelay)
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": map[string]any{
					"capabilities": map[string]any{
						"hoverProvider":                   true,
						"completionProvider":              map[string]any{},
						"codeActionProvider":              true,
						"signatureHelpProvider":           map[string]any{},
						"referencesProvider":              true,
						"documentSymbolProvider":          true,
						"renameProvider":                  true,
						"workspaceSymbolProvider":         true,
						"callHierarchyProvider":           true,
						"inlayHintProvider":               true,
						"documentFormattingProvider":      true,
						"documentRangeFormattingProvider": true,
					},
				},
			})
		case "textDocument/hover":
			time.Sleep(f.hoverDelay)
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  map[string]any{"contents": map[string]any{"kind": "plaintext", "value": "hi"}},
			})
		case "textDocument/completion":
			time.Sleep(f.complDelay)
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  []map[string]any{{"label": "foobar"}},
			})
		case "textDocument/formatting":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 3},
					},
					"newText": "fmt",
				}},
			})
		case "textDocument/codeAction":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"title": "Make greeting",
					"kind":  "quickfix",
					"edit": map[string]any{"changes": map[string]any{
						"file:///tmp/x.go": []map[string]any{{
							"range": map[string]any{
								"start": map[string]any{"line": 0, "character": 0},
								"end":   map[string]any{"line": 0, "character": 2},
							},
							"newText": "hello",
						}},
					}},
				}},
			})
		case "textDocument/signatureHelp":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": map[string]any{
					"signatures": []map[string]any{{
						"label":      "foo(a, b int)",
						"parameters": []map[string]any{{"label": "a"}, {"label": "b int"}},
					}},
					"activeParameter": 1,
				},
			})
		case "textDocument/references":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"uri": "file:///tmp/x.go",
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 2},
					},
				}},
			})
		case "textDocument/documentSymbol":
			var result []map[string]any
			if f.symbolsFlat {
				result = []map[string]any{{
					"name": "bar",
					"kind": 12, // Function
					"location": map[string]any{
						"uri": "file:///tmp/x.go",
						"range": map[string]any{
							"start": map[string]any{"line": 1, "character": 5},
							"end":   map[string]any{"line": 1, "character": 8},
						},
					},
				}}
			} else {
				result = []map[string]any{{
					"name": "foo",
					"kind": 12, // Function
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 2, "character": 1},
					},
					"selectionRange": map[string]any{
						"start": map[string]any{"line": 0, "character": 5},
						"end":   map[string]any{"line": 0, "character": 8},
					},
				}}
			}
			f.write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": result})
		case "textDocument/rename":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": map[string]any{"changes": map[string]any{
					"file:///tmp/x.go": []map[string]any{{
						"range": map[string]any{
							"start": map[string]any{"line": 0, "character": 0},
							"end":   map[string]any{"line": 0, "character": 2},
						},
						"newText": "renamed",
					}},
				}},
			})
		case "workspace/symbol":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"name": "fooSymbol",
					"kind": 12, // Function
					"location": map[string]any{
						"uri": "file:///tmp/x.go",
						"range": map[string]any{
							"start": map[string]any{"line": 0, "character": 0},
							"end":   map[string]any{"line": 0, "character": 3},
						},
					},
				}},
			})
		case "textDocument/prepareCallHierarchy":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"name": "target",
					"kind": 12, // Function
					"uri":  "file:///tmp/x.go",
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 6},
					},
					"selectionRange": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 6},
					},
				}},
			})
		case "callHierarchy/incomingCalls":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"from": map[string]any{
						"name": "caller",
						"kind": 12,
						"uri":  "file:///tmp/x.go",
						"range": map[string]any{
							"start": map[string]any{"line": 1, "character": 0},
							"end":   map[string]any{"line": 1, "character": 6},
						},
						"selectionRange": map[string]any{
							"start": map[string]any{"line": 1, "character": 0},
							"end":   map[string]any{"line": 1, "character": 6},
						},
					},
					"fromRanges": []map[string]any{{
						"start": map[string]any{"line": 1, "character": 1},
						"end":   map[string]any{"line": 1, "character": 7},
					}},
				}},
			})
		case "callHierarchy/outgoingCalls":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"to": map[string]any{
						"name": "callee",
						"kind": 12,
						"uri":  "file:///tmp/x.go",
						"range": map[string]any{
							"start": map[string]any{"line": 2, "character": 0},
							"end":   map[string]any{"line": 2, "character": 6},
						},
						"selectionRange": map[string]any{
							"start": map[string]any{"line": 2, "character": 0},
							"end":   map[string]any{"line": 2, "character": 6},
						},
					},
					"fromRanges": []map[string]any{{
						"start": map[string]any{"line": 0, "character": 1},
						"end":   map[string]any{"line": 0, "character": 7},
					}},
				}},
			})
		case "textDocument/rangeFormatting":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 2},
					},
					"newText": "hi",
				}},
			})
		case "textDocument/inlayHint":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": []map[string]any{{
					"position": map[string]any{"line": 0, "character": 2},
					"label":    ": int",
				}},
			})
		case "shutdown":
			f.write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
		}
	}
}

// startFakeLspServer wires an LspServer to the given in-process fake.
// configure (optional) runs before Initialize, e.g. to set settings.
func startFakeLspServer(fake *fakeLspServer, cb lspCallbacks, configure func(*LspServer)) *LspServer {
	c2sR, c2sW := io.Pipe() // client writes → server reads
	s2cR, s2cW := io.Pipe() // server writes → client reads

	fake.in = bufio.NewReader(c2sR)
	fake.out = s2cW
	go fake.run()

	s := newLspServerIO(c2sW, s2cR)
	if configure != nil {
		configure(s)
	}
	s.Initialize("/tmp", cb)
	return s
}

// startFakeLsp wires an LspServer to an in-process fake server.
func startFakeLsp(initDelay time.Duration, onDiag func(lsp.PublishDiagnosticsParams)) (*LspServer, *fakeLspServer) {
	fake := &fakeLspServer{initDelay: initDelay}
	s := startFakeLspServer(fake, lspCallbacks{onDiag: onDiag}, nil)
	return s, fake
}

// waitReady fails the test if the handshake doesn't finish.
func waitReady(t *testing.T, s *LspServer) {
	t.Helper()
	select {
	case <-s.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("initialize did not complete")
	}
}

// drainUntil pumps the editor's main queue until cond holds.
func drainUntil(t *testing.T, ed *Editor, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ed.drainMain()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never happened", what)
}

// Notifications must not block the caller while the server is still
// initializing (previously DidOpen waited for the whole handshake).
func TestLspNotificationsDontBlock(t *testing.T) {
	s, fake := startFakeLsp(200*time.Millisecond, nil)

	start := time.Now()
	s.DidOpen("/tmp/x.go", "go", "package main\n", 0)
	s.DidChange("/tmp/x.go", 1, nil)
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Fatalf("notifications blocked for %v during initialization", elapsed)
	}

	// The messages must still arrive, after the handshake, in order.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ms := fake.methods()
		if len(ms) >= 4 {
			want := []string{"initialize", "initialized", "textDocument/didOpen", "textDocument/didChange"}
			for i, w := range want {
				if ms[i] != w {
					t.Fatalf("message order: got %v, want %v", ms, want)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("queued messages never arrived: %v", fake.methods())
}

// Requests work after initialization and respect capabilities.
func TestLspHoverRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)

	// Wait for the handshake so the hover capability is known.
	select {
	case <-s.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("initialize did not complete")
	}

	info, err := s.Hover("/tmp/x.go", lsp.Position{})
	if err != nil {
		t.Fatal(err)
	}
	if info != "hi" {
		t.Fatalf("hover: got %q, want %q", info, "hi")
	}
}

// Concurrent requests and diagnostic pushes must not race on the response
// map (run with -race).
func TestLspConcurrentTraffic(t *testing.T) {
	diags := make(chan struct{}, 64)
	s, fake := startFakeLsp(0, func(lsp.PublishDiagnosticsParams) {
		diags <- struct{}{}
	})
	select {
	case <-s.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("initialize did not complete")
	}

	// Server pushes diagnostics continuously.
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				fake.write(map[string]any{
					"jsonrpc": "2.0",
					"method":  "textDocument/publishDiagnostics",
					"params":  map[string]any{"uri": "file:///tmp/x.go", "diagnostics": []any{}},
				})
				time.Sleep(time.Millisecond)
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				s.Hover("/tmp/x.go", lsp.Position{})
			}
		}()
	}
	wg.Wait()
	close(stop)
}

// Buffer-word completion fallback actually inserts the candidate.
func TestBufferWordCompletionApplies(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foobar fo"))
	*b.Cursor() = b.Cursor().MoveTo(9)

	ed.triggerCompletion()
	if got := string(b.Slice(0, b.Len())); got != "foobar foobar" {
		t.Fatalf("buffer completion: got %q, want %q", got, "foobar foobar")
	}
	ed.acceptCompletion()
}

// Format edits are applied correctly even when the server returns them in
// ascending order (the spec doesn't guarantee any order).
func TestApplyTextEditsUnsorted(t *testing.T) {
	b := NewEmptyBuffer()
	b.text.Insert(0, []byte("aaa bbb\n"))

	edits := []lsp.TextEdit{
		// Descending order: applying naively in reverse index order would
		// apply the early edit first, shifting the later range.
		{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 4}, End: lsp.Position{Line: 0, Character: 7}}, NewText: "BBB"},
		{Range: lsp.Range{Start: lsp.Position{Line: 0, Character: 0}, End: lsp.Position{Line: 0, Character: 3}}, NewText: "ZZZZZ"},
	}
	applyTextEdits(b, edits)
	if got := string(b.Slice(0, b.Len())); got != "ZZZZZ BBB\n" {
		t.Fatalf("unsorted edits: got %q, want %q", got, "ZZZZZ BBB\n")
	}
}

// Hover must not block the event loop: the request runs in the background
// and the result arrives via the main queue.
func TestLspAsyncHoverNonBlocking(t *testing.T) {
	fake := &fakeLspServer{hoverDelay: 150 * time.Millisecond}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.lspServer = s

	start := time.Now()
	ed.lspHover()
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("lspHover blocked for %v", elapsed)
	}
	drainUntil(t, ed, "hover result", func() bool {
		return ed.infobar.message == "hi"
	})
}

// Async completion opens once the server answers, if nothing changed
// meanwhile.
func TestCompletionAsyncOpens(t *testing.T) {
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.lspServer = s
	b.text.Insert(0, []byte("fo"))
	*b.Cursor() = b.Cursor().MoveTo(2)
	ed.ks.SetMode(ModeInsert)

	ed.triggerCompletion()
	if ed.hasCompletion() {
		t.Fatal("completion opened synchronously")
	}
	drainUntil(t, ed, "completion", func() bool { return ed.hasCompletion() })
	if got := string(b.Slice(0, b.Len())); got != "foobar" {
		t.Fatalf("completion applied: got %q, want %q", got, "foobar")
	}
}

// A completion answer that arrives after the buffer changed is dropped.
func TestCompletionStaleDropped(t *testing.T) {
	fake := &fakeLspServer{complDelay: 100 * time.Millisecond}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.lspServer = s
	b.text.Insert(0, []byte("fo"))
	*b.Cursor() = b.Cursor().MoveTo(2)
	ed.ks.SetMode(ModeInsert)

	ed.triggerCompletion()
	b.lspVersion++ // the user typed while the request was in flight

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		ed.drainMain()
		time.Sleep(5 * time.Millisecond)
	}
	if ed.hasCompletion() {
		t.Fatal("stale completion answer was not dropped")
	}
	if got := string(b.Slice(0, b.Len())); got != "fo" {
		t.Fatalf("stale completion modified the buffer: %q", got)
	}
}

// A server textEdit range overrides the client-side word scan, including
// text after the cursor; cancelling restores it.
func TestCompletionTextEditRange(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("foXXX"))
	*b.Cursor() = b.Cursor().MoveTo(2)

	items := []lsp.CompletionItem{{
		Label: "foobar",
		TextEdit: &lsp.TextEdit{
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 0},
				End:   lsp.Position{Line: 0, Character: 5},
			},
			NewText: "foobar",
		},
	}}
	ed.finishCompletion(b, items, 0, "fo", 2)
	if got := string(b.Slice(0, b.Len())); got != "foobar" {
		t.Fatalf("textEdit range: got %q, want %q", got, "foobar")
	}
	ed.cancelCompletion()
	if got := string(b.Slice(0, b.Len())); got != "foXXX" {
		t.Fatalf("cancel after textEdit: got %q, want %q", got, "foXXX")
	}
}

// A textEdit starting before the scanned word start (e.g. completing past a
// '.') replaces from the server's start.
func TestCompletionTextEditStart(t *testing.T) {
	ed := newTestEditor()
	b := ed.ActiveView().buf
	b.text.Insert(0, []byte("a.fo"))
	*b.Cursor() = b.Cursor().MoveTo(4)

	items := []lsp.CompletionItem{{
		Label: "Z.fooZ",
		TextEdit: &lsp.TextEdit{
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 0},
				End:   lsp.Position{Line: 0, Character: 4},
			},
			NewText: "Z.fooZ",
		},
	}}
	// The word scan found startPos 2 ("fo"); the server range wins.
	ed.finishCompletion(b, items, 2, "fo", 4)
	if got := string(b.Slice(0, b.Len())); got != "Z.fooZ" {
		t.Fatalf("textEdit start: got %q, want %q", got, "Z.fooZ")
	}
}

// Server → client traffic: progress display, workDoneProgress/create,
// applyEdit refusal, and workspace/configuration answered from settings.
func TestLspServerToClientRequests(t *testing.T) {
	progress := make(chan string, 16)
	fake := &fakeLspServer{}
	s := startFakeLspServer(fake, lspCallbacks{
		onProgress: func(m string) { progress <- m },
	}, func(s *LspServer) {
		s.settings = map[string]any{"gopls": map[string]any{"usePlaceholders": true}}
	})
	waitReady(t, s)

	// Configured settings are pushed after the handshake.
	deadline := time.Now().Add(3 * time.Second)
	pushed := false
	for time.Now().Before(deadline) && !pushed {
		for _, m := range fake.methods() {
			if m == "workspace/didChangeConfiguration" {
				pushed = true
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !pushed {
		t.Fatalf("didChangeConfiguration never sent: %v", fake.methods())
	}

	expectProgress := func(want string) {
		t.Helper()
		select {
		case got := <-progress:
			if got != want {
				t.Fatalf("progress: got %q, want %q", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("progress %q never arrived", want)
		}
	}

	// Progress: create (a request we must answer) + begin/report/end.
	fake.write(map[string]any{"jsonrpc": "2.0", "id": 50,
		"method": "window/workDoneProgress/create", "params": map[string]any{"token": "t1"}})
	fake.waitReply(t, 50)

	fake.write(map[string]any{"jsonrpc": "2.0", "method": "$/progress",
		"params": map[string]any{"token": "t1", "value": map[string]any{"kind": "begin", "title": "Indexing"}}})
	expectProgress("Indexing")
	fake.write(map[string]any{"jsonrpc": "2.0", "method": "$/progress",
		"params": map[string]any{"token": "t1", "value": map[string]any{"kind": "report", "message": "3/5", "percentage": 60}}})
	expectProgress("Indexing: 3/5 (60%)")
	fake.write(map[string]any{"jsonrpc": "2.0", "method": "$/progress",
		"params": map[string]any{"token": "t1", "value": map[string]any{"kind": "end"}}})
	expectProgress("Indexing: done")

	// workspace/applyEdit is refused per spec, not answered with null.
	fake.write(map[string]any{"jsonrpc": "2.0", "id": 51,
		"method": "workspace/applyEdit", "params": map[string]any{"edit": map[string]any{}}})
	var applyResp struct {
		Result struct {
			Applied *bool `json:"applied"`
		} `json:"result"`
	}
	json.Unmarshal(fake.waitReply(t, 51), &applyResp)
	if applyResp.Result.Applied == nil || *applyResp.Result.Applied {
		t.Fatalf("applyEdit not refused: %s", string(mustReply(t, fake, 51)))
	}

	// workspace/configuration answers each item from settings.
	fake.write(map[string]any{"jsonrpc": "2.0", "id": 52,
		"method": "workspace/configuration",
		"params": map[string]any{"items": []map[string]any{{"section": "gopls"}, {"section": "nope"}}}})
	var confResp struct {
		Result []any `json:"result"`
	}
	json.Unmarshal(fake.waitReply(t, 52), &confResp)
	if len(confResp.Result) != 2 {
		t.Fatalf("configuration reply: %v", confResp.Result)
	}
	gopls, ok := confResp.Result[0].(map[string]any)
	if !ok || gopls["usePlaceholders"] != true {
		t.Fatalf("gopls section = %v, want usePlaceholders=true", confResp.Result[0])
	}
	if confResp.Result[1] != nil {
		t.Fatalf("unknown section = %v, want null", confResp.Result[1])
	}
}

func mustReply(t *testing.T, fake *fakeLspServer, id int) json.RawMessage {
	t.Helper()
	r, _ := fake.reply(id)
	return r
}

func TestLookupSettings(t *testing.T) {
	settings := map[string]any{
		"gopls": map[string]any{
			"analyses": map[string]any{"unusedparams": true},
		},
	}
	if got := lookupSettings(settings, "gopls.analyses.unusedparams"); got != true {
		t.Fatalf("dotted lookup = %v, want true", got)
	}
	if got := lookupSettings(settings, "missing"); got != nil {
		t.Fatalf("missing section = %v, want nil", got)
	}
	if got := lookupSettings(nil, "gopls"); got != nil {
		t.Fatalf("nil settings = %v, want nil", got)
	}
	if got := lookupSettings(settings, ""); got == nil {
		t.Fatal("empty section should return the whole map")
	}
}

// A hung server that ignores the shutdown handshake and EOF must be killed
// at exit, within a bounded time.
func TestLspShutdownKillsHungServer(t *testing.T) {
	s, err := startLspServer(LspLanguage{Command: "sleep", Args: []string{"30"}})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	s.Shutdown()
	elapsed := time.Since(start)
	if elapsed > 4*time.Second {
		t.Fatalf("Shutdown stalled for %v", elapsed)
	}

	select {
	case <-s.exited:
	default:
		t.Fatal("server process still running after Shutdown")
	}
}

// A server that exits on stdin EOF is not killed: the EOF cue suffices.
func TestLspShutdownEOFExit(t *testing.T) {
	s, err := startLspServer(LspLanguage{Command: "cat"})
	if err != nil {
		t.Fatal(err)
	}

	s.Shutdown()
	select {
	case <-s.exited:
	default:
		t.Fatal("server process still running after Shutdown")
	}
	if st := s.cmd.ProcessState; st == nil || !st.Exited() || st.ExitCode() != 0 {
		t.Fatalf("expected clean EOF exit, got %v", s.cmd.ProcessState)
	}
}

// An initialized, responsive server gets the polite shutdown + exit
// handshake and Shutdown returns promptly.
func TestLspShutdownPolite(t *testing.T) {
	s, fake := startFakeLsp(0, nil)
	waitReady(t, s)

	start := time.Now()
	s.Shutdown()
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("polite shutdown took %v", elapsed)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var sawShutdown, sawExit bool
		for _, m := range fake.methods() {
			switch m {
			case "shutdown":
				sawShutdown = true
			case "exit":
				sawExit = true
			}
		}
		if sawShutdown && sawExit {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("shutdown/exit never sent: %v", fake.methods())
}

// Tier 2 requests: signature help, references, document symbols, rename.

func TestLspSignatureHelpRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	help, err := s.SignatureHelp("/tmp/x.go", lsp.Position{})
	if err != nil {
		t.Fatal(err)
	}
	if got := signatureHelpText(help); got != "foo(a, [b int])" {
		t.Fatalf("signature help: got %q, want %q", got, "foo(a, [b int])")
	}
}

// signatureHelpText must also handle the [start, end) offset-pair form of
// ParameterInformation.Label, not just a plain substring.
func TestSignatureHelpTextOffsetLabel(t *testing.T) {
	help := &lspSignatureHelp{
		Signatures: []lspSignatureInformation{{
			Label: "foo(a, b int)",
			Parameters: []lspParameterInformation{
				{Label: json.RawMessage(`[4,5]`)},
				{Label: json.RawMessage(`[7,12]`)},
			},
		}},
		ActiveParameter: uint32Ptr(1),
	}
	if got := signatureHelpText(help); got != "foo(a, [b int])" {
		t.Fatalf("offset label: got %q, want %q", got, "foo(a, [b int])")
	}
}

func uint32Ptr(v uint32) *uint32 { return &v }

func TestLspReferencesRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	locs, err := s.References("/tmp/x.go", lsp.Position{})
	if err != nil {
		t.Fatal(err)
	}
	if len(locs) != 1 || locs[0].URI.Filename() != "/tmp/x.go" {
		t.Fatalf("references: got %v", locs)
	}
}

func TestLspDocumentSymbolsHierarchical(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	syms, err := s.DocumentSymbols("/tmp/x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0].Name != "foo" || syms[0].Kind != lsp.SymbolKindFunction {
		t.Fatalf("document symbols: got %+v", syms)
	}
	if syms[0].SelectionRange.Start.Character != 5 {
		t.Fatalf("selection range: got %+v", syms[0].SelectionRange)
	}
}

// DocumentSymbols must also normalize the flat, legacy SymbolInformation
// response shape (identified by a "location" field) into DocumentSymbol.
func TestLspDocumentSymbolsFlatFallback(t *testing.T) {
	fake := &fakeLspServer{symbolsFlat: true}
	s := startFakeLspServer(fake, lspCallbacks{}, nil)
	waitReady(t, s)

	syms, err := s.DocumentSymbols("/tmp/x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0].Name != "bar" {
		t.Fatalf("flat document symbols: got %+v", syms)
	}
	if syms[0].SelectionRange != syms[0].Range {
		t.Fatalf("flat symbol should reuse location as both range and selection range: %+v", syms[0])
	}
}

func TestLspRenameRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	edit, err := s.Rename("/tmp/x.go", lsp.Position{}, "renamed")
	if err != nil {
		t.Fatal(err)
	}
	edits, ok := edit.Changes["file:///tmp/x.go"]
	if !ok || len(edits) != 1 || edits[0].NewText != "renamed" {
		t.Fatalf("rename edit: got %+v", edit)
	}
}

func TestLspFormatRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	edits, err := s.Format("/tmp/x.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].NewText != "fmt" {
		t.Fatalf("format: got %+v", edits)
	}
}

func TestLspRangeFormattingRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	edits, err := s.RangeFormatting("/tmp/x.go", lsp.Range{})
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].NewText != "hi" {
		t.Fatalf("range formatting: got %+v", edits)
	}
}

func TestLspWorkspaceSymbolsRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	syms, err := s.WorkspaceSymbols("foo")
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 || syms[0].Name != "fooSymbol" {
		t.Fatalf("workspace symbols: got %+v", syms)
	}
}

func TestLspCallHierarchyRequests(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	items, err := s.PrepareCallHierarchy("/tmp/x.go", lsp.Position{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "target" {
		t.Fatalf("prepare call hierarchy: got %+v", items)
	}

	incoming, err := s.IncomingCalls(items[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(incoming) != 1 || incoming[0].From.Name != "caller" {
		t.Fatalf("incoming calls: got %+v", incoming)
	}

	outgoing, err := s.OutgoingCalls(items[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 || outgoing[0].To.Name != "callee" {
		t.Fatalf("outgoing calls: got %+v", outgoing)
	}
}

// The wire-only inlay hint types must decode both a plain-string label and
// the array-of-parts label form.
func TestInlayHintLabelText(t *testing.T) {
	if got := inlayHintLabelText(json.RawMessage(`": int"`)); got != ": int" {
		t.Fatalf("string label: got %q", got)
	}
	parts := json.RawMessage(`[{"value":": "},{"value":"int"}]`)
	if got := inlayHintLabelText(parts); got != ": int" {
		t.Fatalf("parts label: got %q", got)
	}
}

func TestLspInlayHintsRequest(t *testing.T) {
	s, _ := startFakeLsp(0, nil)
	waitReady(t, s)

	hints, err := s.InlayHints("/tmp/x.go", lsp.Position{Line: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || inlayHintLabelText(hints[0].Label) != ": int" {
		t.Fatalf("inlay hints: got %+v", hints)
	}
}
