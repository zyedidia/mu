package config

import (
	"errors"
	"io"
)

func (cfs *ConfigFS) CacheCreate(path string) (io.Writer, error) {
	return nil, errors.New("unimplemented")
}

func (cfs *ConfigFS) CacheOpen(path string) (io.Reader, error) {
	return nil, errors.New("unimplemented")
}
