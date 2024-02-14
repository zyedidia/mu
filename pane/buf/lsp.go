package buf

import (
	"strings"

	"go.lsp.dev/protocol"
)

func (bp *BufPane) LspHover() error {
	info, err := bp.Lsp.Hover(bp.FullName(), bp.LspPosition(bp.LineColAt(bp.Cursor().Pos)))
	if err != nil {
		return err
	}
	info = strings.ReplaceAll(info, "\n", " ")
	bp.messager.Message(info)
	return nil
}

func (bp *BufPane) LspFormat() error {
	var err error
	var edits []protocol.TextEdit

	c := bp.Cursor()
	if c.HasSelection() {
		edits, err = bp.Lsp.DocumentRangeFormat(bp.FullName(), protocol.Range{
			Start: bp.LspPosition(bp.LineColAt(c.Sel[0])),
			End:   bp.LspPosition(bp.LineColAt(c.Sel[1])),
		}, protocol.FormattingOptions{
			InsertSpaces: false,
			TabSize:      uint32(bp.tabsize),
		})
	} else {
		edits, err = bp.Lsp.DocumentFormat(bp.FullName(), protocol.FormattingOptions{
			InsertSpaces: false,
			TabSize:      uint32(bp.tabsize),
		})
	}

	if err != nil {
		return err
	}

	bp.ApplyLspEdits(edits)
	return nil
}
