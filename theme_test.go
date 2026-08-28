package main

import (
	"math"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestLoadThemeYAML(t *testing.T) {
	data := []byte(`
default:
  fg: '#f8f8f2'
  bg: '#282828'
comment:
  fg: '#75715e'
  bg: '#282828'
keyword:
  fg: '#f92672'
  bg: '#282828'
  attr:
  - bold
`)
	th, err := LoadThemeYAML(data)
	if err != nil {
		t.Fatal(err)
	}

	// Default style.
	def := th.Default()
	if def.Fg.color != tcell.GetColor("#f8f8f2") {
		t.Fatalf("default fg: got %v", def.Fg.color)
	}

	// Named style.
	kw := th.Style("keyword")
	if kw.Attr&AttrBold == 0 {
		t.Fatal("keyword should be bold")
	}

	// Unknown style falls back to default.
	unk := th.Style("nonexistent")
	if unk.Fg.color != def.Fg.color {
		t.Fatal("unknown style should fall back to default")
	}
}

func TestThemeHierarchy(t *testing.T) {
	data := []byte(`
default:
  fg: '#ffffff'
  bg: '#000000'
constant:
  fg: '#ae81ff'
  bg: '#000000'
constant.string:
  fg: '#e6db74'
  bg: '#000000'
`)
	th, err := LoadThemeYAML(data)
	if err != nil {
		t.Fatal(err)
	}

	// Direct match.
	cs := th.Style("constant.string")
	if cs.Fg.color != tcell.GetColor("#e6db74") {
		t.Fatal("constant.string should match directly")
	}

	// Hierarchical fallback: constant.number -> constant.
	cn := th.Style("constant.number")
	if cn.Fg.color != tcell.GetColor("#ae81ff") {
		t.Fatal("constant.number should fall back to constant")
	}
}

func TestThemeMissingDefault(t *testing.T) {
	data := []byte(`
keyword:
  fg: '#ff0000'
`)
	_, err := LoadThemeYAML(data)
	if err == nil {
		t.Fatal("should error without default style")
	}
}

func TestThemeColorPalette(t *testing.T) {
	data := []byte(`
default:
  fg: 15
  bg: 0
`)
	th, err := LoadThemeYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	def := th.Default()
	if def.Fg.color != tcell.PaletteColor(15) {
		t.Fatalf("palette fg: got %v, want PaletteColor(15)", def.Fg.color)
	}
}

func TestLoadEmbeddedTheme(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	th, err := cfg.LoadTheme("monokai")
	if err != nil {
		t.Fatal(err)
	}
	if !th.HasStyle("keyword") {
		t.Fatal("monokai should have keyword style")
	}
}

func TestColorString(t *testing.T) {
	data := []byte(`
default:
  fg: '#ffffff'
  bg: '#000000'
error:
  fg: '#ff0000'
  bg: '#000000'
`)
	th, err := LoadThemeYAML(data)
	if err != nil {
		t.Fatal(err)
	}

	segs := th.ColorString("ok {{error}}ERR{{default}} done", Style{})
	// "ok " | "ERR" | " done" = 3 segments
	if len(segs) != 3 {
		t.Fatalf("segments: got %d, want 3", len(segs))
	}
	if segs[0].Text != "ok " {
		t.Fatalf("segment 0 text: got %q", segs[0].Text)
	}
	if segs[1].Text != "ERR" {
		t.Fatalf("segment 1 text: got %q", segs[1].Text)
	}
	if segs[1].Style.Fg.color != tcell.GetColor("#ff0000") {
		t.Fatal("segment 1 should have error color")
	}
}

// --- Status bar shading ---

// embeddedThemeNames are the themes shipped with the editor, each of which
// the status-bar shading has to look right on without being edited.
var embeddedThemeNames = []string{"monokai", "gruvbox", "solarized", "one-dark", "dracula", "molokai"}

func loadEmbeddedTheme(t *testing.T, name string) *Theme {
	t.Helper()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	th, err := cfg.LoadTheme(name)
	if err != nil {
		t.Fatal(err)
	}
	return th
}

// Every shipped theme gets three distinct status-bar shades, each step
// closer to the editor background than the last, without touching the
// theme file.
func TestStatusShadesStepTowardBackground(t *testing.T) {
	for _, name := range embeddedThemeNames {
		t.Run(name, func(t *testing.T) {
			th := loadEmbeddedTheme(t, name)
			outer, info, fill := statusShades(th)

			dist := func(s Style) float64 {
				l1, ok1 := relativeLuminance(s.Bg)
				l2, ok2 := relativeLuminance(th.Default().Bg)
				if !ok1 || !ok2 {
					t.Fatal("theme has no measurable background")
				}
				return math.Abs(l1 - l2)
			}
			if !(dist(outer) > dist(info) && dist(info) > dist(fill)) {
				t.Fatalf("shades do not step toward the editor background: outer %.3f, info %.3f, fill %.3f",
					dist(outer), dist(info), dist(fill))
			}
			if outer.Bg == info.Bg || info.Bg == fill.Bg {
				t.Fatal("shades are not visually distinct")
			}
		})
	}
}

// The section that carries text keeps a readable contrast on every shipped
// theme, backing the shading off where it would not.
func TestStatusShadesStayReadable(t *testing.T) {
	for _, name := range embeddedThemeNames {
		t.Run(name, func(t *testing.T) {
			th := loadEmbeddedTheme(t, name)
			outer, info, _ := statusShades(th)
			floor := statusMinContrast
			if r := outer.ContrastRatio(); r < floor {
				floor = r
			}
			if got := info.ContrastRatio(); got < floor {
				t.Fatalf("info contrast %.2f:1, want at least %.2f:1", got, floor)
			}
		})
	}
}

// A theme can set the inner shades itself instead of having them derived.
func TestStatusShadesThemeOverride(t *testing.T) {
	th, err := LoadThemeYAML([]byte(`
default:
  fg: '#ffffff'
  bg: '#000000'
statusline:
  fg: '#000000'
  bg: '#ffffff'
statusline.info:
  fg: '#111111'
  bg: '#cccccc'
statusline.fill:
  fg: '#222222'
  bg: '#444444'
`))
	if err != nil {
		t.Fatal(err)
	}
	_, info, fill := statusShades(th)
	if info != th.Style("statusline.info") {
		t.Errorf("info = %v, want the theme's own statusline.info", info)
	}
	if fill != th.Style("statusline.fill") {
		t.Errorf("fill = %v, want the theme's own statusline.fill", fill)
	}
}

// Colors the terminal picks for itself cannot be mixed, so a theme using
// them gets a flat status bar rather than a guessed one.
func TestStatusShadesTerminalDefaultColors(t *testing.T) {
	th, err := LoadThemeYAML([]byte("default:\n  fg: default\n  bg: default\n"))
	if err != nil {
		t.Fatal(err)
	}
	outer, info, fill := statusShades(th)
	if info != outer || fill != outer {
		t.Fatalf("shaded a theme with terminal default colors: %v %v %v", outer, info, fill)
	}
}

func TestStyleBlendBg(t *testing.T) {
	s := Style{Fg: NewHexColor(0x000000), Bg: NewHexColor(0x000000)}
	dst := Style{Fg: NewHexColor(0xffffff), Bg: NewHexColor(0x646464)}

	half := s.BlendBg(dst, 50)
	if r, g, b := half.Bg.TCellColor().RGB(); r != 50 || g != 50 || b != 50 {
		t.Errorf("50%% blend = (%d,%d,%d), want (50,50,50)", r, g, b)
	}
	if half.Fg != s.Fg {
		t.Error("blend changed the foreground")
	}
	if none := s.BlendBg(dst, 0); none.Bg != s.Bg {
		t.Error("0%% blend changed the background")
	}
	if all := s.BlendBg(dst, 100); all.Bg != dst.Bg {
		t.Error("100%% blend did not reach the destination")
	}
}

func TestStyleContrastRatio(t *testing.T) {
	black := NewHexColor(0x000000)
	white := NewHexColor(0xffffff)
	if got := (Style{Fg: black, Bg: white}).ContrastRatio(); math.Abs(got-21) > 0.1 {
		t.Errorf("black on white = %.2f:1, want 21:1", got)
	}
	if got := (Style{Fg: white, Bg: white}).ContrastRatio(); math.Abs(got-1) > 0.001 {
		t.Errorf("white on white = %.2f:1, want 1:1", got)
	}
	// Terminal default colors have nothing to measure.
	if got := (Style{}).ContrastRatio(); got != 1 {
		t.Errorf("default colors = %.2f:1, want 1:1", got)
	}
}

// The inner sections are dark enough that the theme's own status-bar
// foreground — dark, on the light bars most themes use — would read poorly,
// so they take the lighter of the theme's two foregrounds: light text on a
// dark section, as lightline draws its inner sections.
func TestStatusShadesLightTextOnDarkSections(t *testing.T) {
	for _, name := range embeddedThemeNames {
		t.Run(name, func(t *testing.T) {
			th := loadEmbeddedTheme(t, name)
			_, info, fill := statusShades(th)

			for _, s := range []struct {
				name  string
				style Style
			}{{"info", info}, {"fill", fill}} {
				fg, ok1 := relativeLuminance(s.style.Fg)
				bg, ok2 := relativeLuminance(s.style.Bg)
				if !ok1 || !ok2 {
					t.Fatalf("%s section has no measurable colors", s.name)
				}
				if fg <= bg {
					t.Errorf("%s section draws %.3f-luminance text on a %.3f-luminance background, want light on dark",
						s.name, fg, bg)
				}
			}
		})
	}
}

// A light editor background leaves no lighter foreground to switch to, so
// the bar keeps its own and shades only as far as that text stays readable.
func TestStatusShadesLightThemeKeepsForeground(t *testing.T) {
	th, err := LoadThemeYAML([]byte(`
default:
  fg: '#000000'
  bg: '#ffffff'
statusline:
  fg: '#000000'
  bg: '#dddddd'
`))
	if err != nil {
		t.Fatal(err)
	}
	outer, info, fill := statusShades(th)
	if info.Fg != outer.Fg || fill.Fg != outer.Fg {
		t.Fatal("switched to a light foreground on a light theme")
	}
	if info.ContrastRatio() < statusMinContrast {
		t.Fatalf("info contrast %.2f:1, want at least %.2f:1", info.ContrastRatio(), statusMinContrast)
	}
}
