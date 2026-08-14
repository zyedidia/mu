package main

// RegisterID identifies a vim register.
type RegisterID byte

const (
	RegDefault   RegisterID = '"' // unnamed register (default for d/c/y)
	RegClipboard RegisterID = '+' // system clipboard
	RegYank      RegisterID = '0' // last yank (not delete)
	RegBlackhole RegisterID = '_' // discard
	RegSmallDel  RegisterID = '-' // small delete (less than one line)
)

// Register holds the contents of a vim register.
type Register struct {
	Content  []byte
	Linewise bool // if true, paste inserts on a new line
}

// RegisterSet manages all vim registers.
type RegisterSet struct {
	regs map[RegisterID]Register
}

// NewRegisterSet creates an empty register set.
func NewRegisterSet() *RegisterSet {
	return &RegisterSet{
		regs: make(map[RegisterID]Register),
	}
}

// Get returns the register contents. Returns an empty register if not set.
// Uppercase A-Z reads the corresponding lowercase register (vim: "Ap = "ap).
func (rs *RegisterSet) Get(id RegisterID) Register {
	if id >= 'A' && id <= 'Z' {
		id = RegisterID(id - 'A' + 'a')
	}
	return rs.regs[id]
}

// Set writes to a register, replacing its contents.
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
		rs.regs[lower] = r
		return
	}
	rs.regs[id] = Register{
		Content:  content,
		Linewise: linewise,
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
