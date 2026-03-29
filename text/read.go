package text

import (
	"bytes"
	"fmt"
	"io"

	"github.com/gogs/chardet"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

// Options configures how a buffer loads and saves text.
type Options struct {
	Charset *encoding.Encoding
	Endings *LineEnding
}

// readToUTF8LF reads raw data into the internal representation format (UTF-8
// with LF line endings). Assigns any auto-detected values to opts.
func readToUTF8LF(data []byte, opts *Options) ([]byte, error) {
	charset := utf8enc
	if opts.Charset != nil {
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

	// If the charset is not utf8 we need to transform the data.
	if charset != utf8enc {
		data, err = reread(data, charset.NewDecoder())
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
	}

	var endtype LineEnding
	if opts.Endings != nil {
		endtype = *opts.Endings
	} else {
		endtype = DetectLineEnding(data)
	}

	// Convert non-LF line endings to LF for the internal representation.
	if endtype != LF {
		data, err = reread(data, &ToLF{})
		if err != nil {
			return nil, fmt.Errorf("crlf-convert: %w", err)
		}
	}
	opts.Endings = &endtype
	opts.Charset = &charset

	return data, nil
}

// reread applies a transformer to data.
func reread(data []byte, t transform.Transformer) ([]byte, error) {
	return io.ReadAll(transform.NewReader(bytes.NewBuffer(data), t))
}
