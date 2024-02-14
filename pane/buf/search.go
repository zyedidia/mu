package buf

import (
	"errors"
	"fmt"
	"regexp"
)

func (bp *BufPane) FindDown(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := bp.Buffer.FindDown(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

func (bp *BufPane) FindUp(off int, regex string) ([]int, error) {
	r, err := regexp.Compile(regex)
	if err != nil {
		return nil, err
	}
	match := bp.Buffer.FindUp(r, off)
	if len(match) < 1 {
		return nil, fmt.Errorf("no match found")
	}
	return match, nil
}

func (bp *BufPane) Find(search string) error {
	rxp, err := regexp.Compile(search)
	if err != nil {
		return err
	}
	match := bp.Buffer.FindDown(rxp, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		bp.search = rxp
		return nil
	}

	return errors.New("no matches")
}

func (bp *BufPane) FindNext() error {
	if bp.search == nil {
		return errors.New("no search term")
	}

	match := bp.Buffer.FindDown(bp.search, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		return nil
	}
	return errors.New("no matches")
}

func (bp *BufPane) FindPrev() error {
	if bp.search == nil {
		return errors.New("no search term")
	}
	match := bp.Buffer.FindUp(bp.search, bp.Cursor().Pos)
	if match != nil {
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		return nil
	}
	return errors.New("no matches")
}

func (bp *BufPane) FindLiteral(search string) error {
	return bp.Find(regexp.QuoteMeta(search))
}

func (bp *BufPane) FindPrompt() error {
	start := bp.Cursor().Pos
	search, canceled := bp.messager.IPrompt("find", "Find: ", func(cur string) {
		rxp, err := regexp.Compile(cur)
		if err != nil {
			bp.MoveTo(start)
			return
		}
		match := bp.Buffer.FindDown(rxp, start)
		if match != nil {
			bp.MoveTo(match[0])
			bp.SelectTo(match[1])
		} else {
			bp.MoveTo(start)
		}
		bp.RelocateToCur()
	})
	if canceled {
		bp.MoveTo(start)
		return nil
	}
	bp.MoveTo(start)
	return bp.Find(search)
}

func (bp *BufPane) FindLiteralPrompt() error {
	search, canceled := bp.messager.Prompt("find", "Find (no regex): ")
	if canceled {
		return nil
	}
	return bp.FindLiteral(search)
}

func (bp *BufPane) Replace(search, replace string) error {
	repl := []byte(replace)
	re, err := regexp.Compile(search)
	if err != nil {
		return err
	}
	for {
		match := bp.Buffer.FindDown(re, bp.Cursor().Pos)
		if match == nil {
			bp.Deselect()
			return nil
		}
		bp.MoveTo(match[0])
		bp.SelectTo(match[1])
		bp.RelocateToCur()

		resp, canceled := bp.messager.CharPrompt("Replace? (y,n,esc)")
		if canceled {
			bp.Deselect()
			return nil
		}
		if resp == "y" || resp == "Y" {
			bp.Buffer.Replace(re, match, repl)
		}
	}
}

func (bp *BufPane) ReplaceAll(search, replace string) error {
	repl := []byte(replace)
	re, err := regexp.Compile(search)
	if err != nil {
		return err
	}
	searchRange := []int{0, bp.Len()}
	for {
		match := bp.Buffer.FindDown(re, searchRange[0])
		if match == nil {
			return nil
		}
		searchRange[0] = match[0] + bp.Buffer.Replace(re, match, repl)
	}
}
