package config

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type WriteFS string

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
	embed  embed.FS
	config WriteFS
}

func NewConfigFS(dir string) *ConfigFS {
	cfg := &ConfigFS{
		embed: builtin,
	}
	cfg.SetConfigDir(dir)
	return cfg
}

func (c *ConfigFS) SetConfigDir(dir string) {
	c.config = WriteFS(dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0750)
	}
}

func (c *ConfigFS) ConfigDir() string {
	return string(c.config)
}

func (c *ConfigFS) Open(name string) (f fs.File, err error) {
	if c.config != "" {
		f, err = c.config.Open(name)
		if err == nil {
			return f, nil
		}
	}
	f, err = c.embed.Open(filepath.Join("embed", name))
	return f, err
}

func (c *ConfigFS) WalkDir(root string, walkfn fs.WalkDirFunc) error {
	if c.config != "" {
		err := fs.WalkDir(c.config, root, walkfn)
		if err != nil {
			log.Printf("config walkdir error (%s): %v", root, err)
		}
	}
	err := fs.WalkDir(c.embed, filepath.Join("embed", root), walkfn)
	if err != nil {
		log.Printf("embed walkdir error (%s): %v", root, err)
	}
	return nil
}
