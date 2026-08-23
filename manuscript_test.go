package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifestRaw(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkChapterDir(t *testing.T, manuscriptDir, folder string, texts map[string]string) {
	t.Helper()
	chDir := filepath.Join(manuscriptDir, folder)
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range texts {
		if err := os.WriteFile(filepath.Join(chDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResolveManifestBareChaptersNoPart(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "hello world"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"Windermere","items":[
		{"chapter":{"folder":"opening","title":"Chapter One","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceManifest || !v.ordered() {
		t.Fatalf("want manifest source, got %v", v.source)
	}
	if len(v.parts) != 1 || v.parts[0].title != "" {
		t.Fatalf("bare chapters must live under a single synthetic untitled part, got parts=%+v", v.parts)
	}
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "opening" ||
		v.parts[0].chapters[0].title != "Chapter One" {
		t.Fatalf("chapter = %+v", v.parts[0].chapters)
	}
	if len(v.parts[0].chapters[0].texts) != 1 || v.parts[0].chapters[0].texts[0].file != "opening.md" {
		t.Fatalf("texts = %+v", v.parts[0].chapters[0].texts)
	}
}

func TestResolveManifestMixedPartsAndBareChapters(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "prologue", map[string]string{"prologue.md": "x"})
	mkChapterDir(t, dir, "the-letter", map[string]string{"the-letter.md": "y"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"Windermere","items":[
		{"chapter":{"folder":"prologue","title":"Prologue","texts":[{"file":"prologue.md","title":"Prologue"}]}},
		{"part":"Part One","chapters":[
			{"folder":"the-letter","title":"The Letter","texts":[{"file":"the-letter.md","title":"The Letter"}]}
		]}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts) != 2 {
		t.Fatalf("want 2 parts (synthetic + Part One), got %+v", v.parts)
	}
	if v.parts[0].title != "" || len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "prologue" {
		t.Fatalf("part 0 (bare chapters) = %+v", v.parts[0])
	}
	if v.parts[1].title != "Part One" || len(v.parts[1].chapters) != 1 || v.parts[1].chapters[0].folder != "the-letter" {
		t.Fatalf("part 1 = %+v", v.parts[1])
	}
}

func TestResolveManifestMultiTextChapterOrder(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "chapitre-un", map[string]string{
		"scene-ouverture.md":     "a",
		"scene-confrontation.md": "b",
	})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"chapitre-un","title":"Chapitre un","texts":[
			{"file":"scene-confrontation.md","title":"Confrontation"},
			{"file":"scene-ouverture.md","title":"Ouverture"}
		]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	texts := v.parts[0].chapters[0].texts
	if len(texts) != 2 || texts[0].file != "scene-confrontation.md" || texts[1].file != "scene-ouverture.md" {
		t.Fatalf("manifest order must win, got %+v", texts)
	}
}

func TestResolveManifestChapterWithZeroTexts(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "empty-chapter", nil)
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"empty-chapter","title":"Vide","texts":[]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].title != "Vide" {
		t.Fatalf("an empty chapter must still be listed (never auto-removed), got %+v", v.parts)
	}
	if len(v.parts[0].chapters[0].texts) != 0 {
		t.Fatalf("texts = %+v, want empty", v.parts[0].chapters[0].texts)
	}
}

func TestResolveManifestAbsentTextOmitted(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"}) // "gone.md" never written
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[
			{"file":"opening.md","title":"Opening"},{"file":"gone.md","title":"Lost"}
		]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	texts := v.parts[0].chapters[0].texts
	if len(texts) != 1 || texts[0].file != "opening.md" {
		t.Fatalf("a truly-absent text must be omitted, got %+v", texts)
	}
}

func TestResolveManifestAbsentChapterFolderOmitted(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"}) // "gone" folder never created
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[{"file":"opening.md","title":"Opening"}]}},
		{"chapter":{"folder":"gone","title":"Lost","texts":[{"file":"gone.md","title":"Lost"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.parts[0].chapters) != 1 || v.parts[0].chapters[0].folder != "opening" {
		t.Fatalf("a truly-absent chapter folder must be omitted, got %+v", v.parts[0].chapters)
	}
}

func TestResolveManifestUnlistedIsLoose(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "a", map[string]string{"a.md": "x"})
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644) // loose, at manuscript root
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"a","title":"One","texts":[{"file":"a.md","title":"A"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("loose = %+v, want notes.md", v.loose)
	}
}

func TestResolveLegacyFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "02-b.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("z"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceLegacy || !v.ordered() {
		t.Fatalf("numbered files + no manifest -> legacy, got %v", v.source)
	}
	if len(v.parts) != 1 || v.parts[0].title != "" {
		t.Fatalf("legacy chapters must also live under a single synthetic untitled part, got %+v", v.parts)
	}
	chs := v.parts[0].chapters
	if chs[0].folder != "" || len(chs[0].texts) != 1 || chs[0].texts[0].file != "01-a.md" {
		t.Fatalf("legacy chapter 0 = %+v, want folder \"\" (flat file), single text 01-a.md", chs[0])
	}
	if chs[1].texts[0].file != "02-b.md" {
		t.Fatalf("legacy order = %+v, want numeric", chs)
	}
	if chs[0].title != "a" { // de-slugged
		t.Fatalf("legacy title = %q, want de-slugged 'a'", chs[0].title)
	}
	if len(v.loose) != 1 || v.loose[0].name != "notes.md" {
		t.Fatalf("legacy loose = %+v", v.loose)
	}
}

func TestResolveCategory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "on-silence.md"), []byte("x"), 0o644)
	v := resolveManuscript(dir, readEntries(dir))
	if v.source != sourceNone || v.ordered() {
		t.Fatalf("plain folder -> category, got %v", v.source)
	}
}

func TestResolveUnreadableManifestRefuses(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-a.md"), []byte("x"), 0o644) // numbered, but...
	writeManifestRaw(t, dir, `{"schemaVersion":4,"title":"N","items":[]}`)
	v := resolveManuscript(dir, readEntries(dir))
	// Refuse to guess: NOT legacy, files shown flat as loose, warning set.
	if v.source != sourceManifest {
		t.Fatalf("unreadable manifest still marks the folder a manuscript, got %v", v.source)
	}
	if len(v.parts) != 0 {
		t.Fatalf("must not invent parts/chapters from a bad manifest, got %+v", v.parts)
	}
	if v.warning == "" {
		t.Fatal("an unreadable manifest must surface a warning")
	}
	if len(v.loose) != 1 || v.loose[0].name != "01-a.md" {
		t.Fatalf("files shown flat as loose, got %+v", v.loose)
	}
}

func TestIsChapterOf(t *testing.T) {
	dir := t.TempDir()
	mkChapterDir(t, dir, "opening", map[string]string{"opening.md": "x"})
	writeManifestRaw(t, dir, `{"schemaVersion":3,"title":"N","items":[
		{"chapter":{"folder":"opening","title":"One","texts":[{"file":"opening.md","title":"Opening"}]}}
	]}`)
	v := resolveManuscript(dir, readEntries(dir))
	if !isChapterOf(v, "opening") {
		t.Fatal("opening must be recognized as a chapter")
	}
	if isChapterOf(v, "nope") {
		t.Fatal("nope must not be recognized as a chapter")
	}
}
