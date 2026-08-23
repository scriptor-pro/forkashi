package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// glyph is a file-pane icon: a Nerd Font (or ASCII) glyph plus its color. The
// ch string includes its own trailing space for alignment. color "" means the
// glyph is rendered uncolored (the plain/ascii set). A plain-space regression
// in the nerd set is caught by TestNerdIconsAreRealGlyphs.
type glyph struct {
	ch    string
	color lipgloss.Color
}

// iconSet is the glyph set for the file pane and launch lists.
type iconSet struct {
	folder, parent, file, action, scene glyph
	byExt                               map[string]glyph
}

// autoIconMode guesses whether the terminal likely has a Nerd Font. We cannot detect
// the font itself, only the terminal — so default to "plain" for the environments that
// ship a non-patched font (Terminal.app's Menlo/SF Mono, the Linux VT console), where
// Nerd Font PUA glyphs render as tofu and their inconsistent width breaks box alignment.
// Power-user terminals (iTerm, WezTerm, kitty, …) keep "nerd"; OKASHI_ICONS overrides.
func autoIconMode() string {
	if os.Getenv("TERM_PROGRAM") == "Apple_Terminal" || os.Getenv("TERM") == "linux" {
		return "plain"
	}
	return "nerd"
}

// resolveIcons picks the glyph set once at startup. OKASHI_ICONS=plain (or ascii) forces
// safe glyphs; =nerd forces Nerd Font glyphs; unset or =auto auto-detects the terminal.
func resolveIcons() iconSet {
	mode := strings.ToLower(os.Getenv("OKASHI_ICONS"))
	if mode == "" || mode == "auto" {
		mode = autoIconMode()
	}
	switch mode {
	case "plain", "ascii":
		return iconSet{
			folder: glyph{ch: "▸ "},
			parent: glyph{ch: "↑ "},
			file:   glyph{ch: "  "},
			action: glyph{ch: "+ "},
			scene:  glyph{ch: "· "},
			byExt:  map[string]glyph{},
		}
	}
	text := glyph{ch: " ", color: iconTextColor} // nf-fa-file_text_o
	img := glyph{ch: " ", color: iconImageColor} // nf-fa-file_image_o
	code := glyph{ch: " ", color: iconCodeColor} // nf-fa-file_code_o
	return iconSet{
		folder: glyph{ch: " ", color: iconFolderColor},  // nf-fa-folder
		parent: glyph{ch: " ", color: iconParentColor},  // nf-fa-arrow_up
		file:   glyph{ch: " ", color: iconGenericColor}, // nf-fa-file
		action: glyph{ch: " ", color: accent},           // nf-fa-plus
		scene:  glyph{ch: " ", color: iconTextColor},    // nf-fa-align_left — a single ordered text, no folder
		byExt: map[string]glyph{
			".md":       text,
			".markdown": text,
			".txt":      text,
			".wg":       text,
			".pdf":      {ch: " ", color: iconPdfColor}, // nf-fa-file_pdf_o
			".png":      img,
			".jpg":      img,
			".jpeg":     img,
			".gif":      img,
			".webp":     img,
			".json":     code,
			".toml":     code,
			".yml":      code,
			".yaml":     code,
			".sh":       code,
		},
	}
}

// iconFor returns the glyph (ch + color) for an entry.
func (s iconSet) iconFor(e fileEntry) glyph {
	switch {
	case e.name == "..":
		return s.parent
	case e.isScene:
		return s.scene
	case e.isDir:
		return s.folder
	}
	if g, ok := s.byExt[strings.ToLower(filepath.Ext(e.name))]; ok {
		return g
	}
	return s.file
}

// icon returns just the glyph string for an entry (back-compat).
func (s iconSet) icon(e fileEntry) string { return s.iconFor(e).ch }
