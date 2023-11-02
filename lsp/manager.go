package lsp

import (
	"fmt"
	"os"

	"go.lsp.dev/protocol"
)

type ShowFn func(protocol.ShowMessageParams)
type DiagnosticFn func(protocol.PublishDiagnosticsParams)

type Manager struct {
	servers map[string]*Server

	show       ShowFn
	diagnostic DiagnosticFn
}

func NewManager(show ShowFn, diagnostic DiagnosticFn) *Manager {
	return &Manager{
		servers:    make(map[string]*Server),
		show:       show,
		diagnostic: diagnostic,
	}
}

func (m *Manager) Open(ft, filename, contents string, version int32) (*Server, error) {
	if m == nil {
		return nil, nil
	}

	l, ok := GetLanguage(ft)
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
		m.servers[ft] = s
	}

	s := m.servers[ft]
	s.DidOpen(filename, l.Ft, contents, version)

	return m.servers[l.Ft], nil
}
