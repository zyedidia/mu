package config

import (
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/zyedidia/flare"
)

type WriteFS string

func (wr WriteFS) Create(name string) (*os.File, error) {
	return os.Create(filepath.Join(string(wr), name))
}

func (wr WriteFS) Open(name string) (fs.File, error) {
	f, err := os.Open(filepath.Join(string(wr), name))
	if err != nil {
		return nil, err // nil fs.File
	}
	return f, nil
}

func (wr WriteFS) Stat(name string) (fs.FileInfo, error) {
	f, err := os.Stat(filepath.Join(string(wr), name))
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (wr WriteFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	return ioutil.WriteFile(filepath.Join(string(wr), name), data, perm)
}

type ConfigFS struct {
	embed  fs.FS
	config WriteFS
	cache  WriteFS
	opts   *Options
}

func NewConfigFS(dir string, sys string) *ConfigFS {
	cfg := &ConfigFS{
		embed: GetSysFS(sys),
	}
	cfg.SetConfigDir(dir)
	cfg.SetCacheDir(DefaultCacheDir())

	log.Println("config dir:", cfg.config)
	log.Println("cache dir:", cfg.cache)

	data, _ := fs.ReadFile(cfg, "options.toml")
	opts, err := LoadOptions(data)
	if err != nil {
		log.Printf("error loading options.toml: %v\n", err)
		data, _ = fs.ReadFile(cfg.embed, "options.toml")
		opts, _ = LoadOptions(data)
	}
	cfg.opts = opts
	cfg.WriteOpts()
	if _, err := os.Stat(filepath.Join(dir, "bindings")); os.IsNotExist(err) {
		cfg.WriteDefaultBindings()
	}
	if _, err := os.Stat(filepath.Join(dir, "lsp.yaml")); os.IsNotExist(err) {
		cfg.WriteDefaultLsp()
	}
	if _, err := os.Stat(filepath.Join(dir, "plugins.yaml")); os.IsNotExist(err) {
		cfg.WriteDefaultPluginManifest()
	}
	if _, err := os.Stat(filepath.Join(dir, "plugins")); os.IsNotExist(err) {
		cfg.WriteDefaultPlugins()
	}
	// TODO: this is a global setting in flare, which is bad
	flare.SetLoader(func(name string) ([]byte, error) {
		return fs.ReadFile(cfg, filepath.Join(highlighterDir, name+".lang"))
	})

	return cfg
}

func (c *ConfigFS) WriteOpts() {
	if c.config != "" {
		t, err := c.opts.ToToml()
		if err != nil {
			log.Printf("Could not write options.toml: %v\n", err)
			return
		}
		err = c.config.WriteFile("options.toml", t, 0666)
		if err != nil {
			log.Printf("Could not write options.toml: %v\n", err)
		}
	}
}

func (c *ConfigFS) WriteDefaultPluginManifest() {
	if c.config == "" {
		return
	}
	plugins, err := fs.ReadFile(c.embed, "plugins.yaml")
	if err != nil {
		log.Println(err)
		return
	}
	c.config.WriteFile("plugins.yaml", plugins, 0666)
}

func (c *ConfigFS) WriteDefaultLsp() {
	if c.config == "" {
		return
	}
	lsp, err := fs.ReadFile(c.embed, "lsp.yaml")
	if err != nil {
		log.Println(err)
		return
	}
	c.config.WriteFile("lsp.yaml", lsp, 0666)
}

func (c *ConfigFS) WriteDefaultPlugins() {
	if c.config != "" {
		os.Mkdir(filepath.Join(c.ConfigDir(), "plugins"), 0750)
		err := fs.WalkDir(c.embed, "plugins", func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				os.Mkdir(filepath.Join(c.ConfigDir(), path), 0750)
				return nil
			}
			data, _ := fs.ReadFile(c.embed, path)
			werr := c.config.WriteFile(path, data, 0666)
			if werr != nil {
				return werr
			}
			return nil
		})
		if err != nil {
			log.Printf("error writing plugins: %v\n", err)
		}
	}
}

func (c *ConfigFS) WriteDefaultBindings() {
	if c.config != "" {
		os.Mkdir(filepath.Join(c.ConfigDir(), "bindings"), 0750)
		err := fs.WalkDir(c.embed, "bindings", func(path string, d fs.DirEntry, err error) error {
			if strings.HasSuffix(path, ".kbd") {
				name := filepath.Base(path)
				data, _ := fs.ReadFile(c.embed, path)
				err := c.config.WriteFile(filepath.Join("bindings", name), data, 0666)
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			log.Printf("error writing bindings: %v\n", err)
		}
	}
}

func (c *ConfigFS) SetConfigDir(dir string) {
	c.config = WriteFS(dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0750)
	}
}

func (c *ConfigFS) SetCacheDir(dir string) {
	c.cache = WriteFS(dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0750)
	}
}

func (c *ConfigFS) ConfigDir() string {
	return string(c.config)
}

func (c *ConfigFS) CacheFS() WriteFS {
	return c.cache
}

func (c *ConfigFS) Open(name string) (f fs.File, err error) {
	if c.config != "" {
		f, err = c.config.Open(name)
		if err == nil {
			return f, nil
		}
	}
	f, err = c.embed.Open(name)
	return f, err
}

func (c *ConfigFS) WalkDir(root string, walkfn fs.WalkDirFunc) error {
	if c.config != "" {
		err := fs.WalkDir(c.config, root, walkfn)
		if err != nil {
			log.Printf("config walkdir error (%s): %v", root, err)
		}
	}
	err := fs.WalkDir(c.embed, root, walkfn)
	if err != nil {
		log.Printf("embed walkdir error (%s): %v", root, err)
	}
	return nil
}
