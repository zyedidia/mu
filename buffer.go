package ned

import "github.com/zyedidia/ned/buffer"

type Buffer struct {
	*buffer.Buffer

	cursor int
}

func NewBuffer(b *buffer.Buffer) *Buffer {
	return &Buffer{
		Buffer: b,
	}
}
