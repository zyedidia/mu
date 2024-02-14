package buf

func (bp *BufPane) Read(from, to int) string {
	b := make([]byte, to-from)
	n, _ := bp.ReadAt(b, int64(from))
	return string(b[:n])
}

func (bp *BufPane) ReadLine(l int) string {
	return string(bp.GetLine(l))
}

func (bp *BufPane) ReadAll() string {
	return string(bp.Bytes())
}
