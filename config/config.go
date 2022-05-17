package config

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/ned/pkg/theme"
)

const (
	themeDir       = "themes"
	highlighterDir = "highlighters"
	bindingsDir    = "bindings"
	detectorDir    = "detectors"
)

var cdir string
var cfs *ConfigFS

func init() {
	cfs = &ConfigFS{
		embed:  builtin,
		config: nil,
	}
}

func ConfigDir() string {
	return cdir
}

func SetConfigDir(dir string) {
	cdir = dir
	cfs.config = os.DirFS(dir)
}

func LoadHighlighter(name string) (*flare.Highlighter, error) {
	data, err := fs.ReadFile(cfs, filepath.Join(highlighterDir, name+".lang"))
	if err != nil {
		return nil, err
	}
	return flare.LoadHighlighter(name, data, true)
}

func LoadDetectors() ftdetect.Detectors {
	detectors := make(ftdetect.Detectors)
	walkfn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			data, err := fs.ReadFile(cfs.embed, path)
			if err != nil {
				return nil
			}
			detector, err := ftdetect.LoadDetectorJson(data)
			if err != nil {
				return nil
			}
			detectors.RegisterDetector(detector)
		}

		return nil
	}

	cfs.WalkDir(detectorDir, walkfn)

	return detectors
}

func LoadTheme(name string) (*theme.Theme, error) {
	data, err := fs.ReadFile(cfs, filepath.Join(themeDir, name+".yaml"))
	if err != nil {
		return nil, err
	}
	return theme.LoadYAML(data)
}

// func LoadBindings(name string) kbd.Config {
//
// }
