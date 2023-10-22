//go:build !linux && !darwin && !freebsd && !dragonfly && !openbsd_amd64

package term

import (
	"errors"
)

type TermPane struct{}

var ErrUnsupported = errors.New("unsupported system")
