package text

import "golang.org/x/text/encoding/htmlindex"

var utf8enc, _ = htmlindex.Get("utf-8")

// IsUTF8 returns true if the byte slice is a valid UTF-8 encoding.
func IsUTF8(b []byte) bool {
	inputLen := len(b)
	var numValid, numInvalid uint32
	var trailBytes uint8
	for i := 0; i < inputLen; i++ {
		c := b[i]
		if c&0x80 == 0 {
			continue
		}
		if c&0xE0 == 0xC0 {
			trailBytes = 1
		} else if c&0xF0 == 0xE0 {
			trailBytes = 2
		} else if c&0xF8 == 0xF0 {
			trailBytes = 3
		} else {
			numInvalid++
			if numInvalid > 5 {
				break
			}
			trailBytes = 0
		}

		for i++; i < inputLen; i++ {
			c = b[i]
			if c&0xC0 != 0x80 {
				numInvalid++
				break
			}
			if trailBytes--; trailBytes == 0 {
				numValid++
				break
			}
		}
	}

	// if there are no invalid bytes we automatically assume utf-8
	_ = numValid
	return numInvalid == 0
}
