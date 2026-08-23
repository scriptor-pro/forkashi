package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// firstRune returns the first rune of a glyph string (the icon itself, before
// its trailing space). Tests assert codepoints, not glyph literals, so a
// glyph-collapse regression can't make them pass vacuously.
func firstRune(s string) rune { return []rune(s)[0] }

func TestResolveIconsPlainViaEnv(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "plain")
	s := resolveIcons()
	if firstRune(s.folder.ch) != 0x25B8 { // ▸
		t.Fatalf("plain folder rune = U+%04X, want U+25B8 (▸)", firstRune(s.folder.ch))
	}
	// Plain set is monochrome: no per-type color.
	if s.folder.color != "" || s.iconFor(fileEntry{name: "x.md"}).color != "" {
		t.Fatal("plain set must carry no color")
	}
}

func TestIconForGlyphAndColor(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "nerd") // force nerd (empty now auto-detects)
	s := resolveIcons()
	cases := []struct {
		e     fileEntry
		cp    rune
		color lipgloss.Color
	}{
		{fileEntry{name: "..", isDir: true}, 0xF062, iconParentColor}, // nf-fa-arrow_up
		{fileEntry{name: "ch", isDir: true}, 0xF07B, iconFolderColor}, // nf-fa-folder
		{fileEntry{name: "a.md"}, 0xF0F6, iconTextColor},              // nf-fa-file_text_o
		{fileEntry{name: "a.pdf"}, 0xF1C1, iconPdfColor},              // nf-fa-file_pdf_o
		{fileEntry{name: "a.png"}, 0xF1C5, iconImageColor},            // nf-fa-file_image_o
		{fileEntry{name: "a.toml"}, 0xF1C9, iconCodeColor},            // nf-fa-file_code_o
		{fileEntry{name: "a.bin"}, 0xF15B, iconGenericColor},          // nf-fa-file
	}
	for _, c := range cases {
		g := s.iconFor(c.e)
		if firstRune(g.ch) != c.cp || g.color != c.color {
			t.Fatalf("iconFor(%q) = {U+%04X,%v}, want {U+%04X,%v}", c.e.name, firstRune(g.ch), g.color, c.cp, c.color)
		}
	}
	// icon() back-compat returns the glyph string.
	if firstRune(s.icon(fileEntry{name: "a.pdf"})) != 0xF1C1 {
		t.Fatalf("icon() back-compat rune = U+%04X, want U+F1C1", firstRune(s.icon(fileEntry{name: "a.pdf"})))
	}
}

// TestNerdIconsAreRealGlyphs guards against a glyph-collapse regression: every
// glyph in the nerd set must be a real Nerd Font glyph (Private Use Area,
// >= U+E000), never a plain space.
func TestNerdIconsAreRealGlyphs(t *testing.T) {
	t.Setenv("OKASHI_ICONS", "nerd") // force nerd (empty now auto-detects)
	s := resolveIcons()
	all := []glyph{s.folder, s.parent, s.file, s.action, s.scene}
	for _, g := range s.byExt {
		all = append(all, g)
	}
	for _, g := range all {
		if r := firstRune(g.ch); r < 0xE000 {
			t.Fatalf("nerd glyph %q starts with U+%04X, want a PUA glyph (>= U+E000) — collapsed to a plain char?", g.ch, r)
		}
	}
}

func TestIconAutoDetect(t *testing.T) {
	isPlain := func() bool { return firstRune(resolveIcons().folder.ch) == 0x25B8 } // ▸
	// Terminal.app (Menlo, no Nerd Font) → plain.
	t.Setenv("OKASHI_ICONS", "")
	t.Setenv("TERM", "")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	if !isPlain() {
		t.Error("Apple_Terminal should auto-select plain icons")
	}
	// Linux VT console → plain.
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("TERM", "linux")
	if !isPlain() {
		t.Error("TERM=linux should auto-select plain icons")
	}
	// A power-user terminal → nerd.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if isPlain() {
		t.Error("iTerm should keep nerd icons")
	}
	// Override wins both ways.
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("OKASHI_ICONS", "nerd")
	if isPlain() {
		t.Error("OKASHI_ICONS=nerd must force nerd even on Apple_Terminal")
	}
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("OKASHI_ICONS", "plain")
	if !isPlain() {
		t.Error("OKASHI_ICONS=plain must force plain")
	}
}

func TestIconForSceneEntry(t *testing.T) {
	s := resolveIcons()
	g := s.iconFor(fileEntry{name: "aparte.md", isScene: true})
	if g.ch != s.scene.ch {
		t.Fatalf("a scene entry must render the scene glyph, got %+v want %+v", g, s.scene)
	}
}
