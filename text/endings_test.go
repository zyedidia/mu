package text_test

import (
	"bytes"
	"testing"

	"github.com/zyedidia/mu/text"
	"golang.org/x/text/transform"
)

func TestDetectLineEndingLF(t *testing.T) {
	if got := text.DetectLineEnding([]byte("hello\nworld\n")); got != text.LF {
		t.Fatalf("got %v, want LF", got)
	}
}

func TestDetectLineEndingCRLF(t *testing.T) {
	if got := text.DetectLineEnding([]byte("hello\r\nworld\r\n")); got != text.CRLF {
		t.Fatalf("got %v, want CRLF", got)
	}
}

func TestDetectLineEndingNoNewline(t *testing.T) {
	if got := text.DetectLineEnding([]byte("hello")); got != text.LF {
		t.Fatalf("got %v, want LF (default)", got)
	}
}

func TestDetectLineEndingEmpty(t *testing.T) {
	if got := text.DetectLineEnding([]byte{}); got != text.LF {
		t.Fatalf("got %v, want LF (default)", got)
	}
}

func TestDetectLineEndingNewlineFirst(t *testing.T) {
	// LF at index 0 — no preceding byte to check for CR
	if got := text.DetectLineEnding([]byte("\nhello")); got != text.LF {
		t.Fatalf("got %v, want LF", got)
	}
}

func transformAll(t transform.Transformer, input []byte) []byte {
	r := transform.NewReader(bytes.NewReader(input), t)
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.Bytes()
}

func TestToLF_CRLF(t *testing.T) {
	got := transformAll(&text.ToLF{}, []byte("a\r\nb\r\n"))
	want := []byte("a\nb\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToLF CRLF: got %q, want %q", got, want)
	}
}

func TestToLF_BareCR(t *testing.T) {
	got := transformAll(&text.ToLF{}, []byte("a\rb\r"))
	want := []byte("a\nb\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToLF bare CR: got %q, want %q", got, want)
	}
}

func TestToLF_LF(t *testing.T) {
	got := transformAll(&text.ToLF{}, []byte("a\nb\n"))
	want := []byte("a\nb\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToLF LF passthrough: got %q, want %q", got, want)
	}
}

func TestToLF_Empty(t *testing.T) {
	got := transformAll(&text.ToLF{}, []byte{})
	if len(got) != 0 {
		t.Fatalf("ToLF empty: got %q", got)
	}
}

func TestToLF_Mixed(t *testing.T) {
	// Mix of CR, LF, and CRLF — all become LF
	got := transformAll(&text.ToLF{}, []byte("a\rb\nc\r\nd"))
	want := []byte("a\nb\nc\nd")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToLF mixed: got %q, want %q", got, want)
	}
}

func TestToCRLF_Basic(t *testing.T) {
	got := transformAll(text.ToCRLF{}, []byte("a\nb\n"))
	want := []byte("a\r\nb\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToCRLF: got %q, want %q", got, want)
	}
}

func TestToCRLF_NoNewlines(t *testing.T) {
	got := transformAll(text.ToCRLF{}, []byte("hello"))
	want := []byte("hello")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToCRLF no newlines: got %q, want %q", got, want)
	}
}

func TestToCRLF_Empty(t *testing.T) {
	got := transformAll(text.ToCRLF{}, []byte{})
	if len(got) != 0 {
		t.Fatalf("ToCRLF empty: got %q", got)
	}
}

func TestToCRLF_OnlyNewlines(t *testing.T) {
	got := transformAll(text.ToCRLF{}, []byte("\n\n\n"))
	want := []byte("\r\n\r\n\r\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("ToCRLF only newlines: got %q, want %q", got, want)
	}
}

func TestLineEndingString(t *testing.T) {
	if text.LF.String() != "LF" {
		t.Fatalf("LF.String(): got %q", text.LF.String())
	}
	if text.CRLF.String() != "CRLF" {
		t.Fatalf("CRLF.String(): got %q", text.CRLF.String())
	}
}

func TestToLFToCRLFRoundTrip(t *testing.T) {
	original := []byte("line1\r\nline2\r\nline3")
	lf := transformAll(&text.ToLF{}, original)
	back := transformAll(text.ToCRLF{}, lf)
	if !bytes.Equal(back, []byte("line1\r\nline2\r\nline3")) {
		t.Fatalf("round trip: got %q", back)
	}
}
