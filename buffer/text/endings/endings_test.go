package endings

import (
	"testing"

	"golang.org/x/text/transform"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		in   string
		want Type
	}{
		{"hello, world\r\n", CRLF},
		{"hello, world\r", LF},
		{"hello, world\n", LF},
		{"", LF},
		{"\r\n", CRLF},
		{"hello,\r\nworld", CRLF},
		{"hello,\rworld", LF},
		{"hello,\nworld", LF},
		{"hello,\n\rworld", LF},
		{"hello,\r\n\r\nworld", CRLF},
	}

	for _, c := range tests {
		got := Detect([]byte(c.in))
		if got != c.want {
			t.Errorf("detect %q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToLF(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello, world\r\n", "hello, world\n"},
		{"hello, world\r", "hello, world\n"},
		{"hello, world\n", "hello, world\n"},
		{"", ""},
		{"\r\n", "\n"},
		{"hello,\r\nworld", "hello,\nworld"},
		{"hello,\rworld", "hello,\nworld"},
		{"hello,\nworld", "hello,\nworld"},
		{"hello,\n\rworld", "hello,\n\nworld"},
		{"hello,\r\n\r\nworld", "hello,\n\nworld"},
	}

	lf := &ToLF{}

	for _, c := range tests {
		got, _, err := transform.String(lf, c.in)
		if err != nil {
			t.Errorf("error transforming %q: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("transforming %q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestToCRLF(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello, world\n", "hello, world\r\n"},
		{"", ""},
		{"\n", "\r\n"},
		{"hello,\nworld", "hello,\r\nworld"},
		{"hello,\n\nworld", "hello,\r\n\r\nworld"},
	}

	for _, c := range tests {
		got, _, err := transform.String(ToCRLF{}, c.in)
		if err != nil {
			t.Errorf("error transforming %q: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("transforming %q: got %q, want %q", c.in, got, c.want)
		}
	}
}
