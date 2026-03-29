package text_test

import (
	"bytes"
	"testing"

	"github.com/zyedidia/mu/text"
)

func newBuf(s string) *text.Buffer {
	return text.NewBufferFromUTF8([]byte(s), text.Options{})
}

func TestBufferConstruction(t *testing.T) {
	b := newBuf("hello\nworld\n")
	if b.Len() != 12 {
		t.Fatalf("Len: got %d, want 12", b.Len())
	}
	if b.Size() != 12 {
		t.Fatalf("Size: got %d, want 12", b.Size())
	}
	if b.NumLines() != 2 {
		t.Fatalf("NumLines: got %d, want 2", b.NumLines())
	}
	if !bytes.Equal(b.Bytes(), []byte("hello\nworld\n")) {
		t.Fatalf("Bytes: got %q", b.Bytes())
	}
}

func TestBufferConstructionEmpty(t *testing.T) {
	b := newBuf("")
	if b.Len() != 0 {
		t.Fatalf("Len: got %d, want 0", b.Len())
	}
	if b.NumLines() != 0 {
		t.Fatalf("NumLines: got %d, want 0", b.NumLines())
	}
}

func TestBufferGetLine(t *testing.T) {
	b := newBuf("hello\nworld\nfoo")
	tests := []struct {
		line int
		want string
	}{
		{0, "hello"},
		{1, "world"},
		{2, "foo"},
	}
	for _, tt := range tests {
		got := string(b.GetLine(tt.line))
		if got != tt.want {
			t.Errorf("GetLine(%d): got %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestBufferLineLen(t *testing.T) {
	b := newBuf("ab\ncde\n\nf")
	tests := []struct {
		line int
		want int
	}{
		{0, 2},
		{1, 3},
		{2, 0},
		{3, 1},
	}
	for _, tt := range tests {
		got := b.LineLen(tt.line)
		if got != tt.want {
			t.Errorf("LineLen(%d): got %d, want %d", tt.line, got, tt.want)
		}
	}
}

func TestBufferInsert(t *testing.T) {
	b := newBuf("helloworld")
	b.Insert(5, []byte(" "))
	if got := string(b.Bytes()); got != "hello world" {
		t.Fatalf("after insert: got %q", got)
	}
	if b.Len() != 11 {
		t.Fatalf("Len after insert: got %d", b.Len())
	}
}

func TestBufferRemove(t *testing.T) {
	b := newBuf("hello world")
	b.Remove(5, 6) // remove the space
	if got := string(b.Bytes()); got != "helloworld" {
		t.Fatalf("after remove: got %q", got)
	}
}

func TestBufferSlice(t *testing.T) {
	b := newBuf("abcdefghij")
	got := string(b.Slice(2, 7))
	if got != "cdefg" {
		t.Fatalf("Slice(2,7): got %q, want %q", got, "cdefg")
	}
}

func TestBufferOffsetAtLineColAt(t *testing.T) {
	b := newBuf("abc\ndef\nghi")
	tests := []struct {
		line, col int
		off       int
	}{
		{0, 0, 0},
		{0, 2, 2},
		{1, 0, 4},
		{1, 2, 6},
		{2, 0, 8},
		{2, 2, 10},
	}
	for _, tt := range tests {
		off := b.OffsetAt(tt.line, tt.col)
		if off != tt.off {
			t.Errorf("OffsetAt(%d,%d): got %d, want %d", tt.line, tt.col, off, tt.off)
		}
		line, col := b.LineColAt(tt.off)
		if line != tt.line || col != tt.col {
			t.Errorf("LineColAt(%d): got (%d,%d), want (%d,%d)", tt.off, line, col, tt.line, tt.col)
		}
	}
}

func TestBufferByteAt(t *testing.T) {
	b := newBuf("abcd")
	for i, expected := range []byte("abcd") {
		got := b.ByteAt(i)
		if got != expected {
			t.Errorf("ByteAt(%d): got %c, want %c", i, got, expected)
		}
	}
}

func TestBufferDecodeRuneAt(t *testing.T) {
	b := newBuf("aé日")
	// 'a' at 0: 1 byte
	r, sz := b.DecodeRuneAt(0)
	if r != 'a' || sz != 1 {
		t.Errorf("DecodeRuneAt(0): got (%c, %d), want ('a', 1)", r, sz)
	}
	// 'é' at 1: 2 bytes
	r, sz = b.DecodeRuneAt(1)
	if r != 'é' || sz != 2 {
		t.Errorf("DecodeRuneAt(1): got (%c, %d), want ('é', 2)", r, sz)
	}
	// '日' at 3: 3 bytes
	r, sz = b.DecodeRuneAt(3)
	if r != '日' || sz != 3 {
		t.Errorf("DecodeRuneAt(3): got (%c, %d), want ('日', 3)", r, sz)
	}
}

func TestBufferWrite(t *testing.T) {
	b := newBuf("hello")
	n, err := b.Write([]byte(" world"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 6 {
		t.Fatalf("Write n: got %d, want 6", n)
	}
	if got := string(b.Bytes()); got != "hello world" {
		t.Fatalf("after Write: got %q", got)
	}
}

func TestBufferWriteToCRLF(t *testing.T) {
	crlf := text.CRLF
	b := text.NewBufferFromUTF8([]byte("line1\nline2\n"), text.Options{
		Endings: &crlf,
	})
	var buf bytes.Buffer
	_, err := b.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo error: %v", err)
	}
	got := buf.String()
	want := "line1\r\nline2\r\n"
	if got != want {
		t.Fatalf("WriteTo CRLF: got %q, want %q", got, want)
	}
}

func TestBufferInsertNewlines(t *testing.T) {
	b := newBuf("ab")
	b.Insert(1, []byte("\n"))
	// "ab" -> "a\nb"
	if b.NumLines() != 1 {
		t.Fatalf("NumLines after inserting newline: got %d, want 1", b.NumLines())
	}
	if got := string(b.GetLine(0)); got != "a" {
		t.Fatalf("GetLine(0): got %q, want %q", got, "a")
	}
	if got := string(b.GetLine(1)); got != "b" {
		t.Fatalf("GetLine(1): got %q, want %q", got, "b")
	}
}

func TestBufferReadAt(t *testing.T) {
	b := newBuf("hello world")
	p := make([]byte, 5)
	n, err := b.ReadAt(p, 6)
	if err != nil {
		t.Fatalf("ReadAt error: %v", err)
	}
	if n != 5 || string(p) != "world" {
		t.Fatalf("ReadAt: got (%d, %q), want (5, %q)", n, p, "world")
	}
}

func TestNewBufferCRLF(t *testing.T) {
	b, err := text.NewBuffer([]byte("hello\r\nworld\r\n"), text.Options{})
	if err != nil {
		t.Fatalf("NewBuffer error: %v", err)
	}
	// Internal representation should be LF
	if got := string(b.Bytes()); got != "hello\nworld\n" {
		t.Fatalf("NewBuffer CRLF: internal got %q, want %q", got, "hello\nworld\n")
	}
	// WriteTo should restore CRLF
	var buf bytes.Buffer
	b.WriteTo(&buf)
	if got := buf.String(); got != "hello\r\nworld\r\n" {
		t.Fatalf("WriteTo after CRLF load: got %q", got)
	}
}
