package buf

import (
	"errors"
	"fmt"
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
			TabSize:      uint32(bp.IntOpt("tabsize")),
		})
	} else {
		edits, err = bp.Lsp.DocumentFormat(bp.FullName(), protocol.FormattingOptions{
			InsertSpaces: false,
			TabSize:      uint32(bp.IntOpt("tabsize")),
		})
	}

	if err != nil {
		return err
	}

	bp.ApplyLspEdits(edits)
	bp.RecalcVX(bp.Cursor())
	return nil
}

func (bp *BufPane) LspDefinition() error {
	locs, err := bp.Lsp.Definition(bp.FullName(), bp.LspPosition(bp.LineColAt(bp.Cursor().Pos)))
	if err != nil {
		return err
	}
	if len(locs) == 0 {
		return errors.New("no definition found")
	}
	if bp.FullName() != locs[0].URI.Filename() {
		return fmt.Errorf("defined in %s", locs[0].URI.Filename())
	}
	pos := locs[0].Range.Start
	bp.MoveTo(bp.FromLspPosition(pos))
	return nil
}
