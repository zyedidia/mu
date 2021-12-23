package ned

import "github.com/zyedidia/ned/buffer"

type Buffer struct {
	*buffer.Buffer

	Cursor Cursor
}

func NewBuffer(b *buffer.Buffer) *Buffer {
	return &Buffer{
		Buffer: b,
		Cursor: SpawnCursorAt(0),
	}
}
