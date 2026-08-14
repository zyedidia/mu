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

	initDelay time.Duration

	mu       sync.Mutex
	received []string // method order as received
}

func (f *fakeLspServer) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.received))
	copy(out, f.received)
	return out
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
		f.mu.Lock()
		f.received = append(f.received, msg.Method)
		f.mu.Unlock()

		switch msg.Method {
		case "initialize":
			time.Sleep(f.initDelay)
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result": map[string]any{
					"capabilities": map[string]any{
						"hoverProvider": true,
					},
				},
			})
		case "textDocument/hover":
			f.write(map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  map[string]any{"contents": map[string]any{"kind": "plaintext", "value": "hi"}},
			})
		case "shutdown":
			f.write(map[string]any{"jsonrpc": "2.0", "id": *msg.ID, "result": nil})
		}
	}
}

// startFakeLsp wires an LspServer to an in-process fake server.
func startFakeLsp(initDelay time.Duration, onDiag func(lsp.PublishDiagnosticsParams)) (*LspServer, *fakeLspServer) {
	c2sR, c2sW := io.Pipe() // client writes → server reads
	s2cR, s2cW := io.Pipe() // server writes → client reads

	fake := &fakeLspServer{in: bufio.NewReader(c2sR), out: c2sW, initDelay: initDelay}
	fake.out = s2cW
	go fake.run()

	s := newLspServerIO(c2sW, s2cR)
	s.Initialize("/tmp", nil, onDiag)
	return s, fake
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
