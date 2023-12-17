package plugin

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/zyedidia/mu/lua"
	"gopkg.in/yaml.v2"
)

type Info struct {
	Name     string
	Url      string
	Version  string
	Inactive bool
	Custom   string
}

func (i *Info) Installed(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, i.Name))
	return err == nil
}

func (i *Info) Install(dir string) error {
	if i.Custom != "" {
		buf := &bytes.Buffer{}
		cmd := exec.Command("sh", "-c", i.Custom)
		cmd.Stderr = buf
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("%w: %s", err, buf.String())
		}
		return nil
	}

	_, err := git.PlainClone(filepath.Join(dir, i.Name), false, &git.CloneOptions{
		URL: i.Url,
	})
	return err
}

func (i *Info) Load(dir string, lstate *lua.State) error {
	init := filepath.Join(dir, i.Name, "init.lua")
	b, err := os.ReadFile(init)
	if err != nil {
		return err
	}
	log.Println("loading", init)
	return lstate.LoadFile(i.Name, init, b)
}

type Manager struct {
	manifest []Info
	dir      string
	lua      *lua.State
}

func NewManager(dir string) (*Manager, error) {
	var manifest []Info
	plugins, err := os.ReadFile(filepath.Join(dir, "plugins.yaml"))
	if err == nil {
		err = yaml.Unmarshal(plugins, &manifest)
		if err != nil {
			return nil, err
		}
	}

	return &Manager{
		lua:      lua.NewState(dir),
		dir:      filepath.Join(dir, "plugins"),
		manifest: manifest,
	}, nil
}

func (m *Manager) Install(w io.Writer) (err error) {
	for _, i := range m.manifest {
		if !i.Installed(m.dir) {
			fmt.Fprintf(w, "installing %s\n", i.Name)
			e := i.Install(m.dir)
			if e != nil {
				err = e
				fmt.Fprintf(w, "error: %v\n", err)
			}
		}
	}
	return err
}

func (m *Manager) Load() (err error) {
	for _, i := range m.manifest {
		if !i.Inactive {
			e := i.Load(m.dir, m.lua)
			if e != nil {
				err = e
			}
		}
	}
	return err
}
