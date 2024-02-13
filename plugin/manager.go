package plugin

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zyedidia/mu/lua"
	"github.com/zyedidia/mu/pkg/shell"
	"gopkg.in/yaml.v2"
)

func load(module, file string, lstate *lua.State) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	log.Println("loading", file)
	return lstate.LoadFile(module, file, b)
}

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

	stderr := &bytes.Buffer{}
	err := shell.RunWith(fmt.Sprintf("git clone %s %s", i.Url, filepath.Join(dir, i.Name)), nil, io.Discard, stderr, false)
	if err != nil {
		return fmt.Errorf("failed to clone: %s", stderr.String())
	}
	return nil
}

func (i *Info) Load(dir string, lstate *lua.State) error {
	init := filepath.Join(dir, i.Name, "init.lua")
	return load(i.Name, init, lstate)
}

type Manager struct {
	manifest []Info
	dir      string
	base     string
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
		base:     dir,
		manifest: manifest,
	}, nil
}

func (m *Manager) AddPackage(pkg string, vals map[string]any) {
	m.lua.AddPackage(pkg, vals)
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
	e := load("init", filepath.Join(m.base, "init.lua"), m.lua)
	if e != nil {
		err = e
	}
	return err
}
