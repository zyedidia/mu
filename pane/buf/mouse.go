package buf

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zyedidia/mu/buffer"
)

var mouseRe = regexp.MustCompile(`\{\d+ \d+\}`)

func parseMouse(loc string) (int, int, error) {
	if !mouseRe.MatchString(loc) {
		return 0, 0, fmt.Errorf("invalid mouse location: %s", loc)
	}
	parts := strings.Split(loc[1:len(loc)-1], " ")
	x, _ := strconv.Atoi(parts[0])
	y, _ := strconv.Atoi(parts[1])
	return x, y, nil
}

func (bp *BufPane) MouseClick(loc string) error {
	x, y, err := parseMouse(loc)
	if err != nil {
		return err
	}
	line, col := bp.MouseLoc(x, y)
	off := bp.OffsetAt(line, col)

	if bp.mouse.click && bp.mouse.last != off {
		bp.mouse.drag = true
	}

	if bp.mouse.drag {
		bp.SelectWithModeTo(off)
	} else {
		if !bp.mouse.click && bp.mouse.last == off && time.Since(bp.mouse.clicktm) < mouseClickThreshold {
			if bp.mouse.double {
				bp.mouse.triple = true
			} else {
				bp.mouse.double = true
			}
		} else {
			bp.Cursor().SelectMode(buffer.SelectChar)
			bp.mouse.double = false
			bp.mouse.triple = false
		}

		bp.MoveTo(off)
		if bp.mouse.triple {
			bp.Cursor().SelectMode(buffer.SelectLine)
			bp.SelectWithModeTo(off)
		} else if bp.mouse.double {
			bp.Cursor().SelectMode(buffer.SelectWord)
			bp.SelectWithModeTo(off)
		}

		bp.mouse.clicktm = time.Now()
		bp.mouse.click = true
	}

	bp.mouse.last = off
	return nil
}

func (bp *BufPane) MouseRelease(loc string) error {
	bp.mouse.click = false
	bp.mouse.drag = false
	return nil
}
