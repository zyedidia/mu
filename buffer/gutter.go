package buffer

import (
	"sort"

	"go.lsp.dev/protocol"
)

type DiagnosticType byte

func (t DiagnosticType) String() string {
	switch t {
	case TypeError:
		return "error"
	case TypeWarning:
		return "warning"
	}
	return ""
}

const (
	TypeError = iota
	TypeWarning
)

type Diagnostic struct {
	Line, Col int
	Text      string
	Type      DiagnosticType
}

func (b *Buffer) GetDiagnosticAt(line int) (Diagnostic, bool) {
	for _, d := range b.Diagnostics {
		if d.Line == line {
			return d, true
		}
	}
	return Diagnostic{}, false
}

func (b *Buffer) GetDiagnosticBelow(line int) (Diagnostic, bool) {
	sort.Slice(b.Diagnostics, func(i, j int) bool {
		return b.Diagnostics[i].Line > b.Diagnostics[i].Line
	})
	for _, d := range b.Diagnostics {
		if d.Line < line {
			return d, true
		}
	}
	return Diagnostic{}, false
}

func (b *Buffer) GetDiagnosticAbove(line int) (Diagnostic, bool) {
	if line < 0 {
		return Diagnostic{}, false
	}
	sort.Slice(b.Diagnostics, func(i, j int) bool {
		return b.Diagnostics[i].Line < b.Diagnostics[i].Line
	})
	for _, d := range b.Diagnostics {
		if d.Line > line {
			return d, true
		}
	}
	return Diagnostic{}, false
}

func (b *Buffer) GetDiagnosticLineCol(line, col int) (Diagnostic, bool) {
	for _, d := range b.Diagnostics {
		if d.Line == line && d.Col == col {
			return d, true
		}
	}
	return Diagnostic{}, false
}

func (b *Buffer) ClearDiagnostics() {
	b.Diagnostics = b.Diagnostics[:0]
}

func (b *Buffer) AddLspDiagnostic(r protocol.Range, severity protocol.DiagnosticSeverity, message string) {
	line, col := b.Utf8Loc(int(r.Start.Line), int(r.Start.Character))
	b.Diagnostics = append(b.Diagnostics, Diagnostic{
		Line: line,
		Col:  col,
		Text: message,
		Type: toType(severity),
	})
}

func toType(s protocol.DiagnosticSeverity) DiagnosticType {
	switch s {
	case protocol.DiagnosticSeverityError:
		return TypeError
	default:
		return TypeWarning
	}
}
