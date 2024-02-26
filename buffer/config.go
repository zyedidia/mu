package buffer

import (
	"fmt"
	"log"
	"reflect"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/mu/config"
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
	GlobalOpt(name string) interface{}

	CacheFS() config.WriteFS
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

func (b *BufferData) GetOpt(name string) (o interface{}, v bool) {
	if opt, ok := b.Options[name]; ok {
		return opt, true
	}
	log.Printf("error getting option %s: not found\n", name)
	return
}

func (b *BufferData) GetStrOpt(name string) (o string, v bool) {
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

func (b *BufferData) IntOpt(name string) int {
	if opt, ok := b.Options[name]; ok {
		if bl, ok := opt.(int64); ok {
			return int(bl)
		}
	}
	return 0
}

func (b *BufferData) BoolOpt(name string) bool {
	if opt, ok := b.Options[name]; ok {
		if bl, ok := opt.(bool); ok {
			return bl
		}
	}
	return false
}

func (b *BufferData) StrOpt(name string) string {
	o, ok := b.GetStrOpt(name)
	if !ok {
		return ""
	}
	return o
}

func Opt[T any](b *Buffer, name string) (t T) {
	if opt, ok := b.Options[name]; ok {
		if bl, ok := opt.(T); ok {
			return bl
		}
	}
	return
}

func (b *BufferData) SetOpt(name string, val interface{}) error {
	if _, ok := b.Options[name]; !ok {
		return fmt.Errorf("%s: option not found", name)
	}
	if reflect.TypeOf(val) != reflect.TypeOf(b.Options[name]) {
		return fmt.Errorf("%w: expected %v, got %v", config.ErrTypeMismatch, reflect.TypeOf(b.Options[name]), reflect.TypeOf(val))
	}
	b.Options[name] = val

	b.updateOpt(name)

	return nil
}

func (b *BufferData) updateOpt(name string) {
	switch name {
	case "syntax":
		b.LoadHighlighter()
	case "filetype":
		b.InitFiletype()
	case "charmap":
		b.vis.CharMap = parseCharMap(b.StrOpt("charmap"))
	case "tabsize":
		b.vis.TabSize = b.IntOpt("tabsize")
	}
}

func parseCharMap(s string) map[rune]string {
	// charmap encoded as rune for '\t', '\n'
	runes := []rune{'\t', '\n'}
	m := make(map[rune]string)
	for i, r := range s {
		if i >= len(runes) {
			break
		}
		m[runes[i]] = string(r)
	}
	return m
}
