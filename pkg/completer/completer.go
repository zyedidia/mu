package completer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func FileComplete(prefix, wd string) (candidates []string) {
	dslash := ""
	if strings.HasPrefix(prefix, "./") {
		dslash = "./"
		prefix = prefix[2:]
	}

	relpre := filepath.Join(wd, prefix)
	dir := filepath.Dir(relpre)
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		rpath, err := filepath.Rel(wd, path)
		if err != nil {
			return nil
		}
		if strings.HasPrefix(rpath, prefix) {
			if d.IsDir() {
				candidates = append(candidates, dslash+rpath+string(os.PathSeparator))
			} else {
				candidates = append(candidates, dslash+rpath)
			}
		}
		if d.IsDir() && strings.HasSuffix(prefix, "/") && path == relpre {
			return nil
		}
		if d.IsDir() && path != dir {
			return fs.SkipDir
		}
		return nil
	})
	return candidates
}

func GenericComplete(prefix string, list func() []string) (candidates []string) {
	opts := list()
	for _, o := range opts {
		if strings.HasPrefix(o, prefix) {
			candidates = append(candidates, o)
		}
	}
	return candidates
}
