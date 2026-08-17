package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SearchState tracks the current search pattern and direction.
type SearchState struct {
	pattern   string
	direction int // 1 = forward, -1 = backward
	re        *regexp.Regexp
}

// --- Buffer search methods ---

// compileSearch compiles a search pattern with multi-line mode enabled so
// that ^ and $ anchor to line boundaries, as in vim.
func compileSearch(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?m)" + pattern)
}

// findAllInLine returns all matches of re within line l, with indices
// converted to absolute buffer offsets. Running the regex on the isolated
// line keeps ^ and $ anchored to the real line boundaries.
func (b *Buffer) findAllInLine(re *regexp.Regexp, l int) [][]int {
	ls := b.OffsetAt(l, 0)
	if ls >= b.Len() && b.Len() > 0 {
		// The phantom position after a trailing newline is not a line
		// (vim: no match for ^ there).
		return nil
	}
	matches := re.FindAllSubmatchIndex(b.GetLine(l), -1)
	for _, m := range matches {
		for i := range m {
			if m[i] >= 0 {
				m[i] += ls
			}
		}
	}
	return matches
}

// FindDown finds the first match at or after off, wrapping to the start if
// needed. Returns the submatch indices (adjusted to absolute offsets), or nil.
func (b *Buffer) FindDown(re *regexp.Regexp, off int) []int {
	loc := b.findDownFrom(re, off)
	if loc == nil && off != 0 {
		// Wrap to start.
		return b.findDownFrom(re, 0)
	}
	return loc
}

// findDownFrom finds the first match at or after off without wrapping.
func (b *Buffer) findDownFrom(re *regexp.Regexp, off int) []int {
	if off > b.Len() {
		return nil
	}
	// Search the remainder of the line containing off line-locally, so a
	// mid-line starting offset can't produce a false ^ anchor.
	l, _ := b.LineColAt(off)
	for _, m := range b.findAllInLine(re, l) {
		if m[0] >= off {
			return m
		}
	}
	// Search the rest of the buffer with the streaming reader. The section
	// begins at a true line start, so (?m) anchors line up. If the next
	// line start is at (or past) EOF there is nothing left to search —
	// scanning an empty section would let ^ falsely match at EOF.
	next := b.OffsetAt(l+1, 0)
	if next >= b.Len() || next <= off {
		return nil
	}
	sr := io.NewSectionReader(b, int64(next), int64(b.Len()-next))
	br := bufio.NewReader(sr)
	loc := re.FindReaderSubmatchIndex(br)
	if loc == nil {
		return nil
	}
	for i := range loc {
		if loc[i] >= 0 {
			loc[i] += next
		}
	}
	return loc
}

// FindUp finds the last match beginning before off, wrapping to the end if
// needed. Returns the submatch indices (adjusted to absolute offsets), or nil.
func (b *Buffer) FindUp(re *regexp.Regexp, off int) []int {
	loc := b.findUpFrom(re, off)
	if loc == nil && off != b.Len() {
		// Wrap to end.
		return b.findUpFrom(re, b.Len())
	}
	return loc
}

// findUpFrom scans lines backward from off without wrapping, returning the
// last match that begins before off.
func (b *Buffer) findUpFrom(re *regexp.Regexp, off int) []int {
	if off > b.Len() {
		off = b.Len()
	}
	startLine, _ := b.LineColAt(off)
	for l := startLine; l >= 0; l-- {
		var last []int
		for _, m := range b.findAllInLine(re, l) {
			if m[0] < off {
				last = m
			}
		}
		if last != nil {
			return last
		}
	}
	return nil
}

// advancePast returns the next search offset after a match at loc that was
// replaced by n bytes (n < 0 means not replaced), guaranteeing progress on
// empty matches.
func (b *Buffer) advancePast(loc []int, n int) int {
	var off int
	if n >= 0 {
		off = loc[0] + n
	} else {
		off = loc[1]
	}
	if loc[1] == loc[0] {
		// Empty match: step one rune forward so the loop can't stall.
		_, sz := b.DecodeRuneAt(off)
		if sz == 0 {
			return b.Len() + 1 // past the end: terminates the caller's loop
		}
		off += sz
	}
	return off
}

// Replace replaces the match at loc with repl, expanding submatch references
// ($1, $2, etc.). Returns the number of bytes inserted.
func (b *Buffer) Replace(re *regexp.Regexp, loc []int, repl string) int {
	if len(loc) < 2 {
		return 0
	}
	dst := expand(re, nil, repl, b, loc)
	b.DoEdit(&Edit{
		Start: loc[0],
		End:   loc[1],
		Text:  dst,
	})
	return len(dst)
}

// ReplaceLiteral replaces the match at loc with repl without expanding
// submatch references.
func (b *Buffer) ReplaceLiteral(loc []int, repl []byte) int {
	if len(loc) < 2 {
		return 0
	}
	b.DoEdit(&Edit{
		Start: loc[0],
		End:   loc[1],
		Text:  repl,
	})
	return len(repl)
}

// --- Submatch expansion (adapted from regexp package for Slicer) ---

func expand(re *regexp.Regexp, dst []byte, template string, src interface{ Slice(int, int) []byte }, match []int) []byte {
	for len(template) > 0 {
		i := strings.Index(template, "$")
		if i < 0 {
			break
		}
		dst = append(dst, template[:i]...)
		template = template[i:]
		if len(template) > 1 && template[1] == '$' {
			dst = append(dst, '$')
			template = template[2:]
			continue
		}
		name, num, rest, ok := extractSubmatch(template)
		if !ok {
			dst = append(dst, '$')
			template = template[1:]
			continue
		}
		template = rest
		if num >= 0 {
			if 2*num+1 < len(match) && match[2*num] >= 0 {
				dst = append(dst, src.Slice(match[2*num], match[2*num+1])...)
			}
		} else {
			for i, namei := range re.SubexpNames() {
				if name == namei && 2*i+1 < len(match) && match[2*i] >= 0 {
					dst = append(dst, src.Slice(match[2*i], match[2*i+1])...)
					break
				}
			}
		}
	}
	dst = append(dst, template...)
	return dst
}

func extractSubmatch(str string) (name string, num int, rest string, ok bool) {
	if len(str) < 2 || str[0] != '$' {
		return
	}
	brace := false
	if str[1] == '{' {
		brace = true
		str = str[2:]
	} else {
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		r, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		return
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			return
		}
		i++
	}
	num = 0
	for j := 0; j < len(name); j++ {
		if name[j] < '0' || '9' < name[j] || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[j]) - '0'
	}
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}
	rest = str[i:]
	ok = true
	return
}

// --- Editor search methods ---

// incrementalSearch moves the cursor to the nearest match as the user types
// and highlights the match. When the pattern is empty or matches nothing,
// the cursor and viewport return to their pre-search state.
func (e *Editor) incrementalSearch(input string, origPos int, origView Viewport, dir int) {
	b := e.ActiveView().buf
	v := e.ActiveView()
	restore := func() {
		*b.Cursor() = b.Cursor().MoveTo(origPos)
		v.SetViewport(origView)
		v.Highlight = [2]int{}
	}
	if input == "" {
		restore()
		return
	}
	re, err := compileSearch(input)
	if err != nil {
		restore()
		return
	}
	var loc []int
	if dir > 0 {
		loc = b.FindDown(re, origPos)
	} else {
		loc = b.FindUp(re, origPos)
	}
	if loc != nil {
		*b.Cursor() = b.Cursor().MoveTo(loc[0])
		v.Highlight = [2]int{loc[0], loc[1]}
	} else {
		restore()
	}
}

// clearSearchHighlight removes the incremental search highlight.
func (e *Editor) clearSearchHighlight() {
	if v := e.ActiveView(); v != nil {
		v.Highlight = [2]int{}
	}
}

// finalizeSearch saves the search state so n/N work, without moving the
// cursor (which is already at the match from incremental search).
func (e *Editor) finalizeSearch(pattern string, dir int) {
	if pattern == "" {
		return
	}
	re, err := compileSearch(pattern)
	if err != nil {
		e.infobar.Error(fmt.Sprintf("Invalid pattern: %v", err))
		return
	}
	e.search = SearchState{pattern: pattern, direction: dir, re: re}
}

func (e *Editor) searchForward(pattern string) {
	if pattern == "" {
		return
	}
	re, err := compileSearch(pattern)
	if err != nil {
		e.infobar.Error(fmt.Sprintf("Invalid pattern: %v", err))
		return
	}
	e.search = SearchState{pattern: pattern, direction: 1, re: re}
	e.findNext(1)
}

func (e *Editor) searchBackward(pattern string) {
	if pattern == "" {
		return
	}
	re, err := compileSearch(pattern)
	if err != nil {
		e.infobar.Error(fmt.Sprintf("Invalid pattern: %v", err))
		return
	}
	e.search = SearchState{pattern: pattern, direction: -1, re: re}
	e.findNext(-1)
}

func (e *Editor) searchNext() {
	if e.search.re == nil {
		e.infobar.Error("No previous search")
		return
	}
	e.findNext(e.search.direction)
}

func (e *Editor) searchPrev() {
	if e.search.re == nil {
		e.infobar.Error("No previous search")
		return
	}
	e.findNext(-e.search.direction)
}

func (e *Editor) findNext(dir int) {
	b := e.ActiveView().buf
	pos := b.Cursor().Pos

	var loc []int
	if dir > 0 {
		loc = b.FindDown(e.search.re, pos+1)
		if loc != nil && loc[0] <= pos {
			e.infobar.Message("search wrapped")
		}
	} else {
		loc = b.FindUp(e.search.re, pos)
		if loc != nil && loc[0] >= pos {
			e.infobar.Message("search wrapped")
		}
	}

	if loc == nil {
		e.infobar.Error(fmt.Sprintf("Pattern not found: %s", e.search.pattern))
		return
	}

	*b.Cursor() = b.Cursor().MoveTo(loc[0])
}

// --- Substitute ---

// cmdSubstitute replaces matches of a pattern in the entire buffer.
// Usage: substitute <pattern> <replacement> [all]
// By default it is interactive (y/n/q/a). Pass "all" to replace everything.
func cmdSubstitute(e *Editor, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("substitute: usage: substitute <pattern> <replacement> [all]")
	}
	pattern := args[0]
	replacement := args[1]
	replaceAll := len(args) >= 3 && args[2] == "all"

	re, err := compileSearch(pattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %v", err)
	}

	e.search = SearchState{pattern: pattern, direction: 1, re: re}

	if replaceAll {
		e.substituteAll(re, replacement)
	} else {
		e.substituteInteractive(re, replacement)
	}
	return nil
}

func (e *Editor) substituteAll(re *regexp.Regexp, replacement string) {
	b := e.ActiveView().buf
	b.UndoBarrier()

	count := 0
	off := 0
	for off <= b.Len() {
		loc := b.findDownFrom(re, off)
		if loc == nil {
			break
		}
		n := b.Replace(re, loc, replacement)
		count++
		off = b.advancePast(loc, n)
	}

	if count == 0 {
		e.infobar.Error(fmt.Sprintf("Pattern not found: %s", re.String()))
	} else {
		e.infobar.Message(fmt.Sprintf("%d substitution(s)", count))
	}
}

func (e *Editor) substituteInteractive(re *regexp.Regexp, replacement string) {
	b := e.ActiveView().buf
	b.UndoBarrier()
	e.subStep(re, replacement, 0, 0)
}

// subStep finds the next match from off and prompts the user. It chains
// via the Prompt callback so the event loop drives each step.
func (e *Editor) subStep(re *regexp.Regexp, replacement string, off, count int) {
	v := e.ActiveView()
	b := v.buf

	loc := b.findDownFrom(re, off)
	if loc == nil {
		e.subDone(re, count)
		return
	}

	*b.Cursor() = b.Cursor().MoveTo(loc[0])
	v.Highlight = [2]int{loc[0], loc[1]}
	matched := string(b.Slice(loc[0], loc[1]))

	e.infobar.Prompt(fmt.Sprintf("Replace \"%s\"? (y/n/q/a)", matched), func(key string) {
		switch key {
		case "y":
			n := b.Replace(re, loc, replacement)
			e.subStep(re, replacement, b.advancePast(loc, n), count+1)
		case "n":
			e.subStep(re, replacement, b.advancePast(loc, -1), count)
		case "a":
			n := b.Replace(re, loc, replacement)
			newOff := b.advancePast(loc, n)
			newCount := count + 1
			// Replace all remaining without asking.
			for newOff <= b.Len() {
				rloc := b.findDownFrom(re, newOff)
				if rloc == nil {
					break
				}
				n = b.Replace(re, rloc, replacement)
				newCount++
				newOff = b.advancePast(rloc, n)
			}
			e.subDone(re, newCount)
		default: // q, Escape, anything else
			e.subDone(re, count)
		}
	})
}

func (e *Editor) subDone(re *regexp.Regexp, count int) {
	e.ActiveView().Highlight = [2]int{}
	if count == 0 {
		e.infobar.Error(fmt.Sprintf("Pattern not found: %s", re.String()))
	} else {
		e.infobar.Message(fmt.Sprintf("%d substitution(s)", count))
	}
}

// --- Binding registration ---

const incSearchMaxSize = 10 * 1024 * 1024 // 10MB

func (e *Editor) useIncSearch() bool {
	inc, ok := GetOptBool(e.config.opts.top, "incsearch")
	if ok && !inc {
		return false
	}
	if e.ActiveView() != nil && e.ActiveView().buf.Len() > incSearchMaxSize {
		return false
	}
	return true
}

func (e *Editor) registerSearchBindings() {
	// /: search forward
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		v := e.ActiveView()
		b := v.buf
		if e.useIncSearch() {
			origPos := b.Cursor().Pos
			origView := v.Viewport()
			e.infobar.StartPromptIncremental("/",
				func(input string) { e.incrementalSearch(input, origPos, origView, 1) },
				func(input string) { e.clearSearchHighlight(); e.finalizeSearch(input, 1) },
				func() {
					// Cancel: put both the cursor and the viewport back
					// exactly where they were.
					e.clearSearchHighlight()
					*b.Cursor() = b.Cursor().MoveTo(origPos)
					v.SetViewport(origView)
				},
			)
		} else {
			e.infobar.StartPrompt("/", func(input string) {
				e.searchForward(input)
			})
		}
	}, "/")

	// ?: search backward
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		v := e.ActiveView()
		b := v.buf
		if e.useIncSearch() {
			origPos := b.Cursor().Pos
			origView := v.Viewport()
			e.infobar.StartPromptIncremental("?",
				func(input string) { e.incrementalSearch(input, origPos, origView, -1) },
				func(input string) { e.clearSearchHighlight(); e.finalizeSearch(input, -1) },
				func() {
					e.clearSearchHighlight()
					*b.Cursor() = b.Cursor().MoveTo(origPos)
					v.SetViewport(origView)
				},
			)
		} else {
			e.infobar.StartPrompt("?", func(input string) {
				e.searchBackward(input)
			})
		}
	}, "?")

	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		count := ks.Count()
		for i := 0; i < count; i++ {
			e.searchNext()
		}
		ks.ResetAction()
	}, "n")

	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		count := ks.Count()
		for i := 0; i < count; i++ {
			e.searchPrev()
		}
		ks.ResetAction()
	}, "N")

	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		word := e.wordUnderCursor()
		if word != "" {
			e.searchForward(`\b` + regexp.QuoteMeta(word) + `\b`)
		}
		ks.ResetAction()
	}, "*")

	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		word := e.wordUnderCursor()
		if word != "" {
			e.searchBackward(`\b` + regexp.QuoteMeta(word) + `\b`)
		}
		ks.ResetAction()
	}, "#")
}

func (e *Editor) wordUnderCursor() string {
	b := e.ActiveView().buf
	c := *b.Cursor()
	start := c.WordStart(b, IsWordChar).Pos
	end := c.WordEnd(b, IsWordChar).Pos
	if end < b.Len() {
		_, _, sz := b.DecodeGraphemeAt(end)
		end += sz
	}
	if start >= end {
		return ""
	}
	return string(b.Slice(start, end))
}
