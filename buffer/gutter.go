package buffer

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

func (b *Buffer) GetDiagnosticLineCol(line, col int) (Diagnostic, bool) {
	for _, d := range b.Diagnostics {
		if d.Line == line && d.Col == col {
			return d, true
		}
	}
	return Diagnostic{}, false
}
