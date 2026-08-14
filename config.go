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

	"github.com/pelletier/go-toml"
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
		for k, v := range fto.opts {
			if globalOpts[k] {
				continue
			}
			m[k] = v
		}
	}
	return m
}

// --- Config ---

// Config holds the editor configuration: parsed options, user config
// directory, and methods to load resources (themes, LSP config).
type Config struct {
	opts *Options
	dir  string // user config directory (~/.config/mu)
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

// --- LSP config ---

// LspLanguage describes how to launch a language server.
type LspLanguage struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Ft      string   `yaml:"ft"`
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
	return nil
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
