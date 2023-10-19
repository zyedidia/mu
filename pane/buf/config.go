package buf

import (
	"log"

	"github.com/zyedidia/mu/buffer"
)

type Config interface {
	buffer.Config
}

func (bp *BufPane) InitOpts() {
	if th, ok := bp.Buffer.GetStrOpt("charmap"); ok {
		bp.SetOpt("charmap", th)
	}
}

func (bp *BufPane) SetOpt(opt string, val interface{}) error {
	switch opt {
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

func (bp *BufPane) GetOpt(opt string) (interface{}, bool) {
	return bp.Buffer.GetOpt(opt)
}
