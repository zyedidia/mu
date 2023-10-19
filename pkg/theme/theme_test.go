package theme_test

import (
	"testing"

	"github.com/zyedidia/mu/pkg/theme"
)

func TestThemeYAML(t *testing.T) {
	data := `
default:
    fg: "#646464"
    bg: "#282828"
constant:
    fg: 1
    bg: 2
    attr: ["bold", "underline"]
`

	colors, err := theme.LoadYAML([]byte(data))
	if err != nil {
		t.Fatal(err)
	}

	if colors.Default().Fg.Hex() != 0x646464 {
		t.Fatalf("incorrect hex %x", colors.Default().Fg.Hex())
	}
}
