package buffer

import "github.com/zyedidia/ned/buffer/text"

type Options struct {
	text.Options

	Filetype *string
	Syntax   bool
}
