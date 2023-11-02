package lsp

import (
	"fmt"
	"os"

	"go.lsp.dev/protocol"
)

type ShowFn func(protocol.ShowMessageParams)
type DiagnosticFn func(protocol.PublishDiagnosticsParams)

type Language struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Ft      string
}

type Manager struct {
	servers map[string]*Server

	show       ShowFn
	diagnostic DiagnosticFn

	langs map[string]Language
}

func NewManager(show ShowFn, diagnostic DiagnosticFn, langs map[string]Language) *Manager {
	return &Manager{
		servers:    make(map[string]*Server),
		show:       show,
		diagnostic: diagnostic,
		langs:      langs,
	}
}

func (m *Manager) Open(ft, filename, contents string, version int32) (*Server, error) {
	if m == nil {
		return nil, nil
	}

	l, ok := m.langs[ft]
	if !ok {
		return nil, fmt.Errorf("no lsp for filetype: %s", ft)
	}

	if _, ok := m.servers[l.Ft]; !ok {
		s, err := StartServer(l)
		if err != nil {
			return nil, err
		}
		wd, _ := os.Getwd()
		s.Initialize(wd, m.show, m.diagnostic)
		m.servers[l.Ft] = s
	}

	s := m.servers[l.Ft]
	s.DidOpen(filename, l.Ft, contents, version)

	return m.servers[l.Ft], nil
}
