package main

import "bytes"

// RegisterID identifies a vim register.
type RegisterID byte

const (
	RegDefault   RegisterID = '"' // unnamed register (default for d/c/y)
	RegClipboard RegisterID = '+' // system clipboard
	RegStar      RegisterID = '*' // alias for the system clipboard register
	RegYank      RegisterID = '0' // last yank (not delete)
	RegBlackhole RegisterID = '_' // discard
	RegSmallDel  RegisterID = '-' // small delete (less than one line)
)

// Register holds the contents of a vim register.
type Register struct {
	Content  []byte
	Linewise bool // if true, paste inserts on a new line
	// Block marks blockwise-visual content: Content holds one row per
	// block line (joined with '\n') and paste inserts them as a rectangle.
	Block bool
	// BlockWidth is the display width of the block, used to pad rows when
	// pasting with a count.
	BlockWidth int
}

// RegisterSet manages all vim registers.
type RegisterSet struct {
	regs map[RegisterID]Register

	// ReadClip/WriteClip connect the '+'/'*' registers to the system
	// clipboard (nil leaves them as ordinary internal registers). They are
	// set by the editor according to the clipboard option.
	ReadClip  func() ([]byte, bool)
	WriteClip func(data []byte) bool
}

// NewRegisterSet creates an empty register set.
func NewRegisterSet() *RegisterSet {
	return &RegisterSet{
		regs: make(map[RegisterID]Register),
	}
}

// normalizeReg maps register aliases: uppercase A-Z to their lowercase
// register and '*' to the clipboard register '+'.
func normalizeReg(id RegisterID) RegisterID {
	if id >= 'A' && id <= 'Z' {
		return RegisterID(id - 'A' + 'a')
	}
	if id == RegStar {
		return RegClipboard
	}
	return id
}

// storeClipboard caches externally received clipboard content in the '+'
// register without echoing it back through WriteClip. Foreign content is
// charwise, or linewise if it ends with a newline (as in vim).
func (rs *RegisterSet) storeClipboard(data []byte) {
	rs.regs[RegClipboard] = Register{
		Content:  data,
		Linewise: len(data) > 0 && data[len(data)-1] == '\n',
	}
}

// Get returns the register contents. Returns an empty register if not set.
// Uppercase A-Z reads the corresponding lowercase register (vim: "Ap = "ap).
// The clipboard register consults the system clipboard: if its content
// still matches what mu last wrote there, the stored register (with its
// linewise/blockwise type) is used; otherwise the external content wins.
func (rs *RegisterSet) Get(id RegisterID) Register {
	id = normalizeReg(id)
	if id == RegClipboard && rs.ReadClip != nil {
		if data, ok := rs.ReadClip(); ok {
			if !bytes.Equal(rs.regs[RegClipboard].Content, data) {
				rs.storeClipboard(data)
			}
			return rs.regs[RegClipboard]
		}
	}
	return rs.regs[id]
}

// Set writes to a register, replacing its contents. Writes to the
// clipboard register also go to the system clipboard.
func (rs *RegisterSet) Set(id RegisterID, content []byte, linewise bool) {
	if id == RegBlackhole {
		return
	}
	// Uppercase A-Z appends to the corresponding lowercase register.
	if id >= 'A' && id <= 'Z' {
		lower := RegisterID(id - 'A' + 'a')
		r := rs.regs[lower]
		r.Content = append(r.Content, content...)
		// If either part is linewise the result is linewise, and linewise
		// content always ends with a newline.
		r.Linewise = r.Linewise || linewise
		if r.Linewise && (len(r.Content) == 0 || r.Content[len(r.Content)-1] != '\n') {
			r.Content = append(r.Content, '\n')
		}
		// Appending charwise/linewise content converts a block register.
		r.Block = false
		r.BlockWidth = 0
		rs.regs[lower] = r
		return
	}
	id = normalizeReg(id)
	rs.regs[id] = Register{
		Content:  content,
		Linewise: linewise,
	}
	if id == RegClipboard && rs.WriteClip != nil {
		rs.WriteClip(content)
	}
}

// SetBlock writes blockwise content to a register. Uppercase A-Z appends the
// rows as additional block rows to the corresponding lowercase register.
// Writes to the clipboard register also go to the system clipboard.
func (rs *RegisterSet) SetBlock(id RegisterID, content []byte, width int) {
	if id == RegBlackhole {
		return
	}
	if id >= 'A' && id <= 'Z' {
		lower := RegisterID(id - 'A' + 'a')
		r := rs.regs[lower]
		if len(r.Content) > 0 {
			r.Content = append(r.Content, '\n')
		}
		r.Content = append(r.Content, content...)
		r.Linewise = false
		r.Block = true
		if width > r.BlockWidth {
			r.BlockWidth = width
		}
		rs.regs[lower] = r
		return
	}
	id = normalizeReg(id)
	rs.regs[id] = Register{
		Content:    content,
		Block:      true,
		BlockWidth: width,
	}
	if id == RegClipboard && rs.WriteClip != nil {
		rs.WriteClip(content)
	}
}

// SetDefaultBlock writes blockwise content to the unnamed register and also
// updates the yank register if isYank is true.
func (rs *RegisterSet) SetDefaultBlock(content []byte, width int, isYank bool) {
	rs.SetBlock(RegDefault, content, width)
	if isYank {
		rs.SetBlock(RegYank, content, width)
	}
}

// SetDefault writes to the unnamed register and also updates the yank
// register if isYank is true.
func (rs *RegisterSet) SetDefault(content []byte, linewise, isYank bool) {
	rs.Set(RegDefault, content, linewise)
	if isYank {
		rs.Set(RegYank, content, linewise)
	}
}
