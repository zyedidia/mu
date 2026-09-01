package main

import "fmt"

// diagnosticItems returns a palette item for every diagnostic across all
// open buffers (not just the current one), each jumping to that
// diagnostic's position on selection.
func (e *Editor) diagnosticItems() []paletteItem {
	var items []paletteItem
	for _, b := range e.buffers {
		buf := b
		for _, d := range buf.GetDiagnostics() {
			d := d
			label := fmt.Sprintf("%s:%d: [%s] %s", bufDisplayName(buf), d.Line+1, d.Type.String(), d.Text)
			items = append(items, paletteItem{label: label, action: func() {
				e.pushJump()
				e.showBuffer(buf)
				// Diagnostics go stale as the buffer is edited; clamp
				// rather than jumping past the end of a shrunken file.
				line := d.Line
				if line > buf.LastLine() {
					line = buf.LastLine()
				}
				pos := buf.OffsetAt(line, 0) + min(d.Col, buf.LineLen(line))
				*buf.Cursor() = buf.Cursor().MoveTo(pos).VimClamp(buf)
			}})
		}
	}
	return items
}

func init() {
	editorCommands = append(editorCommands,
		CommandDef{"lsp-diagnostics", cmdLspDiagnostics, "lsp-diagnostics: list diagnostics across open buffers"},
	)
}

func cmdLspDiagnostics(e *Editor, args []string) error {
	return e.startPalette("diagnostics")
}
