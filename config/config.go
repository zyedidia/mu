package config

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/zyedidia/flare"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/kbd"
	"github.com/zyedidia/kbd/syntax"
	"github.com/zyedidia/ned/pkg/theme"
)

const (
	themeDir       = "themes"
	highlighterDir = "highlighters"
	bindingsDir    = "bindings"
	detectorDir    = "detectors"
)

func (cfs *ConfigFS) LoadHighlighter(name string) (*flare.Highlighter, error) {
	data, err := fs.ReadFile(cfs, filepath.Join(highlighterDir, name+".lang"))
	if err != nil {
		return nil, err
	}
	return flare.LoadHighlighter(name, data, true)
}

func (cfs *ConfigFS) LoadDetectors() ftdetect.Detectors {
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

func (cfs *ConfigFS) LoadTheme(name string) (*theme.Theme, error) {
	data, err := fs.ReadFile(cfs, filepath.Join(themeDir, name+".yaml"))
	if err != nil {
		return nil, err
	}
	return theme.LoadYAML(data)
}

func (cfs *ConfigFS) LoadBindings(name string) (kbd.Config, error) {
	data, err := fs.ReadFile(cfs, filepath.Join(bindingsDir, name+".kbd"))
	if err != nil {
		return kbd.Config{}, err
	}

	prog, err := syntax.Compile(name, string(data))
	if err != nil {
		return kbd.Config{}, err
	}

	return kbd.Config{
		Core: name,
		VM:   kbd.NewVM(prog.Compile()),
	}, nil
}

func (cfs *ConfigFS) MustLoadBindings(name string) kbd.Config {
	b, err := cfs.LoadBindings(name)
	if err != nil {
		panic(fmt.Errorf("error loading internal bindings (%s): %v\n", name, err))
	}
	return b
}

func (cfs *ConfigFS) GetBufferOptions(path, ft string) map[string]interface{} {
	return cfs.opts.LocalOptions(path, ft)
}

func (cfs *ConfigFS) IsGlobalOpt(name string) bool {
	return globals[name]
}

func GlobalOpt[T any](cfs *ConfigFS, name string) (t T, v bool) {
	if !globals[name] {
		return
	}
	if i, ok := cfs.opts.top[name]; ok {
		if o, ok := i.(T); ok {
			return o, true
		}
	}
	return
}

func MustGlobalOpt[T any](cfs *ConfigFS, name string) (t T) {
	if !globals[name] {
		return
	}
	if i, ok := cfs.opts.top[name]; ok {
		if o, ok := i.(T); ok {
			return o
		}
	}
	return
}

func (cfs *ConfigFS) GlobalOpt(name string) (interface{}, bool) {
	if !globals[name] {
		return nil, false
	}
	opt, v := cfs.opts.top[name]
	return opt, v
}

func (cfs *ConfigFS) GlobalStrOpt(name string) (string, bool) {
	return GlobalOpt[string](cfs, name)
}

func (cfs *ConfigFS) MustGlobalStrOpt(name string) string {
	return MustGlobalOpt[string](cfs, name)
}

func (cfs *ConfigFS) MustGlobalOpt(name string) interface{} {
	return cfs.opts.top[name]
}

func (cfs *ConfigFS) SetGlobalOpt(name string, val interface{}) error {
	cfs.opts.top[name] = val
	// TODO: give error if new value has different type from old value
	return nil
}
