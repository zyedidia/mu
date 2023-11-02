package lsp

import (
	lsp "go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func (s *Server) DidOpen(filename, language, text string, version int32) {
	if s == nil {
		return
	}

	doc := lsp.TextDocumentItem{
		URI:        uri.File(filename),
		LanguageID: lsp.LanguageIdentifier(language),
		Version:    version,
		Text:       text,
	}

	params := lsp.DidOpenTextDocumentParams{
		TextDocument: doc,
	}

	s.lock.Lock()
	go s.sendNotification(lsp.MethodTextDocumentDidOpen, params)
}

func (s *Server) DidSave(filename string) {
	if s == nil {
		return
	}

	doc := lsp.TextDocumentIdentifier{
		URI: uri.File(filename),
	}

	params := lsp.DidSaveTextDocumentParams{
		TextDocument: doc,
	}
	s.lock.Lock()
	go s.sendNotification(lsp.MethodTextDocumentDidSave, params)
}

func (s *Server) DidChange(filename string, version int32, changes []lsp.TextDocumentContentChangeEvent) {
	if s == nil {
		return
	}

	doc := lsp.VersionedTextDocumentIdentifier{
		TextDocumentIdentifier: lsp.TextDocumentIdentifier{
			URI: uri.File(filename),
		},
		Version: version,
	}

	params := lsp.DidChangeTextDocumentParams{
		TextDocument:   doc,
		ContentChanges: changes,
	}
	s.lock.Lock()
	go s.sendNotification(lsp.MethodTextDocumentDidChange, params)
}

func (s *Server) DidClose(filename string) {
	if s == nil {
		return
	}

	doc := lsp.TextDocumentIdentifier{
		URI: uri.File(filename),
	}

	params := lsp.DidCloseTextDocumentParams{
		TextDocument: doc,
	}
	s.lock.Lock()
	go s.sendNotification(lsp.MethodTextDocumentDidClose, params)
}
