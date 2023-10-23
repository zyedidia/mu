package config

import (
	"log"
	"os"
	"path/filepath"
)

func DefaultConfigDir() string {
	// TODO: possible error
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("error finding user home dir: %v", err)
		return ""
	}
	return filepath.Join(home, ".config", "mu")
}

func DefaultCacheDir() string {
	// TODO: possible error
	if xdg := os.Getenv("XDG_CACHE_DIR"); xdg != "" {
		return xdg
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("error finding user home dir: %v", err)
		return ""
	}
	return filepath.Join(home, ".cache", "mu")
}
