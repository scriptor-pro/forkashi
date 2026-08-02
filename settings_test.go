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
