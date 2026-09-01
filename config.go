package main

import (
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/pelletier/go-toml"
	"github.com/zyedidia/ftdetect"
	"github.com/zyedidia/glob"
	"gopkg.in/yaml.v2"
)

//go:embed embed
var embedFS embed.FS

// --- Options ---

// globals are options that apply editor-wide and are never overridden
// per-buffer.
var globalOpts = map[string]bool{
	"theme":     true,
	"clipboard": true,
	"cursor":    true,
}

// optionVerify provides validation functions for specific options.
var optionVerify = map[string]func(any) error{
	"tabchar": func(v any) error {
		s, ok := v.(string)
		if !ok {
			return errors.New("tabchar: expected string")
		}
		if s != "" && tabCharOf(s) == 0 {
			return errors.New("tabchar: expected a single narrow character, or \"\" for none")
		}
		return nil
	},
	"clipboard": func(v any) error {
		s, ok := v.(string)
		if !ok {
			return errors.New("clipboard: expected string")
		}
		switch s {
		case "internal", "external", "terminal":
			return nil
		}
		return errors.New("clipboard: expected 'internal', 'external', or 'terminal'")
	},
}

type ftOpts struct {
	ft   string
	opts map[string]any
}

// Options holds parsed TOML options with per-filetype overrides.
type Options struct {
	top map[string]any
	ft  []ftOpts

	// set records options changed by ":set" this session. They live in top
	// alongside the defaults, so this is what tells the two apart: an
	// option the user has just set outranks anything the editor infers
	// about a file (see SetForBuffer).
	set map[string]bool
}

// LoadOptions parses a TOML options file. Top-level keys are global/default
// options. Table sections are per-filetype overrides (or glob patterns with
// the "glob:" prefix).
func LoadOptions(data []byte) (*Options, error) {
	var optmap map[string]any
	if err := toml.Unmarshal(data, &optmap); err != nil {
		return nil, err
	}
	opts := &Options{
		top: make(map[string]any),
	}
	var fts, globs []ftOpts
	for k, v := range optmap {
		switch v := v.(type) {
		case map[string]any:
			if strings.HasPrefix(k, "glob:") {
				globs = append(globs, ftOpts{ft: k, opts: v})
			} else {
				fts = append(fts, ftOpts{ft: k, opts: v})
			}
		default:
			opts.top[k] = v
		}
	}
	// Deterministic application order (Resolve applies later sections over
	// earlier ones): filetype sections first, then glob sections, so glob
	// matches override filetype matches (see PLAN.md resolution order).
	sort.Slice(fts, func(i, j int) bool { return fts[i].ft < fts[j].ft })
	sort.Slice(globs, func(i, j int) bool { return globs[i].ft < globs[j].ft })
	opts.ft = append(fts, globs...)
	return opts, nil
}

// Resolve returns the effective options for a buffer at the given path with
// the given filetype. Global-only options are excluded. Resolution order:
// defaults, then filetype match, then glob match (later matches override).
func (o *Options) Resolve(path, ft string) map[string]any {
	m := make(map[string]any)
	// Start with top-level defaults (excluding globals).
	for k, v := range o.top {
		if globalOpts[k] {
			continue
		}
		m[k] = v
	}
	// Apply filetype and glob overrides.
	o.eachSection(path, ft, func(sec map[string]any) {
		for k, v := range sec {
			if globalOpts[k] {
				continue
			}
			m[k] = v
		}
	})
	return m
}

// eachSection calls fn for every filetype and glob section that applies to a
// buffer at path with the given filetype, in resolution order.
func (o *Options) eachSection(path, ft string, fn func(map[string]any)) {
	for _, fto := range o.ft {
		if strings.HasPrefix(fto.ft, "glob:") {
			globstr := fto.ft[5:]
			rgx, err := glob.Compile(globstr)
			if err != nil {
				log.Printf("config: bad glob %q: %v", globstr, err)
				continue
			}
			if !rgx.MatchString(path) {
				continue
			}
		} else if ft != fto.ft {
			continue
		}
		fn(fto.opts)
	}
}

// SetForBuffer reports whether name is set specifically for this buffer,
// rather than merely inherited from the top-level defaults: either by a
// [filetype] or glob section that applies to it, or by a ":set" this
// session. Used to decide whether something the editor infers about the
// file — its indentation, say — may override the configuration.
func (o *Options) SetForBuffer(path, ft, name string) bool {
	if o.set[name] {
		return true
	}
	found := false
	o.eachSection(path, ft, func(sec map[string]any) {
		if _, ok := sec[name]; ok {
			found = true
		}
	})
	return found
}

// --- Config ---

// Config holds the editor configuration: parsed options, user config
// directory, and methods to load resources (themes, LSP config).
type Config struct {
	opts *Options
	dir  string // user config directory (~/.config/mu)

	// Filetype detectors, built lazily from the embedded set plus the user's
	// detectors/ directory. Cached here rather than in a package global so the
	// set always reflects this config's directory.
	detectorsOnce sync.Once
	detectors     ftdetect.Detectors
}

// LoadConfig loads the configuration, merging embedded defaults with user
// overrides from ~/.config/mu/options.toml.
func LoadConfig() (*Config, error) {
	// Load embedded defaults.
	defaultData, err := embedFS.ReadFile("embed/options.toml")
	if err != nil {
		return nil, fmt.Errorf("config: embedded options.toml: %w", err)
	}
	opts, err := LoadOptions(defaultData)
	if err != nil {
		return nil, fmt.Errorf("config: parse defaults: %w", err)
	}

	dir := configDir()

	// Overlay user config if it exists.
	userPath := filepath.Join(dir, "options.toml")
	if userData, err := os.ReadFile(userPath); err == nil {
		userOpts, err := LoadOptions(userData)
		if err != nil {
			log.Printf("config: parse %s: %v", userPath, err)
		} else {
			for k, v := range userOpts.top {
				opts.top[k] = v
			}
			opts.ft = append(opts.ft, userOpts.ft...)
		}
	}

	return &Config{opts: opts, dir: dir}, nil
}

// configDirOverride can be set by tests to avoid writing to ~/.config/mu.
var configDirOverride string

// configDir returns the user config directory, creating it if needed.
func configDir() string {
	if configDirOverride != "" {
		os.MkdirAll(configDirOverride, 0755)
		return configDirOverride
	}
	dir := filepath.Join(userConfigDir(), "mu")
	os.MkdirAll(dir, 0755)
	return dir
}

// userConfigDir returns the XDG config home or a fallback.
func userConfigDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config")
}

// ReadFile reads a resource file, checking the user config directory first
// and falling back to embedded defaults.
func (c *Config) ReadFile(path string) ([]byte, error) {
	// Try user config dir.
	fp := filepath.Join(c.dir, path)
	data, err := os.ReadFile(fp)
	if err == nil {
		return data, nil
	}
	// Fall back to embedded.
	return embedFS.ReadFile(filepath.Join("embed", path))
}

// --- Theme loading ---

// LoadTheme loads a theme by name from the config.
func (c *Config) LoadTheme(name string) (*Theme, error) {
	data, err := c.ReadFile(filepath.Join("themes", name+".yaml"))
	if err != nil {
		return nil, fmt.Errorf("config: theme %q: %w", name, err)
	}
	return LoadThemeYAML(data)
}

// --- Comment prefixes ---

// LoadComments loads the filetype → line-comment-prefix table from
// comments.toml. User entries in the config directory are merged over the
// embedded defaults, so a user file only needs the overrides.
func (c *Config) LoadComments() map[string]string {
	m := make(map[string]string)
	merge := func(data []byte, src string) {
		var raw map[string]any
		if err := toml.Unmarshal(data, &raw); err != nil {
			log.Printf("comments: parse %s: %v", src, err)
			return
		}
		for ft, v := range raw {
			if s, ok := v.(string); ok {
				m[ft] = s
			} else {
				log.Printf("comments: %s: %s: expected string", src, ft)
			}
		}
	}
	if data, err := embedFS.ReadFile("embed/comments.toml"); err == nil {
		merge(data, "embedded comments.toml")
	}
	userPath := filepath.Join(c.dir, "comments.toml")
	if data, err := os.ReadFile(userPath); err == nil {
		merge(data, userPath)
	}
	return m
}

// --- LSP config ---

// LspLanguage describes how to launch a language server.
type LspLanguage struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Ft      string   `yaml:"ft"`

	// Settings is the workspace configuration for the server: it is pushed
	// via workspace/didChangeConfiguration after initialization and served
	// section-by-section when the server requests workspace/configuration
	// (e.g. a "gopls" key answers the "gopls" section).
	Settings map[string]any `yaml:"settings"`
	// InitOptions is passed verbatim as initializationOptions in the
	// initialize request, for servers configured that way.
	InitOptions map[string]any `yaml:"init_options"`
}

// LoadLspLanguages loads the LSP server configuration.
func (c *Config) LoadLspLanguages() (map[string]LspLanguage, error) {
	data, err := c.ReadFile("lsp.yaml")
	if err != nil {
		return nil, err
	}
	var langs map[string]LspLanguage
	err = yaml.Unmarshal(data, &langs)
	return langs, err
}

// --- Option access ---

// GlobalOpt returns a global option value.
func (c *Config) GlobalOpt(name string) any {
	return c.opts.top[name]
}

// GlobalStrOpt returns a global string option.
func (c *Config) GlobalStrOpt(name string) string {
	if v, ok := c.opts.top[name].(string); ok {
		return v
	}
	return ""
}

// SetGlobalOpt sets a global option, validating the type and value.
func (c *Config) SetGlobalOpt(name string, val any) error {
	if v, ok := c.opts.top[name]; ok {
		if reflect.TypeOf(v) != reflect.TypeOf(val) {
			return fmt.Errorf("config: %s: type mismatch: expected %T, got %T", name, v, val)
		}
	}
	if vf, ok := optionVerify[name]; ok {
		if err := vf(val); err != nil {
			return err
		}
	}
	c.opts.top[name] = val
	if c.opts.set == nil {
		c.opts.set = make(map[string]bool)
	}
	c.opts.set[name] = true
	return nil
}

// SetForBuffer reports whether name is set specifically for the buffer at
// path with the given filetype (see Options.SetForBuffer).
func (c *Config) SetForBuffer(path, ft, name string) bool {
	return c.opts.SetForBuffer(path, ft, name)
}

// IsGlobalOpt returns true if the option is global-only.
func IsGlobalOpt(name string) bool {
	return globalOpts[name]
}

// BufferOptions returns the resolved options for a buffer with the given
// path and filetype.
func (c *Config) BufferOptions(path, ft string) map[string]any {
	return c.opts.Resolve(path, ft)
}

// tabCharOf returns the mark to show at the start of a tab, or 0 for none.
// The mark stands in the tab's first cell and the renderer pads the rest
// (see Visualizer.String), so anything that would not fit one cell — more
// than one character, or a wide one — marks nothing rather than pushing the
// line out of alignment.
func tabCharOf(s string) rune {
	r, size := utf8.DecodeRuneInString(s)
	if size != len(s) || r == utf8.RuneError || runewidth.RuneWidth(r) != 1 {
		return 0
	}
	return r
}

// --- Typed option getters ---

// GetOptBool returns a boolean option from a resolved option map.
func GetOptBool(opts map[string]any, name string) (bool, bool) {
	v, ok := opts[name]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// GetOptString returns a string option.
func GetOptString(opts map[string]any, name string) (string, bool) {
	v, ok := opts[name]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// GetOptInt returns an integer option, handling TOML's int64 type.
func GetOptInt(opts map[string]any, name string) (int, bool) {
	v, ok := opts[name]
	if !ok {
		return 0, false
	}
	switch v := v.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}
