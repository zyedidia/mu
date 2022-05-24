package config

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed embed
var embedded embed.FS

type SysFS string

func (s SysFS) Open(name string) (fs.File, error) {
	if s != "" {
		f, err := os.Open(filepath.Join(string(s), name))
		if err == nil {
			return f, nil
		}
	}
	return embedded.Open(filepath.Join("embed", name))
}

func GetSysFS(dir string) fs.FS {
	if dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return SysFS(dir)
		}
	}
	return SysFS("")
}
