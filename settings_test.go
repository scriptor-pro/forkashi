package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeSettingsPrecedence(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "90")
	t.Setenv("OKASHI_SMARTQUOTES", "off")
	t.Setenv("OKASHI_AUTHOR", "Env Author")
	t.Setenv("OKASHI_CONTACT", "env@example.com")

	// Env tier only.
	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.Width != 90 || eff.Smartquotes != false || eff.Author != "Env Author" || eff.Contact != "env@example.com" {
		t.Fatalf("env tier: %+v", eff)
	}

	// File tiers override env.
	w, sq := 72, true
	eff = mergeSettings(
		userConfig{Author: "File Author", Contact: "file@x"},
		projectSettings{Width: &w, Smartquotes: &sq},
	)
	if eff.Width != 72 || eff.Smartquotes != true || eff.Author != "File Author" || eff.Contact != "file@x" {
		t.Fatalf("file overrides env: %+v", eff)
	}
}

func TestMergeSettingsDefaultsAndFallthrough(t *testing.T) {
	t.Setenv("OKASHI_WIDTH", "")
	t.Setenv("OKASHI_SMARTQUOTES", "")
	t.Setenv("OKASHI_AUTHOR", "")
	t.Setenv("OKASHI_CONTACT", "")

	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.Width != defaultColumnWidth || eff.Smartquotes != true || eff.Author != "" || eff.Contact != "" {
		t.Fatalf("defaults: %+v", eff)
	}

	// File sets width but not smartquotes → width from file, smartquotes default.
	w := 100
	eff = mergeSettings(userConfig{}, projectSettings{Width: &w})
	if eff.Width != 100 || eff.Smartquotes != true {
		t.Fatalf("partial file: %+v", eff)
	}

	// Explicit smartquotes:false present ≠ nil (proves the pointer distinction).
	f := false
	eff = mergeSettings(userConfig{}, projectSettings{Smartquotes: &f})
	if eff.Smartquotes != false {
		t.Fatalf("explicit false ignored: %+v", eff)
	}

	// Out-of-range file width is clamped.
	huge := 9999
	if eff := mergeSettings(userConfig{}, projectSettings{Width: &huge}); eff.Width != 200 {
		t.Fatalf("clamp: width = %d, want 200", eff.Width)
	}
}

func TestProjectSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w, sq := 80, false
	if err := saveProjectSettings(dir, projectSettings{Width: &w, Smartquotes: &sq}); err != nil {
		t.Fatal(err)
	}
	got := loadProjectSettings(dir)
	if got.Width == nil || *got.Width != 80 || got.Smartquotes == nil || *got.Smartquotes != false {
		t.Fatalf("round-trip: %+v", got)
	}
	// No leftover atomic-write temp file.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != ".okashi.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only .okashi.json, got %v", names)
	}
}

func TestUserConfigRoundTripAndTolerantLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := saveUserConfig(path, userConfig{Author: "Jane", Contact: "line1\nline2"}); err != nil {
		t.Fatal(err)
	}
	if got := loadUserConfig(path); got.Author != "Jane" || got.Contact != "line1\nline2" {
		t.Fatalf("round-trip: %+v", got)
	}
	// Corrupt → zero value, no error/panic.
	if err := os.WriteFile(path, []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := loadUserConfig(path); c.Author != "" || c.Contact != "" {
		t.Fatalf("corrupt should be zero: %+v", c)
	}
	// Missing → zero.
	if c := loadUserConfig(filepath.Join(dir, "nope.json")); c.Author != "" {
		t.Fatal("missing should be zero")
	}
}

func TestResolveSettingsReadsProjectFile(t *testing.T) {
	// Clear width/smartquotes env so the project file is the only non-default source for them.
	t.Setenv("OKASHI_WIDTH", "")
	t.Setenv("OKASHI_SMARTQUOTES", "")
	dir := t.TempDir()
	w := 55
	if err := saveProjectSettings(dir, projectSettings{Width: &w}); err != nil {
		t.Fatal(err)
	}
	if eff := resolveSettings(dir); eff.Width != 55 {
		t.Fatalf("resolveSettings width = %d, want 55", eff.Width)
	}
}

func TestMergeSettingsCover(t *testing.T) {
	uc := userConfig{}
	cover := "cover.jpg"
	ps := projectSettings{Cover: &cover}
	eff := mergeSettings(uc, ps)
	if eff.Cover != "cover.jpg" {
		t.Errorf("Cover = %q, want %q", eff.Cover, "cover.jpg")
	}
}

func TestMergeSettingsCoverUnsetIsEmpty(t *testing.T) {
	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.Cover != "" {
		t.Errorf("Cover with no ps.Cover set = %q, want empty", eff.Cover)
	}
}

func TestMergeSettingsPdfDefaults(t *testing.T) {
	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.PdfFont != "courier" {
		t.Errorf("PdfFont default = %q, want %q", eff.PdfFont, "courier")
	}
	if eff.PdfLineHeight != 24 {
		t.Errorf("PdfLineHeight default = %v, want 24", eff.PdfLineHeight)
	}
	if eff.PdfCharsPerLine != 62 {
		t.Errorf("PdfCharsPerLine default = %v, want 62", eff.PdfCharsPerLine)
	}
	for name, got := range map[string]float64{
		"top": eff.PdfMarginTop, "bottom": eff.PdfMarginBottom,
		"left": eff.PdfMarginLeft, "right": eff.PdfMarginRight,
	} {
		if got != 72 {
			t.Errorf("PdfMargin%s default = %v, want 72", name, got)
		}
	}
}

func TestMergeSettingsPdfFileOverrides(t *testing.T) {
	font := "times"
	lh := 20.0
	top, bottom, left, right := 50.0, 60.0, 80.0, 90.0
	eff := mergeSettings(userConfig{}, projectSettings{
		PdfFont: &font, PdfLineHeight: &lh,
		PdfMarginTop: &top, PdfMarginBottom: &bottom, PdfMarginLeft: &left, PdfMarginRight: &right,
	})
	if eff.PdfFont != "times" || eff.PdfLineHeight != 20 {
		t.Fatalf("font/line height override: %+v", eff)
	}
	if eff.PdfMarginTop != 50 || eff.PdfMarginBottom != 60 || eff.PdfMarginLeft != 80 || eff.PdfMarginRight != 90 {
		t.Fatalf("margin override: %+v", eff)
	}
}

func TestMergeSettingsPdfCharsPerLineOverride(t *testing.T) {
	cpl := 80
	eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cpl})
	if eff.PdfCharsPerLine != 80 {
		t.Fatalf("PdfCharsPerLine override: %+v", eff)
	}
}

func TestMergeSettingsPdfClamping(t *testing.T) {
	lhLow, lhHigh := 1.0, 999.0
	if eff := mergeSettings(userConfig{}, projectSettings{PdfLineHeight: &lhLow}); eff.PdfLineHeight != 10 {
		t.Errorf("line height low clamp = %v, want 10", eff.PdfLineHeight)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfLineHeight: &lhHigh}); eff.PdfLineHeight != 40 {
		t.Errorf("line height high clamp = %v, want 40", eff.PdfLineHeight)
	}

	cplLow, cplHigh := 1, 999
	if eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cplLow}); eff.PdfCharsPerLine != 40 {
		t.Errorf("cpl low clamp = %v, want 40", eff.PdfCharsPerLine)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cplHigh}); eff.PdfCharsPerLine != 120 {
		t.Errorf("cpl high clamp = %v, want 120", eff.PdfCharsPerLine)
	}

	mLow, mHigh := 1.0, 999.0
	if eff := mergeSettings(userConfig{}, projectSettings{PdfMarginTop: &mLow}); eff.PdfMarginTop != 20 {
		t.Errorf("margin low clamp = %v, want 20", eff.PdfMarginTop)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfMarginTop: &mHigh}); eff.PdfMarginTop != 200 {
		t.Errorf("margin high clamp = %v, want 200", eff.PdfMarginTop)
	}
}

func TestProjectSettingsPdfRoundTrip(t *testing.T) {
	dir := t.TempDir()
	font := "times"
	lh := 22.0
	cpl := 70
	top, bottom, left, right := 60.0, 60.0, 90.0, 90.0
	ps := projectSettings{
		PdfFont: &font, PdfLineHeight: &lh, PdfCharsPerLine: &cpl,
		PdfMarginTop: &top, PdfMarginBottom: &bottom, PdfMarginLeft: &left, PdfMarginRight: &right,
	}
	if err := saveProjectSettings(dir, ps); err != nil {
		t.Fatal(err)
	}
	got := loadProjectSettings(dir)
	if got.PdfFont == nil || *got.PdfFont != "times" {
		t.Fatalf("PdfFont round-trip: %+v", got)
	}
	if got.PdfLineHeight == nil || *got.PdfLineHeight != 22 {
		t.Fatalf("PdfLineHeight round-trip: %+v", got)
	}
	if got.PdfCharsPerLine == nil || *got.PdfCharsPerLine != 70 {
		t.Fatalf("PdfCharsPerLine round-trip: %+v", got)
	}
	if got.PdfMarginLeft == nil || *got.PdfMarginLeft != 90 {
		t.Fatalf("PdfMarginLeft round-trip: %+v", got)
	}
}
