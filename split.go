package main

// SplitType describes how a node divides its space.
type SplitType byte

const (
	SplitVert  SplitType = iota // children are side-by-side (left|right)
	SplitHoriz                  // children are stacked (top/bottom)
	SplitLeaf                   // no children, holds a pane
)

// SplitNode is a node in the binary split tree. Leaf nodes hold a pane ID.
// Internal nodes have exactly two children.
type SplitNode struct {
	Kind     SplitType
	X, Y     int
	W, H     int
	parent   *SplitNode
	children [2]*SplitNode
	id       uint // meaningful only for leaves
}

var nextSplitID uint = 1

func newLeafNode(x, y, w, h int, parent *SplitNode) *SplitNode {
	id := nextSplitID
	nextSplitID++
	return &SplitNode{
		Kind:   SplitLeaf,
		X:      x,
		Y:      y,
		W:      w,
		H:      h,
		parent: parent,
		id:     id,
	}
}

// NewSplitRoot creates a root leaf node with the given dimensions.
func NewSplitRoot(w, h int) *SplitNode {
	return newLeafNode(0, 0, w, h, nil)
}

// ID returns the pane ID for leaf nodes.
func (n *SplitNode) ID() uint {
	return n.id
}

// IsLeaf returns true if this node has no children.
func (n *SplitNode) IsLeaf() bool {
	return n.Kind == SplitLeaf
}

// VSplit splits this leaf node vertically (side-by-side). Returns the new
// leaf node's ID. The existing content stays on the left.
func (n *SplitNode) VSplit() uint {
	if !n.IsLeaf() {
		return 0
	}
	leftW := n.W / 2
	rightW := n.W - leftW - 1 // -1 for divider

	left := newLeafNode(n.X, n.Y, leftW, n.H, n)
	left.id = n.id // original pane keeps its ID

	right := newLeafNode(n.X+leftW+1, n.Y, rightW, n.H, n)

	n.Kind = SplitVert
	n.children = [2]*SplitNode{left, right}
	n.id = 0

	return right.id
}

// HSplit splits this leaf node horizontally (top/bottom). Returns the new
// leaf node's ID. The existing content stays on top. No gap row is
// reserved — the top pane's status bar acts as the visual divider.
func (n *SplitNode) HSplit() uint {
	if !n.IsLeaf() {
		return 0
	}
	topH := n.H / 2
	botH := n.H - topH

	top := newLeafNode(n.X, n.Y, n.W, topH, n)
	top.id = n.id

	bot := newLeafNode(n.X, n.Y+topH, n.W, botH, n)

	n.Kind = SplitHoriz
	n.children = [2]*SplitNode{top, bot}
	n.id = 0

	return bot.id
}

// Unsplit removes this leaf node from the tree. Its sibling takes over the
// parent's space. Returns the sibling's ID (which becomes active).
func (n *SplitNode) Unsplit() uint {
	if n.parent == nil {
		return n.id // can't unsplit the root
	}
	p := n.parent
	var sibling *SplitNode
	if p.children[0] == n {
		sibling = p.children[1]
	} else {
		sibling = p.children[0]
	}

	// Replace parent with sibling.
	p.Kind = sibling.Kind
	p.id = sibling.id
	p.children = sibling.children
	for i := range p.children {
		if p.children[i] != nil {
			p.children[i].parent = p
		}
	}
	p.Resize(p.W, p.H)

	// Return the deepest left leaf ID.
	return p.firstLeaf().id
}

func (n *SplitNode) firstLeaf() *SplitNode {
	if n.IsLeaf() {
		return n
	}
	return n.children[0].firstLeaf()
}

// GetNode finds the leaf node with the given ID, or nil.
func (n *SplitNode) GetNode(id uint) *SplitNode {
	if n.IsLeaf() {
		if n.id == id {
			return n
		}
		return nil
	}
	if found := n.children[0].GetNode(id); found != nil {
		return found
	}
	return n.children[1].GetNode(id)
}

// Resize sets the node's dimensions and propagates to children.
func (n *SplitNode) Resize(w, h int) {
	n.W = w
	n.H = h
	if n.IsLeaf() {
		return
	}
	switch n.Kind {
	case SplitVert:
		leftW := w / 2
		rightW := w - leftW - 1
		n.children[0].X = n.X
		n.children[0].Y = n.Y
		n.children[0].Resize(leftW, h)
		n.children[1].X = n.X + leftW + 1
		n.children[1].Y = n.Y
		n.children[1].Resize(rightW, h)
	case SplitHoriz:
		topH := h / 2
		botH := h - topH
		n.children[0].X = n.X
		n.children[0].Y = n.Y
		n.children[0].Resize(w, topH)
		n.children[1].X = n.X
		n.children[1].Y = n.Y + topH
		n.children[1].Resize(w, botH)
	}
}

// EachLeaf calls fn for every leaf node in order.
func (n *SplitNode) EachLeaf(fn func(leaf *SplitNode)) {
	if n.IsLeaf() {
		fn(n)
		return
	}
	n.children[0].EachLeaf(fn)
	n.children[1].EachLeaf(fn)
}

// NextLeaf returns the next leaf after the one with the given ID,
// wrapping around.
func (n *SplitNode) NextLeaf(id uint) uint {
	var leaves []uint
	n.EachLeaf(func(l *SplitNode) {
		leaves = append(leaves, l.id)
	})
	for i, lid := range leaves {
		if lid == id {
			return leaves[(i+1)%len(leaves)]
		}
	}
	return id
}

// PrevLeaf returns the previous leaf before the one with the given ID.
func (n *SplitNode) PrevLeaf(id uint) uint {
	var leaves []uint
	n.EachLeaf(func(l *SplitNode) {
		leaves = append(leaves, l.id)
	})
	for i, lid := range leaves {
		if lid == id {
			return leaves[(i-1+len(leaves))%len(leaves)]
		}
	}
	return id
}

// NeighborLeft returns the ID of the nearest leaf to the left of the given
// leaf, or the same ID if none exists.
func (n *SplitNode) NeighborLeft(id uint) uint {
	return n.neighbor(id, func(cur, cand *SplitNode) bool {
		return cand.X+cand.W <= cur.X // candidate is to the left
	}, func(cur, cand *SplitNode) int {
		return cur.X - (cand.X + cand.W) // prefer closest
	})
}

// NeighborRight returns the ID of the nearest leaf to the right.
func (n *SplitNode) NeighborRight(id uint) uint {
	return n.neighbor(id, func(cur, cand *SplitNode) bool {
		return cand.X >= cur.X+cur.W
	}, func(cur, cand *SplitNode) int {
		return cand.X - (cur.X + cur.W)
	})
}

// NeighborUp returns the ID of the nearest leaf above.
func (n *SplitNode) NeighborUp(id uint) uint {
	return n.neighbor(id, func(cur, cand *SplitNode) bool {
		return cand.Y+cand.H <= cur.Y
	}, func(cur, cand *SplitNode) int {
		return cur.Y - (cand.Y + cand.H)
	})
}

// NeighborDown returns the ID of the nearest leaf below.
func (n *SplitNode) NeighborDown(id uint) uint {
	return n.neighbor(id, func(cur, cand *SplitNode) bool {
		return cand.Y >= cur.Y+cur.H
	}, func(cur, cand *SplitNode) int {
		return cand.Y - (cur.Y + cur.H)
	})
}

// neighbor finds the nearest leaf in a given direction. isDir returns true
// if the candidate is in the right direction. dist returns the distance
// (lower is better).
func (n *SplitNode) neighbor(id uint, isDir func(cur, cand *SplitNode) bool, dist func(cur, cand *SplitNode) int) uint {
	cur := n.GetNode(id)
	if cur == nil {
		return id
	}
	bestID := id
	bestDist := -1
	n.EachLeaf(func(leaf *SplitNode) {
		if leaf.id == id {
			return
		}
		if !isDir(cur, leaf) {
			return
		}
		d := dist(cur, leaf)
		if bestDist < 0 || d < bestDist {
			bestDist = d
			bestID = leaf.id
		}
	})
	return bestID
}

