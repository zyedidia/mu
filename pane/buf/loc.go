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
	topl, _ := bp.LineColAt(bp.stpos)
	topl = max(0, topl-amt)
	bp.stpos = bp.OffsetAt(topl, 0)
}

func (bp *BufPane) ScrollDown(amt int) {
	topl, _ := bp.LineColAt(bp.stpos)
	topl = min(bp.NumLines(), topl+amt)
	bp.stpos = bp.OffsetAt(topl, 0)
}
