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

// --- Buffer search methods (work directly on the reader) ---

// FindDown finds the first match at or after off, wrapping to the start if
// needed. Returns the submatch indices (adjusted to absolute offsets), or nil.
func (b *Buffer) FindDown(re *regexp.Regexp, off int) []int {
	sr := io.NewSectionReader(b, int64(off), int64(b.Len()-off))
	br := bufio.NewReader(sr)

	loc := re.FindReaderSubmatchIndex(br)
	if loc == nil && off != 0 {
		// Wrap to start.
		return b.FindDown(re, 0)
	}
	for i := range loc {
		loc[i] += off
	}
	return loc
}

// FindUp finds the last match before off, wrapping to the end if needed.
// Returns the submatch indices (adjusted to absolute offsets), or nil.
func (b *Buffer) FindUp(re *regexp.Regexp, off int) []int {
	sr := io.NewSectionReader(b, 0, int64(off))
	br := bufio.NewReader(sr)
	var last []int
	var start int
	for {
		match := re.FindReaderSubmatchIndex(br)
		if match == nil {
			break
		}
		if last == nil {
			last = make([]int, len(match))
		}
		for i := range match {
			last[i] = start + match[i]
		}
		next := start + match[1]
		if next >= off {
			break
		}
		sr = io.NewSectionReader(b, int64(next), int64(off-next))
		br = bufio.NewReader(sr)
		start = next
	}
	if last == nil && off != b.Len() {
		// Wrap to end.
		return b.FindUp(re, b.Len())
	}
	return last
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
// and highlights the match.
func (e *Editor) incrementalSearch(input string, origPos int, dir int) {
	b := e.ActiveView().buf
	v := e.ActiveView()
	if input == "" {
		*b.Cursor() = b.Cursor().MoveTo(origPos)
		v.Highlight = [2]int{}
		return
	}
	re, err := regexp.Compile(input)
	if err != nil {
		*b.Cursor() = b.Cursor().MoveTo(origPos)
		v.Highlight = [2]int{}
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
		*b.Cursor() = b.Cursor().MoveTo(origPos)
		v.Highlight = [2]int{}
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
	re, err := regexp.Compile(pattern)
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
	re, err := regexp.Compile(pattern)
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
	re, err := regexp.Compile(pattern)
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

	re, err := regexp.Compile(pattern)
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
	for {
		sr := io.NewSectionReader(b, int64(off), int64(b.Len()-off))
		br := bufio.NewReader(sr)
		loc := re.FindReaderSubmatchIndex(br)
		if loc == nil {
			break
		}
		for i := range loc {
			loc[i] += off
		}
		n := b.Replace(re, loc, replacement)
		off = loc[0] + n
		count++
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
	b := e.ActiveView().buf

	sr := io.NewSectionReader(b, int64(off), int64(b.Len()-off))
	br := bufio.NewReader(sr)
	loc := re.FindReaderSubmatchIndex(br)
	if loc == nil {
		e.subDone(re, count)
		return
	}
	for i := range loc {
		loc[i] += off
	}

	*b.Cursor() = b.Cursor().MoveTo(loc[0])
	matched := string(b.Slice(loc[0], loc[1]))

	e.infobar.Prompt(fmt.Sprintf("Replace \"%s\"? (y/n/q/a)", matched), func(key string) {
		switch key {
		case "y":
			n := b.Replace(re, loc, replacement)
			e.subStep(re, replacement, loc[0]+n, count+1)
		case "n":
			e.subStep(re, replacement, loc[1], count)
		case "a":
			n := b.Replace(re, loc, replacement)
			newOff := loc[0] + n
			newCount := count + 1
			// Replace all remaining without asking.
			for {
				sr2 := io.NewSectionReader(b, int64(newOff), int64(b.Len()-newOff))
				br2 := bufio.NewReader(sr2)
				rloc := re.FindReaderSubmatchIndex(br2)
				if rloc == nil {
					break
				}
				for i := range rloc {
					rloc[i] += newOff
				}
				n = b.Replace(re, rloc, replacement)
				newOff = rloc[0] + n
				newCount++
			}
			e.subDone(re, newCount)
		default: // q, Escape, anything else
			e.subDone(re, count)
		}
	})
}

func (e *Editor) subDone(re *regexp.Regexp, count int) {
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
		b := e.ActiveView().buf
		if e.useIncSearch() {
			origPos := b.Cursor().Pos
			e.infobar.StartPromptIncremental("/",
				func(input string) { e.incrementalSearch(input, origPos, 1) },
				func(input string) { e.clearSearchHighlight(); e.finalizeSearch(input, 1) },
				func() { e.clearSearchHighlight(); *b.Cursor() = b.Cursor().MoveTo(origPos) },
			)
		} else {
			e.infobar.StartPrompt("/", func(input string) {
				e.searchForward(input)
			})
		}
	}, "/")

	// ?: search backward
	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		b := e.ActiveView().buf
		if e.useIncSearch() {
			origPos := b.Cursor().Pos
			e.infobar.StartPromptIncremental("?",
				func(input string) { e.incrementalSearch(input, origPos, -1) },
				func(input string) { e.clearSearchHighlight(); e.finalizeSearch(input, -1) },
				func() { e.clearSearchHighlight(); *b.Cursor() = b.Cursor().MoveTo(origPos) },
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
	}, "*")

	e.ks.modes[ModeNormal].Bindings.Bind(func(ks *KeyState) {
		word := e.wordUnderCursor()
		if word != "" {
			e.searchBackward(`\b` + regexp.QuoteMeta(word) + `\b`)
		}
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
