package main

// --- Key constants ---

const (
	KeyEscape = "<Esc>"
	KeyEnter  = "<CR>"
	KeyBacksp = "<BS>"
	KeyTab    = "<Tab>"
	KeyDelete = "<Del>"
	KeyUp     = "<Up>"
	KeyDown   = "<Down>"
	KeyLeft   = "<Left>"
	KeyRight  = "<Right>"
	KeyHome   = "<Home>"
	KeyEnd    = "<End>"
	KeyPgUp   = "<PgUp>"
	KeyPgDn   = "<PgDn>"
)

// --- Binding trie ---

// KeyAction is a function invoked when a key binding matches.
type KeyAction func(ks *KeyState)

type trieNode struct {
	children  map[string]*trieNode
	action    KeyAction
	hasAction bool
}

// TrieResult indicates the result of a trie lookup.
type TrieResult int

const (
	TrieNone   TrieResult = iota // no match and not a prefix
	TriePrefix                   // prefix of one or more bindings
	TrieMatch                    // exact match found
)

// BindingTrie is a prefix tree mapping key sequences to actions.
type BindingTrie struct {
	root *trieNode
}

// NewBindingTrie creates an empty binding trie.
func NewBindingTrie() *BindingTrie {
	return &BindingTrie{
		root: &trieNode{children: make(map[string]*trieNode)},
	}
}

// Bind registers a key sequence to an action. The keys are provided as
// individual key strings (e.g. Bind(action, "d", "d") for "dd").
func (bt *BindingTrie) Bind(action KeyAction, keys ...string) {
	node := bt.root
	for _, k := range keys {
		child, ok := node.children[k]
		if !ok {
			child = &trieNode{children: make(map[string]*trieNode)}
			node.children[k] = child
		}
		node = child
	}
	node.action = action
	node.hasAction = true
}

// Lookup searches the trie for the given key sequence. Returns the action
// (if matched) and the result type.
func (bt *BindingTrie) Lookup(keys []string) (KeyAction, TrieResult) {
	node := bt.root
	for _, k := range keys {
		child, ok := node.children[k]
		if !ok {
			return nil, TrieNone
		}
		node = child
	}
	if node.hasAction {
		return node.action, TrieMatch
	}
	if len(node.children) > 0 {
		return nil, TriePrefix
	}
	return nil, TrieNone
}

// HasPrefix returns true if the key sequence is a prefix of any binding.
func (bt *BindingTrie) HasPrefix(keys []string) bool {
	node := bt.root
	for _, k := range keys {
		child, ok := node.children[k]
		if !ok {
			return false
		}
		node = child
	}
	return len(node.children) > 0
}

// --- Pending operator ---

// PendingOp describes an operator waiting for a motion or text object to
// provide a range.
type PendingOp struct {
	Name string
	Key  string // trigger key, used to detect doubled operators (dd, yy)
	// Fn applies the operator to the byte range [start, end).
	Fn func(ks *KeyState, b *Buffer, start, end int)
}

// --- Key state ---

// KeyState manages the vim key dispatch state machine: mode, accumulated
// count, register selection, pending operator, and key buffer.
type KeyState struct {
	buf   *Buffer
	mode  ModeID
	modes map[ModeID]*Mode
	regs  *RegisterSet

	keys     []string   // accumulated key buffer for trie lookup
	count    int        // count prefix (0 means unset)
	register RegisterID // selected register (0 means default)
	regWait  bool       // waiting for register char after "

	pendingOp *PendingOp // operator waiting for motion/textobject

	// charWait is set when an action needs the next keystroke as an argument
	// (e.g. f, t, r). The function is called with the next key.
	charWait func(ks *KeyState, ch string)

	// lastAction records the last completed action for . repeat.
	lastKeys []string

	// halfPageSize returns half the viewport height. Set by the editor
	// so motions like Ctrl-D/Ctrl-U know the page size.
	halfPageSize func() int
}

// NewKeyState creates a new key dispatch state machine.
func NewKeyState(buf *Buffer, regs *RegisterSet) *KeyState {
	ks := &KeyState{
		buf:   buf,
		mode:  ModeNormal,
		modes: InitModes(),
		regs:  regs,
	}
	return ks
}

// Mode returns the current mode.
func (ks *KeyState) Mode() *Mode {
	return ks.modes[ks.mode]
}

// ModeID returns the current mode ID.
func (ks *KeyState) ModeID() ModeID {
	return ks.mode
}

// SetMode switches to a new mode, calling OnLeave and OnEnter hooks.
func (ks *KeyState) SetMode(id ModeID) {
	if ks.mode == id {
		return
	}
	old := ks.modes[ks.mode]
	if old.OnLeave != nil {
		old.OnLeave(ks)
	}
	ks.mode = id
	ks.keys = ks.keys[:0]
	ks.charWait = nil
	nw := ks.modes[id]
	if nw.OnEnter != nil {
		nw.OnEnter(ks)
	}
}

// Buf returns the active buffer.
func (ks *KeyState) Buf() *Buffer {
	return ks.buf
}

// SetBuffer changes the active buffer.
func (ks *KeyState) SetBuffer(b *Buffer) {
	ks.buf = b
}

// Count returns the effective count (1 if unset).
func (ks *KeyState) Count() int {
	if ks.count == 0 {
		return 1
	}
	return ks.count
}

// RawCount returns the raw count (0 if unset).
func (ks *KeyState) RawCount() int {
	return ks.count
}

// Register returns the selected register, or RegDefault if none selected.
func (ks *KeyState) Register() RegisterID {
	if ks.register == 0 {
		return RegDefault
	}
	return ks.register
}

// Pending returns the pending operator, or nil if none.
func (ks *KeyState) Pending() *PendingOp {
	return ks.pendingOp
}

// SetPending sets the pending operator and switches to operator-pending mode.
func (ks *KeyState) SetPending(op *PendingOp) {
	ks.pendingOp = op
	ks.SetMode(ModeOperatorPending)
}

// WaitForChar sets a callback to receive the next keystroke as a character
// argument (for f, t, r, etc.).
func (ks *KeyState) WaitForChar(fn func(ks *KeyState, ch string)) {
	ks.charWait = fn
}

// ResetAction clears the accumulated count, register, pending operator, and
// returns to normal mode if in operator-pending.
func (ks *KeyState) ResetAction() {
	ks.count = 0
	ks.register = 0
	ks.pendingOp = nil
	if ks.mode == ModeOperatorPending {
		ks.SetMode(ModeNormal)
	}
}

// HandleKey processes a single key event through the vim state machine.
func (ks *KeyState) HandleKey(key string) {
	// If waiting for a character argument (f, t, r, etc.), deliver it.
	if ks.charWait != nil {
		fn := ks.charWait
		ks.charWait = nil
		fn(ks, key)
		return
	}

	// If waiting for register character after ".
	if ks.regWait {
		ks.regWait = false
		if len(key) == 1 {
			ks.register = RegisterID(key[0])
		}
		return
	}

	mode := ks.modes[ks.mode]

	// Try to accumulate as count digit (only before any trie keys).
	if len(ks.keys) == 0 && ks.tryCount(key) {
		return
	}

	// Try register prefix (only before any trie keys).
	if len(ks.keys) == 0 && key == "\"" && ks.register == 0 {
		ks.regWait = true
		return
	}

	// Accumulate key for trie lookup.
	ks.keys = append(ks.keys, key)

	action, result := mode.Bindings.Lookup(ks.keys)

	switch result {
	case TrieMatch:
		// Check if this is also a prefix of longer bindings.
		if mode.Bindings.HasPrefix(ks.keys) {
			// Ambiguous: exact match + prefix. Prefer exact match.
			// (In vim, this rarely happens because operators switch modes.)
		}
		keys := make([]string, len(ks.keys))
		copy(keys, ks.keys)
		ks.keys = ks.keys[:0]
		action(ks)
	case TriePrefix:
		// Wait for more keys.
	case TrieNone:
		ks.keys = ks.keys[:0]
		// Call mode's default key handler.
		if mode.OnKey != nil {
			mode.OnKey(ks, key)
		} else {
			// No handler; reset action state.
			ks.ResetAction()
		}
	}
}

// tryCount attempts to consume the key as a count digit. Returns true if
// consumed.
func (ks *KeyState) tryCount(key string) bool {
	if len(key) != 1 {
		return false
	}
	ch := key[0]
	// Can't start count with 0 (that's a motion in normal mode).
	if ks.count == 0 && ch >= '1' && ch <= '9' {
		ks.count = int(ch - '0')
		return true
	}
	if ks.count > 0 && ch >= '0' && ch <= '9' {
		ks.count = ks.count*10 + int(ch-'0')
		return true
	}
	return false
}
