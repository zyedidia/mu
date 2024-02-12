package mu

import (
	"fmt"

	"github.com/zyedidia/clipper"
)

func (e *Editor) setOpt(name string, val interface{}) error {
	if e.config.IsGlobalOpt(name) {
		err := e.config.SetGlobalOpt(name, val)
		if err != nil {
			return err
		}
		switch name {
		case "theme":
			th, err := e.config.LoadTheme(val.(string))
			if err != nil {
				return err
			}
			e.theme = th
		case "clipboard":
			if err := e.setClipboard(val.(string)); err != nil {
				return err
			}
		case "cursor":
			if e.SetCursor == nil {
				return fmt.Errorf("unable to set cursor style")
			}
			if err := e.SetCursor(val.(string)); err != nil {
				return err
			}
		}
		return nil
	}
	return e.ActivePane().SetOpt(name, val)
}

func (e *Editor) setClipboard(clip string) error {
	switch clip {
	case "external":
		c, err := clipper.GetClipboard(clipper.Clipboards...)
		if err == nil {
			e.clipboard = c
		} else {
			return fmt.Errorf("error loading external clipboard: %v\n", err)
		}
	case "terminal":
		if e.termclip == nil {
			return fmt.Errorf("terminal clipboard is unavailable")
		}
	case "internal":
		e.clipboard = &clipper.Internal{}
	default:
		return fmt.Errorf("%s: invalid clipboard type", clip)
	}
	e.clipboard.Init()
	return nil

}
