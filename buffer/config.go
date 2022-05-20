package buffer

import (
	"log"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
)

const (
	OptEndings  = "endings"
	OptEncoding = "encoding"
	OptSyntax   = "syntax"
	OptFiletype = "filetype"
)

type Config interface {
	LoadDetectors() ftdetect.Detectors
	LoadHighlighter(name string) (*flare.Highlighter, error)

	GetBufferOptions(path string, ft string) map[string]interface{}
	GetGlobalOption(name string) interface{}
}

func GetOpt[T any](opts map[string]interface{}, name string) (t T, v bool) {
	if opt, ok := opts[name]; ok {
		if str, ok := opt.(T); ok {
			return str, true
		}
		log.Printf("error getting option %s: incorrect type\n", name)
		return
	}
	log.Printf("error getting option %s: not found\n", name)
	return
}

func (b *Buffer) GetStrOpt(name string) (o string, v bool) {
	if opt, ok := b.Options[name]; ok {
		if str, ok := opt.(string); ok {
			return str, true
		}
		log.Printf("error getting option %s: not a string\n", name)
		return
	}
	log.Printf("error getting option %s: not found\n", name)
	return
}

func (b *Buffer) GetBoolOpt(name string) (o bool, v bool) {
	if opt, ok := b.Options[name]; ok {
		if bl, ok := opt.(bool); ok {
			return bl, true
		}
		log.Printf("error getting option %s: not a bool\n", name)
		return
	}
	log.Printf("error getting option %s: not found\n", name)
	return
}

// MustGetBoolOpt is the same as GetBoolOpt but returns the default value if it
// is not found
func (b *Buffer) MustGetBoolOpt(name string) bool {
	if opt, ok := b.Options[name]; ok {
		if bl, ok := opt.(bool); ok {
			return bl
		}
	}
	return false
}

func (b *Buffer) SetOpt(name string, val interface{}) error {
	b.Options[name] = val
	return nil
}
