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

// Unbind removes the action for a key sequence, pruning empty trie nodes so
// abandoned prefixes stop shadowing other lookups. Returns whether a binding
// was removed.
func (bt *BindingTrie) Unbind(keys ...string) bool {
	path := make([]*trieNode, 0, len(keys))
	node := bt.root
	for _, k := range keys {
		path = append(path, node)
		child, ok := node.children[k]
		if !ok {
			return false
		}
		node = child
	}
	if !node.hasAction {
		return false
	}
	node.action = nil
	node.hasAction = false
	for i := len(keys) - 1; i >= 0; i-- {
		if node.hasAction || len(node.children) > 0 {
			break
		}
		delete(path[i].children, keys[i])
		node = path[i]
	}
	return true
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
	opCount  int        // count accumulated before an operator or register (multiplies with count)
	register RegisterID // selected register (0 means default)
	regWait  bool       // waiting for register char after "

	pendingOp *PendingOp // operator waiting for motion/textobject

	// forceLinewise is set while executing a linewise operation (dd, yy, dj,
	// V-mode operators) so that register writes are marked linewise even when
	// the affected range has no trailing newline (last line of the file).
	forceLinewise bool

	// blockInsert is set while a visual-block insert (I/A/c) is active: one
	// cursor was spawned per block line, and leaving insert mode collapses
	// them back to the primary cursor.
	blockInsert bool

	// commentPrefix returns the line-comment prefix for a buffer (from the
	// comments.toml config), or "" if unknown. Set by the editor; used by
	// the gc/gcc comment toggle.
	commentPrefix func(b *Buffer) string

	// Macro state (q<reg> / @<reg>).
	macroReg   RegisterID // register being recorded into (0 = not recording)
	macroRec   []string   // keys recorded so far
	macroDepth int        // >0 while replaying a macro (nested @)
	lastMacro  RegisterID // register of the last @<reg> for @@

	// dispatch routes a key the way a real keystroke would be routed
	// (through the editor's infobar/completion checks). Set by the editor;
	// macro replay uses it so recorded ':' and '/' interactions work. When
	// nil, keys go straight to HandleKey.
	dispatch func(key string)

	// charWait is set when an action needs the next keystroke as an argument
	// (e.g. f, t, r). The function is called with the next key.
	charWait func(ks *KeyState, ch string)

	// lastKeys records the keys of the last completed action for . repeat.
	lastKeys []string
	// recording accumulates keys during the current action.
	recording []string
	// replaying is true while . is replaying keys to prevent re-recording.
	replaying bool
	// remapping is true while a user mapping's expansion is replaying: the
	// expansion bypasses the remap layer (noremap semantics) and is not
	// recorded for dot repeat (the mapping's source keys already were).
	remapping bool

	// activeView returns the active view. Set by the editor so that
	// scroll commands (Ctrl-D/Ctrl-U) can adjust the viewport directly.
	activeView func() *View

	// vertical is set by applyMotion for j/k moves to signal that Vx
	// should not be recalculated this cycle.
	vertical bool

	// displayVx is set while cursors' Vx hold row-local display columns
	// from a display-line motion (gj/gk) instead of line-wide visual
	// columns. It is cleared whenever Vx is recalculated.
	displayVx bool

	// marks stores named mark positions (m<char> / '<char> / `<char>).
	marks map[byte]int

	// lastCharSearch stores the last f/t/F/T search for ; and , repeat.
	lastCharSearch charSearch

	// onModeChange is called after every mode switch with the new mode ID.
	onModeChange func(ModeID)

	// onCursorStyle is called when entering/leaving a pending char-wait
	// state (f, t, r, etc.). true = waiting, false = done.
	onCursorStyle func(waiting bool)
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
	prev := ks.mode
	old := ks.modes[prev]
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
	if ks.onModeChange != nil {
		ks.onModeChange(id)
	}

	// Dot repeat: save recorded keys when returning to normal mode.
	if id == ModeNormal && prev != ModeNormal {
		ks.StopRecording()
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
	if n := ks.RawCount(); n > 0 {
		return n
	}
	return 1
}

// RawCount returns the raw count (0 if unset). Counts given before an
// operator or register multiply with counts given after (vim: 2d3w = 6dw).
func (ks *KeyState) RawCount() int {
	if ks.opCount == 0 {
		return ks.count
	}
	if ks.count == 0 {
		return ks.opCount
	}
	return ks.opCount * ks.count
}

// stashCount folds the current count into opCount so that a following count
// multiplies with it. Called when an operator or register prefix is entered.
func (ks *KeyState) stashCount() {
	ks.opCount = ks.RawCount()
	ks.count = 0
}

// ClearCounts resets both count accumulators.
func (ks *KeyState) ClearCounts() {
	ks.count = 0
	ks.opCount = 0
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
	ks.stashCount()
	ks.pendingOp = op
	ks.SetMode(ModeOperatorPending)
}

// WaitForChar sets a callback to receive the next keystroke as a character
// argument (for f, t, r, etc.).
func (ks *KeyState) WaitForChar(fn func(ks *KeyState, ch string)) {
	ks.charWait = fn
	if ks.onCursorStyle != nil {
		ks.onCursorStyle(true)
	}
}

// ResetAction clears the accumulated count, register, pending operator, and
// returns to normal mode if in operator-pending.
func (ks *KeyState) ResetAction() {
	ks.ClearCounts()
	ks.register = 0
	ks.pendingOp = nil
	if ks.mode == ModeOperatorPending {
		ks.SetMode(ModeNormal) // StopRecording called by SetMode
	} else if ks.mode == ModeNormal {
		ks.StopRecording()
	}
	// In normal mode, ensure cursor is not sitting on a newline.
	if ks.mode == ModeNormal {
		b := ks.Buf()
		for i := range b.cursors {
			b.cursors[i] = b.cursors[i].VimClamp(b)
		}
	}
}

// RecordMacroKey appends a key to an active macro recording. HandleKey
// calls it for every key it processes; the editor calls it directly for
// keys consumed elsewhere (infobar prompts, completion menus). Keys from
// macro replays and mapping expansions are not re-recorded.
func (ks *KeyState) RecordMacroKey(key string) {
	if ks.macroReg != 0 && ks.macroDepth == 0 && !ks.replaying && !ks.remapping {
		ks.macroRec = append(ks.macroRec, key)
	}
}

// dispatchKey routes one key as the editor would route a real keystroke.
func (ks *KeyState) dispatchKey(key string) {
	if ks.dispatch != nil {
		ks.dispatch(key)
	} else {
		ks.HandleKey(key)
	}
}

// HandleKey processes a single key event through the vim state machine.
func (ks *KeyState) HandleKey(key string) {
	ks.RecordMacroKey(key)

	// Dot repeat recording: start a fresh recording on the first key of a
	// normal-mode action (no pending state yet). Once started, keep
	// appending until the action completes.
	if !ks.replaying && !ks.remapping {
		if ks.mode == ModeNormal && ks.recording == nil && ks.pendingOp == nil && len(ks.keys) == 0 && !ks.regWait {
			ks.recording = []string{}
		}
		if ks.recording != nil {
			ks.recording = append(ks.recording, key)
		}
	}

	// If waiting for a character argument (f, t, r, etc.), deliver it.
	if ks.charWait != nil {
		fn := ks.charWait
		ks.charWait = nil
		if ks.onCursorStyle != nil {
			ks.onCursorStyle(false)
		}
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

	// Try to accumulate as count digit (only in normal/visual/operator-pending, before any trie keys).
	if len(ks.keys) == 0 && ks.mode != ModeInsert && ks.mode != ModeReplace && ks.tryCount(key) {
		return
	}

	// Try register prefix (in normal and visual modes, before any trie keys).
	if len(ks.keys) == 0 && key == "\"" && ks.register == 0 &&
		(ks.mode == ModeNormal || ks.modes[ks.mode].IsVisual) {
		ks.stashCount()
		ks.regWait = true
		return
	}

	// Accumulate key for trie lookup.
	ks.keys = append(ks.keys, key)

	action, result := ks.lookupSeq(mode, ks.keys)

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
		keys := make([]string, len(ks.keys))
		copy(keys, ks.keys)
		ks.keys = ks.keys[:0]
		if len(keys) > 1 {
			ks.dispatchDead(mode, keys)
			return
		}
		// Call mode's default key handler.
		if mode.OnKey != nil {
			mode.OnKey(ks, key)
		} else {
			// No handler; reset action state.
			ks.ResetAction()
		}
	}
}

// lookupSeq resolves a key sequence: user remaps take priority over default
// bindings, and a partial remap waits for more keys even when a default
// binding already matches (the dead-sequence fallback in dispatchDead runs
// the default if the remap never completes). While a mapping's expansion is
// replaying, the remap layer is skipped so expansions keep their default
// meaning.
func (ks *KeyState) lookupSeq(mode *Mode, keys []string) (KeyAction, TrieResult) {
	if !ks.remapping && mode.Remaps != nil {
		if action, res := mode.Remaps.Lookup(keys); res != TrieNone {
			return action, res
		}
	}
	return mode.Bindings.Lookup(keys)
}

// matchSeq resolves a key sequence to an exact match only: a user remap
// match wins, else a default binding match. Unlike lookupSeq, a remap prefix
// does not block the default binding — used once a sequence is known dead,
// where waiting on the remap is pointless.
func (ks *KeyState) matchSeq(mode *Mode, keys []string) (KeyAction, bool) {
	if !ks.remapping && mode.Remaps != nil {
		if action, res := mode.Remaps.Lookup(keys); res == TrieMatch {
			return action, true
		}
	}
	if action, res := mode.Bindings.Lookup(keys); res == TrieMatch {
		return action, true
	}
	return nil, false
}

// dispatchDead resolves a multi-key sequence that matched no binding: the
// longest matching prefix runs and the remaining keys are dispatched again
// (possibly in a new mode the prefix switched to). Without this, a user
// mapping that shadows a shorter default binding (e.g. mapping "dx" while
// "d" is an operator) would swallow the default. If no prefix matches, the
// first key goes to the mode's fallback handler and the rest are
// re-dispatched.
func (ks *KeyState) dispatchDead(mode *Mode, keys []string) {
	n := len(keys) - 1
	for ; n >= 1; n-- {
		if action, ok := ks.matchSeq(mode, keys[:n]); ok {
			action(ks)
			break
		}
	}
	if n < 1 {
		n = 1
		if mode.OnKey != nil {
			mode.OnKey(ks, keys[0])
		} else {
			ks.ResetAction()
		}
	}
	// Re-dispatch the tail. Pop it from the dot-repeat and macro
	// recordings first, since HandleKey will record those keys again.
	rest := keys[n:]
	if !ks.replaying && !ks.remapping {
		if ks.recording != nil {
			if cut := len(ks.recording) - len(rest); cut >= 0 {
				ks.recording = ks.recording[:cut]
			}
		}
		if ks.macroReg != 0 && ks.macroDepth == 0 {
			if cut := len(ks.macroRec) - len(rest); cut >= 0 {
				ks.macroRec = ks.macroRec[:cut]
			}
		}
	}
	for _, k := range rest {
		ks.HandleKey(k)
	}
}

// replayKeys feeds a mapping's expansion through the dispatcher. Remapping
// is disabled during the replay so the expansion has its default meaning
// (vim noremap semantics), which also makes mutual mappings ("map 0 ^",
// "map ^ 0") safe.
func (ks *KeyState) replayKeys(keys []string) {
	saved := ks.remapping
	ks.remapping = true
	for _, k := range keys {
		ks.HandleKey(k)
	}
	ks.remapping = saved
}

// StopRecording saves the accumulated keys as the last action for dot repeat.
func (ks *KeyState) StopRecording() {
	if ks.replaying {
		return
	}
	if len(ks.recording) > 0 {
		ks.lastKeys = make([]string, len(ks.recording))
		copy(ks.lastKeys, ks.recording)
	}
	ks.recording = nil
}

// Replay executes the last recorded action. The recorded keys are the raw
// keys the user typed, so mappings apply to them again — even when the
// replay itself was triggered from inside a mapping expansion.
func (ks *KeyState) Replay() {
	if len(ks.lastKeys) == 0 || ks.replaying {
		return
	}
	ks.replaying = true
	savedRemap := ks.remapping
	ks.remapping = false
	for _, key := range ks.lastKeys {
		ks.HandleKey(key)
	}
	ks.remapping = savedRemap
	ks.replaying = false
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

// charSearch stores the last f/t/F/T search for ; and , repeat.
type charSearch struct {
	fn      func(b *Buffer, c Cursor, count int, ch rune) int
	reverse func(b *Buffer, c Cursor, count int, ch rune) int
	ch      rune
	flags   MotionFlags
}

// ActiveView returns the active view via the callback. Returns nil if unset.
func (ks *KeyState) ActiveView() *View {
	if ks.activeView != nil {
		return ks.activeView()
	}
	return nil
}

// ensureLineVx converts cursors' Vx back to line-wide visual columns after a
// display-line motion chain (gj/gk store row-local columns there). Call
// before any code that reads Vx as a line-wide column.
func (ks *KeyState) ensureLineVx() {
	if !ks.displayVx {
		return
	}
	b := ks.Buf()
	for i := range b.cursors {
		b.cursors[i].Vx = b.VisualCol(b.cursors[i].Pos)
	}
	ks.displayVx = false
}
