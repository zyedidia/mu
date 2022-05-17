package config

import (
	"embed"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/ned/pkg/theme"
)

//go:embed embed
var builtin embed.FS

var (
	hembed embed.FS
	hbin   map[string]*flare.Highlighter

	dembed embed.FS
	dbin   []*ftdetect.Detector

	tembed embed.FS
	tbin   map[string]*theme.Theme

	bembed embed.FS
)
