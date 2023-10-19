package text

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/gogs/chardet"
	"github.com/zyedidia/mu/buffer/text/endings"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

// Reads the contents of a reader into a byte slice in the internal
// representation format which is UTF-8 with LF line endings. This will be
// efficient for files/data already in that format (files will be mmapped), but
// additional conversions for files in different encodings will be somewhat
// costly.
// Assigns any auto-detected values to the appropriate fields in opts.
func readToUTF8LF(data []byte, opts *Options) ([]byte, error) {
	charset := utf8enc
	if opts.Charset != nil {
		// If a charset has been explicitly provided, use that one.
		charset = *opts.Charset
	} else {
		// Special-case auto-detection for utf8. If there are invalid utf8
		// bytes in the first part of the document then we do an encoding
		// auto-detection.
		utf8 := IsUTF8(data[:min(4096, len(data))])
		if !utf8 {
			encname, err := chardet.NewTextDetector().DetectBest(data)
			if err != nil {
				return nil, fmt.Errorf("chardet: %w", err)
			}
			charset, err = htmlindex.Get(encname.Charset)
			if err != nil {
				return nil, fmt.Errorf("chardet: %w", err)
			}
		}
	}

	var err error

	// If the user ended up requesting utf8 we don't have to reread, but
	// otherwise we need to transform the data to utf8 so that the internal
	// representation is correct.
	if charset != utf8enc {
		data, err = reread(data, charset.NewDecoder())
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
	}

	// If the user explicitly requested a line ending encoding, use that one.
	// Otherwise auto-detect the line endings.
	var endtype endings.Type
	if opts.Endings != nil {
		endtype = *opts.Endings
	} else {
		endtype = endings.Detect(data)
	}

	// If we are loading a non-LF file, we need to convert all line endings to
	// LF so that the internal representation is correct.
	if endtype != endings.LF {
		data, err = reread(data, &endings.ToLF{})
		if err != nil {
			return nil, fmt.Errorf("crlf-convert: %w", err)
		}
	}
	opts.Endings = &endtype
	opts.Charset = &charset

	return data, nil
}

// Re-read the data given with a transformer applied. No matter what, this
// function will free the old data, even if there is an error during the
// re-read. Returns the new data.
func reread(data []byte, t transform.Transformer) ([]byte, error) {
	newdata, err := ioutil.ReadAll(transform.NewReader(bytes.NewBuffer(data), t))
	return newdata, err
}

// Returns true if this file cannot be opened for writing.
func isReadonly(f *os.File) bool {
	wrf, err := os.OpenFile(f.Name(), os.O_WRONLY, 0)
	wrf.Close()
	return err != nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
