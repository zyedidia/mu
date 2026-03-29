package main

import (
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
