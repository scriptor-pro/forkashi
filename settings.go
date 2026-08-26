package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// userConfig is personal, machine-global identity (stored in the OS user-config dir under
// okashi/config.json — macOS: ~/Library/Application Support/okashi; Linux: ~/.config/okashi —
// alongside recent.json) used on the export title page. Applies to every project.
type userConfig struct {
	Author      string `json:"author,omitempty"`
	Contact     string `json:"contact,omitempty"`
	FirstLaunch string `json:"firstLaunch,omitempty"` // "2006-01-02"; seeds the quote-of-the-day cycle
}

// projectSettings are per-project editor preferences in <dir>/.okashi.json. Pointer fields
// distinguish "unset" (nil → fall through to env/default) from an explicit value (e.g.
// smartquotes:false is not the same as omitted).
type projectSettings struct {
	Width       *int    `json:"width,omitempty"`
	Smartquotes *bool   `json:"smartquotes,omitempty"`
	Cover       *string `json:"cover,omitempty"`

	// PDF Manuscript-style typography (Manuscript only; Tufte is unaffected). All nil =
	// today's hardcoded defaults, so an unconfigured project renders identically.
	PdfFont         *string  `json:"pdfFont,omitempty"`         // "courier" | "times"
	PdfLineHeight   *float64 `json:"pdfLineHeight,omitempty"`   // points
	PdfCharsPerLine *int     `json:"pdfCharsPerLine,omitempty"` // Courier only; ignored for Times
	PdfMarginTop    *float64 `json:"pdfMarginTop,omitempty"`    // points
	PdfMarginBottom *float64 `json:"pdfMarginBottom,omitempty"` // points
	PdfMarginLeft   *float64 `json:"pdfMarginLeft,omitempty"`   // points; ignored for Courier (derived from CPL)
	PdfMarginRight  *float64 `json:"pdfMarginRight,omitempty"`  // points; ignored for Courier (derived from CPL)
}

// effectiveSettings is the resolved result after overlaying defaults ← env ← file, per field.
type effectiveSettings struct {
	Author, Contact string
	Width           int
	Smartquotes     bool
	Cover           string

	PdfFont                       string
	PdfLineHeight                 float64
	PdfCharsPerLine               int
	PdfMarginTop, PdfMarginBottom float64
	PdfMarginLeft, PdfMarginRight float64
}

// userConfigPath is the personal config path, or "" if there is no usable config dir.
func userConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "okashi", "config.json")
}

// loadUserConfig reads the personal config; missing/corrupt/empty-path → zero value (tolerant,
// mirroring recent.json).
func loadUserConfig(path string) userConfig {
	var c userConfig
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	_ = json.Unmarshal(data, &c)
	return c
}

// saveUserConfig writes the personal config atomically, creating the config dir if needed. Guards
// the no-config-dir case (path == "") so it never touches the cwd (mirrors loadUserConfig).
func saveUserConfig(path string, c userConfig) error {
	if path == "" {
		return errors.New("no user config directory available")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicWrite(path, data, 0o644)
}

// projectSettingsPath is <dir>/.okashi.json (a dotfile — already excluded from the file pane).
func projectSettingsPath(dir string) string {
	return filepath.Join(dir, ".okashi.json")
}

// loadProjectSettings reads <dir>/.okashi.json; missing/corrupt → zero value (all-nil).
func loadProjectSettings(dir string) projectSettings {
	var s projectSettings
	data, err := os.ReadFile(projectSettingsPath(dir))
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// saveProjectSettings writes <dir>/.okashi.json atomically.
func saveProjectSettings(dir string, s projectSettings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(projectSettingsPath(dir), data, 0o644)
}

// clampWidth constrains a column width to the supported [20,200] range.
func clampWidth(n int) int {
	if n < 20 {
		return 20
	}
	if n > 200 {
		return 200
	}
	return n
}

const (
	defaultPdfFont         = "courier"
	defaultPdfLineHeight   = 24.0
	defaultPdfCharsPerLine = 62
	defaultPdfMargin       = 72.0
)

// clampPdfLineHeight constrains PDF line height to the supported [10,40]pt range.
func clampPdfLineHeight(f float64) float64 {
	if f < 10 {
		return 10
	}
	if f > 40 {
		return 40
	}
	return f
}

// clampPdfCharsPerLine constrains chars-per-line to the supported [40,120] range.
func clampPdfCharsPerLine(n int) int {
	if n < 40 {
		return 40
	}
	if n > 120 {
		return 120
	}
	return n
}

// clampPdfMargin constrains a PDF margin to the supported [20,200]pt range.
func clampPdfMargin(f float64) float64 {
	if f < 20 {
		return 20
	}
	if f > 200 {
		return 200
	}
	return f
}

// mergeSettings overlays defaults ← env ← file per field. Split from IO so the precedence logic is
// unit-testable with constructed inputs (env is controlled via the process environment).
func mergeSettings(uc userConfig, ps projectSettings) effectiveSettings {
	eff := effectiveSettings{
		Author:      os.Getenv("OKASHI_AUTHOR"),
		Contact:     os.Getenv("OKASHI_CONTACT"),
		Width:       defaultColumnWidth,
		Smartquotes: true,

		PdfFont:         defaultPdfFont,
		PdfLineHeight:   defaultPdfLineHeight,
		PdfCharsPerLine: defaultPdfCharsPerLine,
		PdfMarginTop:    defaultPdfMargin,
		PdfMarginBottom: defaultPdfMargin,
		PdfMarginLeft:   defaultPdfMargin,
		PdfMarginRight:  defaultPdfMargin,
	}
	if w, ok := resolveColumnWidthEnv(); ok {
		eff.Width = w
	}
	if sq, ok := resolveSmartQuotesEnv(); ok {
		eff.Smartquotes = sq
	}
	if uc.Author != "" {
		eff.Author = uc.Author
	}
	if uc.Contact != "" {
		eff.Contact = uc.Contact
	}
	if ps.Width != nil {
		eff.Width = clampWidth(*ps.Width)
	}
	if ps.Smartquotes != nil {
		eff.Smartquotes = *ps.Smartquotes
	}
	if ps.Cover != nil {
		eff.Cover = *ps.Cover
	}
	if ps.PdfFont != nil {
		eff.PdfFont = *ps.PdfFont
	}
	if ps.PdfLineHeight != nil {
		eff.PdfLineHeight = clampPdfLineHeight(*ps.PdfLineHeight)
	}
	if ps.PdfCharsPerLine != nil {
		eff.PdfCharsPerLine = clampPdfCharsPerLine(*ps.PdfCharsPerLine)
	}
	if ps.PdfMarginTop != nil {
		eff.PdfMarginTop = clampPdfMargin(*ps.PdfMarginTop)
	}
	if ps.PdfMarginBottom != nil {
		eff.PdfMarginBottom = clampPdfMargin(*ps.PdfMarginBottom)
	}
	if ps.PdfMarginLeft != nil {
		eff.PdfMarginLeft = clampPdfMargin(*ps.PdfMarginLeft)
	}
	if ps.PdfMarginRight != nil {
		eff.PdfMarginRight = clampPdfMargin(*ps.PdfMarginRight)
	}
	return eff
}

// resolveSettings is the effective settings for dir (personal config ← per-project file over env).
func resolveSettings(dir string) effectiveSettings {
	return mergeSettings(loadUserConfig(userConfigPath()), loadProjectSettings(dir))
}
