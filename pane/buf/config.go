package buf

import "github.com/zyedidia/ned/pkg/theme"

type Config interface {
	LoadTheme(name string) (*theme.Theme, error)
}

var cfg Config

func SetConfig(c Config) {
	cfg = c
}
