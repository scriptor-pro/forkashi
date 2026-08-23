package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func downKey() tea.Msg { return tea.KeyMsg{Type: tea.KeyDown} }
func escKey() tea.Msg  { return tea.KeyMsg{Type: tea.KeyEsc} }

func seedExportSelectManuscript(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-1.md"), []byte(strings.Repeat("word ", 30)), 0o644)
	os.WriteFile(filepath.Join(dir, "ch1", "scene-2.md"), []byte("short scene two"), 0o644)
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("A standalone scene's text."), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Research notes, a Resource."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "Chapter One", Texts: []manifestText{
				{File: "scene-1.md", Title: "Opening"},
				{File: "scene-2.md", Title: "Confrontation"},
			}}},
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true,
				Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnterExportSelectBuildsOrderedList(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	if m.screen != screenExportSelect {
		t.Fatalf("screen = %v, want screenExportSelect", m.screen)
	}
	entries := m.exportSelect.entries
	// Order: chapter header, its 2 scenes (indented), the standalone scene, then the Resource.
	if len(entries) != 5 {
		t.Fatalf("want 5 entries (1 header + 2 scenes + 1 standalone scene + 1 resource), got %d: %+v", len(entries), entries)
	}
	if !entries[0].isHeader || entries[0].title != "Chapter One" {
		t.Fatalf("entry 0 = %+v, want chapter header 'Chapter One'", entries[0])
	}
	if entries[1].isHeader || !entries[1].indent || entries[1].title != "Opening" {
		t.Fatalf("entry 1 = %+v, want indented scene 'Opening'", entries[1])
	}
	if entries[2].isHeader || !entries[2].indent || entries[2].title != "Confrontation" {
		t.Fatalf("entry 2 = %+v, want indented scene 'Confrontation'", entries[2])
	}
	if entries[3].isHeader || entries[3].indent || entries[3].title != "Aparté" {
		t.Fatalf("entry 3 = %+v, want non-indented standalone scene 'Aparté'", entries[3])
	}
	if entries[4].isHeader || entries[4].indent || entries[4].title != "notes" {
		t.Fatalf("entry 4 = %+v, want Resource 'notes' (de-slugged filename)", entries[4])
	}
}

func TestEnterExportSelectComputesWordsAndCharsAndPreview(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("one two three"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil {
		t.Fatal("scene entry 'A' not found")
	}
	if sceneEntry.words != 3 {
		t.Fatalf("words = %d, want 3", sceneEntry.words)
	}
	if sceneEntry.chars != charCount("one two three") {
		t.Fatalf("chars = %d, want %d", sceneEntry.chars, charCount("one two three"))
	}
	if sceneEntry.preview != "one two three" {
		t.Fatalf("preview = %q, want %q (shorter than 111 chars, shown whole)", sceneEntry.preview, "one two three")
	}
}

func TestEnterExportSelectPreviewTruncatesAt111CharsAndNormalizesWhitespace(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	long := strings.Repeat("a", 150)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("line one\nline two "+long), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil {
		t.Fatal("scene entry 'A' not found")
	}
	if strings.Contains(sceneEntry.preview, "\n") {
		t.Fatalf("preview must have internal newlines collapsed to spaces, got %q", sceneEntry.preview)
	}
	runes := []rune(sceneEntry.preview)
	if len(runes) > 112 { // 111 chars + possible "…" = 112 runes max
		t.Fatalf("preview too long: %d runes, want <= 112 (111 + ellipsis)", len(runes))
	}
	if !strings.HasSuffix(sceneEntry.preview, "…") {
		t.Fatalf("preview of a text longer than 111 chars must end with an ellipsis, got %q", sceneEntry.preview)
	}
}

func TestEnterExportSelectLoadsExistingExclusions(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("x"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := saveExportSelection(dir, map[string]bool{filepath.Join("ch1", "a.md"): true},
		map[string]bool{filepath.Join("ch1", "a.md"): true}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	var sceneEntry *exportSelectEntry
	for i := range m.exportSelect.entries {
		if m.exportSelect.entries[i].title == "A" {
			sceneEntry = &m.exportSelect.entries[i]
		}
	}
	if sceneEntry == nil || !sceneEntry.excluded {
		t.Fatalf("entry = %+v, want excluded=true (loaded from sidecar)", sceneEntry)
	}
}

func TestExportSelectViewShowsFooterTotals(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("one two three"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "C1", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{width: 80, height: 24}
	m.files.dir = dir
	m.enterExportSelect()
	view := exportSelectView(m)
	if !strings.Contains(view, "3") { // word count
		t.Fatalf("view must show the word total, got:\n%s", view)
	}
}

func TestUpdateExportSelectMovesSelection(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	if m.exportSelect.sel != 0 {
		t.Fatalf("initial sel = %d, want 0", m.exportSelect.sel)
	}
	mm, _ := m.updateExportSelect(downKey())
	m2 := mm.(model)
	if m2.exportSelect.sel != 1 {
		t.Fatalf("sel after down = %d, want 1", m2.exportSelect.sel)
	}
}

func TestUpdateExportSelectEscReturnsToWriting(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(escKey())
	m2 := mm.(model)
	if m2.screen != screenWriting {
		t.Fatalf("screen after esc = %v, want screenWriting", m2.screen)
	}
}

func TestUpdateExportSelectToggleExcludesSceneAndPersists(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[1] is the first scene ("Opening") in seedExportSelectManuscript.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown}) // move to entry 1
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m3 := mm2.(model)
	if !m3.exportSelect.entries[1].excluded {
		t.Fatal("entry 1 must be excluded after toggling space")
	}
	// Persisted: a fresh enterExportSelect reload must show the same exclusion.
	m4 := model{}
	m4.files.dir = dir
	m4.enterExportSelect()
	if !m4.exportSelect.entries[1].excluded {
		t.Fatal("exclusion must survive a reload (persisted to the sidecar)")
	}
}

func TestUpdateExportSelectToggleTwiceReincludes(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(space)
	m3 := mm2.(model)
	mm3, _ := m3.updateExportSelect(space)
	m4 := mm3.(model)
	if m4.exportSelect.entries[1].excluded {
		t.Fatal("toggling twice must re-include the entry")
	}
}

func TestUpdateExportSelectToggleOnHeaderExcludesAllChildren(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[0] is the chapter header "Chapter One".
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	mm, _ := m.updateExportSelect(space)
	m2 := mm.(model)
	if !m2.exportSelect.entries[1].excluded || !m2.exportSelect.entries[2].excluded {
		t.Fatalf("toggling the header must exclude both child scenes, got entries=%+v", m2.exportSelect.entries[:3])
	}
}

func TestUpdateExportSelectHeaderStateIsDerivedIncludedIfAnyChildIncluded(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}
	// Exclude only the FIRST child scene (entry 1), leave the second (entry 2) included.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(space)
	m3 := mm2.(model)
	// The header (entry 0) must read as included, since at least one child (entry 2) still is.
	if m3.exportSelect.entries[0].excluded {
		t.Fatal("header must show included when at least one child scene is included")
	}
}

func TestExportSelectTotalsExcludeExcludedEntries(t *testing.T) {
	entries := []exportSelectEntry{
		{isHeader: true, title: "C1"},
		{words: 10, chars: 50},
		{words: 5, chars: 25, excluded: true},
	}
	w, c := exportSelectTotals(entries)
	if w != 10 || c != 50 {
		t.Fatalf("totals = (%d, %d), want (10, 50) — excluded entry and header must not count", w, c)
	}
}

func TestMoveExportSelectSceneWithinSameChapterReordersTextsOnly(t *testing.T) {
	dir := seedExportSelectManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[1] = "Opening", entry[2] = "Confrontation", both in ch1. Move entry 1 down.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})
	m3 := mm2.(model)
	if m3.exportSelect.entries[1].title != "Confrontation" || m3.exportSelect.entries[2].title != "Opening" {
		t.Fatalf("want scenes swapped, got %q, %q", m3.exportSelect.entries[1].title, m3.exportSelect.entries[2].title)
	}
	got, _, _ := readManifest(dir)
	ch := findChapterByFolder(&got, "ch1")
	if ch == nil || ch.Texts[0].File != "scene-2.md" || ch.Texts[1].File != "scene-1.md" {
		t.Fatalf("manifest Texts[] must reflect the swap, got %+v", ch)
	}
	// No file should have moved on disk — same chapter, same folder.
	if _, err := os.Stat(filepath.Join(dir, "ch1", "scene-1.md")); err != nil {
		t.Fatal("scene-1.md must still exist in ch1/")
	}
}

func TestMoveExportSelectChapterHeaderMovesWholeBlock(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "a.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "b.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "a.md", Title: "A"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "b.md", Title: "B"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entry[0] = header "First". Move it down past "Second"'s header + its scene.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})
	m2 := mm.(model)
	got, _, _ := readManifest(dir)
	if got.Items[0].Chapter.Folder != "ch2" || got.Items[1].Chapter.Folder != "ch1" {
		t.Fatalf("manifest items[] must reflect First/Second swap, got %+v", got.Items)
	}
	// Entries list must be rebuilt in the new order too.
	if m2.exportSelect.entries[0].title != "Second" {
		t.Fatalf("entries[0] = %q, want %q after moving First past Second", m2.exportSelect.entries[0].title, "Second")
	}
}

func TestMoveExportSelectSceneCrossesIntoAdjacentChapter(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "only.md"), []byte("The only scene of ch1."), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "first.md"), []byte("ch2's first scene."), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "only.md", Title: "Only"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "first.md", Title: "First"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entries: [0]=header First, [1]="Only" (indented), [2]=header Second, [3]="First" (indented).
	// Move "Only" (index 1) down, crossing into ch2.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})
	m3 := mm2.(model)

	if _, err := os.Stat(filepath.Join(dir, "ch2", "only.md")); err != nil {
		t.Fatalf("only.md must have moved to ch2/, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ch1", "only.md")); !os.IsNotExist(err) {
		t.Fatal("only.md must no longer exist in ch1/")
	}
	got, _, _ := readManifest(dir)
	ch1 := findChapterByFolder(&got, "ch1")
	ch2 := findChapterByFolder(&got, "ch2")
	if len(ch1.Texts) != 0 {
		t.Fatalf("ch1 must now have 0 texts, got %+v", ch1.Texts)
	}
	found := false
	for _, t := range ch2.Texts {
		if t.File == "only.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ch2 must now list only.md, got %+v", ch2.Texts)
	}
	_ = m3
}

func TestMoveExportSelectMigratesSidecarKeyOnCrossBoundaryMove(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "only.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "first.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "only.md", Title: "Only"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "first.md", Title: "First"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	oldKey := filepath.Join("ch1", "only.md")
	if err := saveExportSelection(dir, map[string]bool{oldKey: true}, map[string]bool{oldKey: true}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})

	newKey := filepath.Join("ch2", "only.md")
	got := loadExportSelection(dir)
	if got[oldKey] {
		t.Fatal("old sidecar key must not survive the move")
	}
	if !got[newKey] {
		t.Fatal("exclusion must migrate to the new path")
	}
}

func TestMoveExportSelectRefusesNameCollisionInDestination(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "ch1"), 0o755)
	os.MkdirAll(filepath.Join(dir, "ch2"), 0o755)
	os.WriteFile(filepath.Join(dir, "ch1", "same.md"), []byte("from ch1"), 0o644)
	os.WriteFile(filepath.Join(dir, "ch2", "same.md"), []byte("already in ch2"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Folder: "ch1", Title: "First", Texts: []manifestText{{File: "same.md", Title: "Same"}}}},
			{Chapter: &manifestChapter{Folder: "ch2", Title: "Second", Texts: []manifestText{{File: "same.md", Title: "Same"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyDown})
	m2 := mm.(model)
	mm2, _ := m2.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})
	m3 := mm2.(model)

	if _, err := os.Stat(filepath.Join(dir, "ch1", "same.md")); err != nil {
		t.Fatal("ch1/same.md must NOT have been moved away (collision refused)")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "ch2", "same.md"))
	if string(data) != "already in ch2" {
		t.Fatal("ch2/same.md must be untouched (not overwritten)")
	}
	if m3.status == "" {
		t.Fatal("a status message must explain the refused move")
	}
}

func TestMoveExportSelectStandaloneSceneBecomesResourceCrossingBoundary(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "aparte.md"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("y"), 0o644)
	if err := writeManifest(dir, manifest{
		Title: "N",
		Items: []manifestItem{
			{Chapter: &manifestChapter{Title: "Aparté", Scene: true, Texts: []manifestText{{File: "aparte.md", Title: "Aparté"}}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := model{}
	m.files.dir = dir
	m.enterExportSelect()
	// entries: [0] = standalone scene "Aparté", [1] = Resource "notes". Move Aparté past notes.
	mm, _ := m.updateExportSelect(tea.KeyMsg{Type: tea.KeyShiftDown})
	m2 := mm.(model)
	got, _, _ := readManifest(dir)
	if len(got.Items) != 0 {
		t.Fatalf("Aparté must have been dropped from items[] (now a plain Resource), got %+v", got.Items)
	}
	if _, err := os.Stat(filepath.Join(dir, "aparte.md")); err != nil {
		t.Fatal("aparte.md must still exist on disk (unmoved — root stays root)")
	}
	_ = m2
}
