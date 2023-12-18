package term

import "github.com/zyedidia/mu/pkg/theme"

func (tp *TermPane) DisplayStatus(draw func(x, y int, mainc rune, combc []rune, style theme.Style), w int, theme *theme.Theme) bool {
	return false
}
