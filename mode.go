package main

// ModeID identifies a vim editing mode.
type ModeID byte

const (
	ModeNormal          ModeID = iota
	ModeInsert          ModeID = iota
	ModeReplace         ModeID = iota
	ModeVisual          ModeID = iota
	ModeVisualLine      ModeID = iota
	ModeVisualBlock     ModeID = iota
	ModeOperatorPending ModeID = iota
	ModeCommand         ModeID = iota
)

// Mode defines a vim editing mode with its key bindings and lifecycle hooks.
type Mode struct {
	ID       ModeID
	Name     string
	Bindings *BindingTrie

	// OnEnter is called when entering this mode.
	OnEnter func(ks *KeyState)
	// OnLeave is called when leaving this mode.
	OnLeave func(ks *KeyState)
	// OnKey is called for keys not found in the binding trie.
	OnKey func(ks *KeyState, key string)

	// IsVisual indicates whether this mode maintains a text selection.
	IsVisual bool
}

// InitModes creates all vim modes with empty binding tries. The caller is
// responsible for populating bindings and setting callbacks.
func InitModes() map[ModeID]*Mode {
	return map[ModeID]*Mode{
		ModeNormal: {
			ID:       ModeNormal,
			Name:     "NORMAL",
			Bindings: NewBindingTrie(),
		},
		ModeInsert: {
			ID:       ModeInsert,
			Name:     "INSERT",
			Bindings: NewBindingTrie(),
		},
		ModeReplace: {
			ID:       ModeReplace,
			Name:     "REPLACE",
			Bindings: NewBindingTrie(),
		},
		ModeVisual: {
			ID:       ModeVisual,
			Name:     "VISUAL",
			Bindings: NewBindingTrie(),
			IsVisual: true,
		},
		ModeVisualLine: {
			ID:       ModeVisualLine,
			Name:     "V-LINE",
			Bindings: NewBindingTrie(),
			IsVisual: true,
		},
		ModeVisualBlock: {
			ID:       ModeVisualBlock,
			Name:     "V-BLOCK",
			Bindings: NewBindingTrie(),
			IsVisual: true,
		},
		ModeOperatorPending: {
			ID:       ModeOperatorPending,
			Name:     "OP-PENDING",
			Bindings: NewBindingTrie(),
		},
		ModeCommand: {
			ID:       ModeCommand,
			Name:     "COMMAND",
			Bindings: NewBindingTrie(),
		},
	}
}
