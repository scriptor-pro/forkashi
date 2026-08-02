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
	Author  string `json:"author,omitempty"`
	Contact string `json:"contact,omitempty"`
}

// projectSettings are per-project editor preferences in <dir>/.okashi.json. Pointer fields
// distinguish "unset" (nil → fall through to env/default) from an explicit value (e.g.
// smartquotes:false is not the same as omitted).
type projectSettings struct {
	Width       *int    `json:"width,omitempty"`
	Smartquotes *bool   `json:"smartquotes,omitempty"`
	Cover       *string `json:"cover,omitempty"`
}

// effectiveSettings is the resolved result after overlaying defaults ← env ← file, per field.
type effectiveSettings struct {
	Author, Contact string
	Width           int
	Smartquotes     bool
	Cover           string
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

// mergeSettings overlays defaults ← env ← file per field. Split from IO so the precedence logic is
// unit-testable with constructed inputs (env is controlled via the process environment).
func mergeSettings(uc userConfig, ps projectSettings) effectiveSettings {
	eff := effectiveSettings{
		Author:      os.Getenv("OKASHI_AUTHOR"),
		Contact:     os.Getenv("OKASHI_CONTACT"),
		Width:       defaultColumnWidth,
		Smartquotes: true,
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
	return eff
}

// resolveSettings is the effective settings for dir (personal config ← per-project file over env).
func resolveSettings(dir string) effectiveSettings {
	return mergeSettings(loadUserConfig(userConfigPath()), loadProjectSettings(dir))
}
