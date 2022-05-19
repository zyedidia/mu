package buf

import (
	"fmt"
	"log"

	"github.com/zyedidia/ned/buffer"
	"github.com/zyedidia/ned/pkg/theme"
)

type Config interface {
	buffer.Config
	LoadTheme(name string) (*theme.Theme, error)
}

func (bp *BufPane) InitOpts() {
	if th, ok := bp.Buffer.GetStrOpt("theme"); ok {
		bp.SetOpt("theme", th)
	}
	if th, ok := bp.Buffer.GetStrOpt("charmap"); ok {
		bp.SetOpt("charmap", th)
	}
}

func (bp *BufPane) SetOpt(opt string, val interface{}) error {
	switch opt {
	case "theme":
		thname, ok := val.(string)
		if !ok {
			return fmt.Errorf("error: value not a string")
		}
		th, err := bp.cfg.LoadTheme(thname)
		if err != nil {
			return fmt.Errorf("error (%s): %v\n", thname, err)
		}
		bp.theme = th
	case "charmap":
		log.Printf("TODO: set charmap %v", val)
		// vals, ok := val.(string)
		// if !ok {
		// 	return fmt.Errorf("error: value not a string")
		// }
		// valb := []byte(vals)
		// utf8.DecodeRuneInString(valb)
	}
	return bp.Buffer.SetOpt(opt, val)
}
