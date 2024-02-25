package buf

func (bp *BufPane) LineCol(pos int) []int {
	line, col := bp.LineColAt(pos)
	return []int{line, col}
}

func (bp *BufPane) Offset(line, col int) int {
	return bp.OffsetAt(line, col)
}

func (bp *BufPane) Size() int {
	return int(bp.Buffer.Size())
}

func (bp *BufPane) RelocateToCur() {
	line, col := bp.LineColAt(bp.Cursor().Pos)
	bp.Relocate(bLoc{line, col})
}

func (bp *BufPane) ScrollUp(amt int) {
	bp.topline, bp.topcol = max(0, bp.topline-amt), 0
}

func (bp *BufPane) ScrollDown(amt int) {
	bp.topline, bp.topcol = min(bp.NumLines(), bp.topline+amt), 0
}
