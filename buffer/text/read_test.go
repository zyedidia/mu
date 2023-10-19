package text_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/zyedidia/mu/buffer/text"
	"github.com/zyedidia/mu/buffer/text/endings"
	"golang.org/x/text/encoding/htmlindex"
)

func check(want, got []byte, t *testing.T) {
	if !bytes.Equal(want, got) {
		t.Errorf("incorrect slices: want %s, got %s", string(want), string(got))
	}
}

func TestEndings(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/crlf.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, err := text.NewBuffer(data, text.Options{})
	if err != nil {
		t.Fatal(err)
	}

	if *b.Opts.Endings != endings.CRLF {
		t.Error("did not detect crlf line endings correctly")
	}

	check(bytes.ReplaceAll(data, []byte{'\r', '\n'}, []byte{'\n'}), b.Bytes(), t)

	buf := bytes.Buffer{}
	b.WriteTo(&buf)
	check(data, buf.Bytes(), t)
}

const utf8 = `the quick
brown fox jumps
over the lazy
dog
`

func TestEncoding(t *testing.T) {
	data, err := ioutil.ReadFile("testdata/utf16.txt")
	if err != nil {
		t.Fatal(err)
	}
	utf16, _ := htmlindex.Get("utf-16")
	b, err := text.NewBuffer(data, text.Options{
		Charset: &utf16,
	})
	if err != nil {
		t.Fatal(err)
	}

	if *b.Opts.Charset != utf16 {
		t.Error("did not detect utf-16 correctly")
	}

	check([]byte(utf8), b.Bytes(), t)

	buf := bytes.Buffer{}
	b.WriteTo(&buf)
	check(data, buf.Bytes(), t)
}
