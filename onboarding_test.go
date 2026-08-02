package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsEmptyish(t *testing.T) {
	empty := t.TempDir()
	if !dirIsEmptyish(empty) {
		t.Fatal("fresh temp dir should be empty-ish")
	}
	withDoc := t.TempDir()
	os.WriteFile(filepath.Join(withDoc, "a.md"), []byte("x"), 0o644)
	if dirIsEmptyish(withDoc) {
		t.Fatal("a .md present → not empty-ish")
	}
	withDir := t.TempDir()
	os.MkdirAll(filepath.Join(withDir, "proj"), 0o755)
	if dirIsEmptyish(withDir) {
		t.Fatal("a subdir present → not empty-ish")
	}
	dotOnly := t.TempDir()
	os.WriteFile(filepath.Join(dotOnly, ".hidden"), []byte("x"), 0o644)
	if !dirIsEmptyish(dotOnly) {
		t.Fatal("only dotfiles → empty-ish")
	}
}

func TestMaybeSeedSample(t *testing.T) {
	wd := t.TempDir()
	marker := filepath.Join(t.TempDir(), "cfg", ".seeded")
	maybeSeedSample(wd, marker)
	// Sample seeded.
	man := filepath.Join(wd, "Demo", "the-lighthouse", "manifest.json")
	if _, err := os.Stat(man); err != nil {
		t.Fatalf("manifest should be seeded: %v", err)
	}
	for _, ch := range []string{"01-the-keeper.md", "02-the-fog.md", "03-the-light.md"} {
		if _, err := os.Stat(filepath.Join(wd, "Demo", "the-lighthouse", ch)); err != nil {
			t.Fatalf("chapter %s should be seeded: %v", ch, err)
		}
	}
	// Marker written.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("marker should exist after seeding: %v", err)
	}
	// A second call is a no-op (doesn't error / re-seed differently).
	maybeSeedSample(wd, marker)

	// Non-empty dir + fresh marker → no seed, but marker written.
	wd2 := t.TempDir()
	os.WriteFile(filepath.Join(wd2, "existing.md"), []byte("x"), 0o644)
	marker2 := filepath.Join(t.TempDir(), "cfg", ".seeded")
	maybeSeedSample(wd2, marker2)
	if _, err := os.Stat(filepath.Join(wd2, "Demo", "the-lighthouse")); err == nil {
		t.Fatal("must NOT seed into a non-empty dir")
	}
	if _, err := os.Stat(marker2); err != nil {
		t.Fatal("marker should still be written for a non-empty dir")
	}
}

// The seeded sample manuscript must carry its corkboard synopses (the dotfile sidecar is embedded
// via `all:` and copied by the seeder), so a first-run user sees a populated corkboard.
func TestSeededSampleHasSynopses(t *testing.T) {
	writingDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), ".seeded")
	maybeSeedSample(writingDir, marker)

	proj := filepath.Join(writingDir, "Demo", "the-lighthouse")
	syn := loadSynopses(proj)
	if len(syn) != 3 {
		t.Fatalf("seeded sample should have 3 synopses, got %d (%v)", len(syn), syn)
	}
	for _, f := range []string{"01-the-keeper.md", "02-the-fog.md", "03-the-light.md"} {
		if syn[f] == "" {
			t.Fatalf("missing synopsis for %s", f)
		}
	}
}

func TestFreshWorkspacePrimer(t *testing.T) {
	// Only the seeded Demo/ → a fresh workspace.
	m := model{width: 120, height: 40, homeItems: []homeItem{{kind: homeFolder, label: "Demo", path: "/w/Demo"}}}
	if !m.freshWorkspace() {
		t.Fatal("a workspace with only Demo/ should be fresh")
	}
	out := ansiStrip(m.homeView())
	for _, want := range []string{"manuscrit", "catégorie", "notes", "Demo/"} {
		if !contains(out, want) {
			t.Fatalf("fresh hub should show the model primer (%q missing)", want)
		}
	}
	// Once the writer has their own project, no primer.
	m2 := model{width: 120, height: 40, homeItems: []homeItem{
		{kind: homeFolder, label: "Demo", path: "/w/Demo"},
		{kind: homeProject, label: "My Novel", path: "/w/my-novel"},
	}}
	if m2.freshWorkspace() {
		t.Fatal("a user-created project means the workspace isn't fresh")
	}
	if contains(ansiStrip(m2.homeView()), "for an example") {
		t.Fatal("the primer should disappear once there's user content")
	}
}
