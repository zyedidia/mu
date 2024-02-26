package buf

import (
	"github.com/zyedidia/mu/buffer"
)

type Config interface {
	buffer.Config
}

func (bp *BufPane) Modified() string {
	if bp.Buffer.Modified() {
		return "+ "
	}
	return ""
}
