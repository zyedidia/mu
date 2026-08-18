package main

// DiagnosticType distinguishes errors from warnings.
type DiagnosticType byte

const (
	DiagError   DiagnosticType = iota
	DiagWarning DiagnosticType = iota
)

func (t DiagnosticType) String() string {
	switch t {
	case DiagError:
		return "error"
	case DiagWarning:
		return "warning"
	}
	return ""
}

// Diagnostic represents a diagnostic message from an LSP server or linter.
type Diagnostic struct {
	Line, Col int
	Text      string
	Type      DiagnosticType
}

// AddDiagnostic adds a diagnostic to the buffer.
func (b *Buffer) AddDiagnostic(line, col int, text string, dtype DiagnosticType) {
	b.diagnostics = append(b.diagnostics, Diagnostic{
		Line: line,
		Col:  col,
		Text: text,
		Type: dtype,
	})
}

// ClearDiagnostics removes all diagnostics from the buffer.
func (b *Buffer) ClearDiagnostics() {
	b.diagnostics = b.diagnostics[:0]
	b.lspDiagnostics = b.lspDiagnostics[:0]
}

// GetDiagnosticAt returns the first diagnostic on the given line.
func (b *Buffer) GetDiagnosticAt(line int) (Diagnostic, bool) {
	for _, d := range b.diagnostics {
		if d.Line == line {
			return d, true
		}
	}
	return Diagnostic{}, false
}

// GetDiagnostics returns all diagnostics.
func (b *Buffer) GetDiagnostics() []Diagnostic {
	return b.diagnostics
}
