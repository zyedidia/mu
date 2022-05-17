package config

import (
	"embed"
	"io/fs"
	"log"
	"path/filepath"
)

type ConfigFS struct {
	embed  embed.FS
	config fs.FS
}

func (c *ConfigFS) Open(name string) (f fs.File, err error) {
	if c.config != nil {
		f, err = c.config.Open(name)
		if err == nil {
			return f, nil
		}
	}
	f, err = c.embed.Open(filepath.Join("embed", name))
	return f, err
}

func (c *ConfigFS) WalkDir(root string, walkfn fs.WalkDirFunc) error {
	if c.config != nil {
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
