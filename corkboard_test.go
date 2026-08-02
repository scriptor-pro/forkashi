package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"okashi/internal/textarea"
)

func TestCorkboardAltMoveMirrorsOutline(t *testing.T) {
	dir := seedCorkManuscript(t) // 01-a, 02-b, 03-c
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()

	// alt+↓ moves the selected chapter down (mirrors the outline's move-beat chord) and follows it.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyDown, Alt: true})
	m = mm.(model)
	if m.structureSel != 1 || !m.structureDirty {
		t.Fatalf("alt+down should move + follow (sel=%d dirty=%v)", m.structureSel, m.structureDirty)
	}
	if m.structureItems[0].File != "02-b.md" || m.structureItems[1].File != "01-a.md" {
		t.Fatalf("alt+down should swap items 0 and 1, got %v", m.structureItems)
	}
	// alt+↑ moves it back.
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	m = mm.(model)
	if m.structureSel != 0 || m.structureItems[0].File != "01-a.md" {
		t.Fatalf("alt+up should restore the order (sel=%d items=%v)", m.structureSel, m.structureItems)
	}
}

func TestCorkboardCardMeta(t *testing.T) {
	// The currently-open chapter carries the open marker; others don't.
	if mk, _, _ := corkboardCardMeta(true, "", ""); !strings.Contains(mk, "●") {
		t.Fatalf("current chapter should carry the open marker, got %q", mk)
	}
	if mk, _, _ := corkboardCardMeta(false, "syn", "fl"); mk != "" {
		t.Fatalf("non-current chapter should have no open marker, got %q", mk)
	}
	// Authored synopsis → not dim, body is the synopsis.
	if _, body, dim := corkboardCardMeta(false, "the synopsis", "first line"); dim || body != "the synopsis" {
		t.Fatalf("authored synopsis: want (synopsis, dim=false), got (%q,%v)", body, dim)
	}
	// No synopsis but a first line → dimmed fallback.
	if _, body, dim := corkboardCardMeta(false, "", "the first line"); !dim || body != "the first line" {
		t.Fatalf("fallback: want (first line, dim=true), got (%q,%v)", body, dim)
	}
	// Neither → empty raw body (caller renders the placeholder).
	if _, body, _ := corkboardCardMeta(false, "", ""); body != "" {
		t.Fatalf("no synopsis + no first line: want empty raw body, got %q", body)
	}
}

func TestCorkboardStatusLine(t *testing.T) {
	items := []manifestItem{{File: "a.md"}, {File: "b.md"}}
	// wc == nil → total counts as 0; still reports the chapter count, no target fragment.
	got := corkboardStatusLine(items, "/x", nil, projectGoals{})
	if !strings.Contains(got, "2 chapitres") {
		t.Fatalf("want chapter count, got %q", got)
	}
	if strings.Contains(got, "/") {
		t.Fatalf("no goal set → no target fragment, got %q", got)
	}
	withGoal := corkboardStatusLine(items, "/x", nil, projectGoals{ProjectGoal: 80000, Deadline: "2026-03-01"})
	if !strings.Contains(withGoal, "/ 80,000") || !strings.Contains(withGoal, "avant 2026-03-01") {
		t.Fatalf("want target + deadline, got %q", withGoal)
	}
	one := corkboardStatusLine(items[:1], "/x", nil, projectGoals{})
	if !strings.Contains(one, "1 chapitre ") {
		t.Fatalf("want singular 'chapter', got %q", one)
	}
	// With a real word-count cache the total sums the chapters (3 + 2 = 5 words).
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("one two three"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("four five"), 0o644)
	if got := corkboardStatusLine(items, dir, newWordCountCache(), projectGoals{}); !strings.Contains(got, "5 mots") {
		t.Fatalf("want summed total '5 words', got %q", got)
	}
}

func seedCorkManuscript(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	for _, f := range []string{"01-a.md", "02-b.md", "03-c.md"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("body of "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         "The Work",
		Items: []manifestItem{
			{File: "01-a.md", Title: "One"}, {File: "02-b.md", Title: "Two"}, {File: "03-c.md", Title: "Three"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCorkboardEntryRequiresManifest(t *testing.T) {
	// No manifest → refused, stays on the binder.
	m := model{}
	m.files.dir = t.TempDir()
	m.enterCorkboard()
	if m.screen == screenCorkboard {
		t.Fatal("a non-manifest dir must not enter the corkboard")
	}

	dir := seedCorkManuscript(t)
	m2 := model{}
	m2.files.dir = dir
	m2.enterCorkboard()
	if m2.screen != screenCorkboard {
		t.Fatalf("a manifest manuscript should enter the corkboard, screen=%v", m2.screen)
	}
	if len(m2.structureItems) != 3 {
		t.Fatalf("staged buffer should hold 3 chapters, got %d", len(m2.structureItems))
	}
}

func TestCorkboardSynopsisEditWritesSidecar(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	m.structureSel = 1 // chapter 02-b.md

	// e → edit; type; esc → commit + immediate sidecar write.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = mm.(model)
	if !m.synEditing {
		t.Fatal("e should open the synopsis editor")
	}
	m.synArea.SetValue("The train is late.")
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if m.synEditing {
		t.Fatal("esc should commit and close the editor")
	}
	// Persisted to the sidecar and not requiring a manifest commit.
	if loadSynopses(dir)["02-b.md"] != "The train is late." {
		t.Fatalf("synopsis not written to sidecar: %+v", loadSynopses(dir))
	}
	if m.structureDirty {
		t.Fatal("a synopsis edit must not mark the manifest dirty")
	}
}

func TestCorkboardReorderCommitsViaStructurePath(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	m.structureSel = 0

	// J moves chapter 1 down → order becomes b, a, c.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = mm.(model)
	if !m.structureDirty {
		t.Fatal("reorder should mark the manifest dirty")
	}
	// esc → confirm, y → commit via the shared commitStructure path.
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	if !m.structureConfirm {
		t.Fatal("esc with a dirty order should raise the commit confirm")
	}
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)

	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 3 || mani.Items[0].File != "02-b.md" || mani.Items[1].File != "01-a.md" {
		t.Fatalf("reorder not committed to the manifest: %+v", mani.Items)
	}
	if m.screen != screenWriting {
		t.Fatalf("committing should return to the writing screen, got %v", m.screen)
	}
}

func TestCorkboardRemoveDemotesChapter(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{editor: textarea.New()}
	m.files.dir = dir
	m.enterCorkboard()
	m.structureSel = 1 // 02-b.md
	// x → demote to Resource (staged), then esc → confirm → y commit.
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = mm.(model)
	if !m.structureDirty {
		t.Fatal("x should stage a change")
	}
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)
	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 2 {
		t.Fatalf("chapter should be demoted (2 items left), got %+v", mani.Items)
	}
	for _, it := range mani.Items {
		if it.File == "02-b.md" {
			t.Fatal("02-b.md should no longer be a listed chapter")
		}
	}
	// The file itself is untouched (non-destructive demote).
	if _, err := os.Stat(filepath.Join(dir, "02-b.md")); err != nil {
		t.Fatal("demote must not delete the file")
	}
}

func TestCorkboardEnterOpensChapter(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{editor: textarea.New()}
	m.files.dir = dir
	m.files.root = dir
	m.files.SetDir(dir)
	m.enterCorkboard()
	m.structureSel = 2 // 03-c.md
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEnter})
	m = mm.(model)
	if m.screen != screenWriting {
		t.Fatalf("enter should open the chapter + return to writing, got %v", m.screen)
	}
	if m.currentFile != filepath.Join(dir, "03-c.md") {
		t.Fatalf("enter should open the selected chapter, got %q", m.currentFile)
	}
}

// Discarding a reorder must leave no stale dirty flag or mutated staged buffer behind.
func TestCorkboardDiscardResetsStagedState(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{}
	m.files.dir = dir
	m.enterCorkboard()
	// Reorder, then esc → confirm → esc (discard).
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'J'}})
	m = mm.(model)
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc}) // discard
	m = mm.(model)

	if m.structureDirty {
		t.Fatal("discard must clear structureDirty")
	}
	if m.structureItems != nil {
		t.Fatal("discard must clear the staged buffer")
	}
	// The on-disk manifest is untouched by a discard.
	mani, _, _ := readManifest(dir)
	if mani.Items[0].File != "01-a.md" {
		t.Fatalf("discard must not change the manifest, got %+v", mani.Items)
	}
}

func TestCorkboardViewWindows(t *testing.T) {
	dir := seedCorkManuscript(t)
	m := model{width: 90, height: 12}
	m.files.dir = dir
	m.enterCorkboard()
	out := m.corkboardView()
	if out == "" {
		t.Fatal("corkboard view should render")
	}
}

// A staged (uncommitted) x-demote must not cause a synopsis edit to prune the still-live chapter's
// synopsis off disk (the Critical the review caught).
func TestCorkboardSynopsisEditPreservesStagedRemovedChapter(t *testing.T) {
	dir := seedCorkManuscript(t) // 01-a, 02-b, 03-c
	saveSynopses(dir, map[string]string{"01-a.md": "A syn", "02-b.md": "B syn", "03-c.md": "C syn"},
		map[string]bool{"01-a.md": true, "02-b.md": true, "03-c.md": true})
	m := model{editor: textarea.New()}
	m.files.dir = dir
	m.enterCorkboard()
	m.structureSel = 1
	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}) // stage demote of 02-b (not committed)
	m = mm.(model)
	m.structureSel = 0
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}}) // edit 01-a synopsis
	m = mm.(model)
	m.synArea.SetValue("A edited")
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc})
	m = mm.(model)

	syn := loadSynopses(dir)
	if syn["02-b.md"] != "B syn" {
		t.Fatalf("a staged-removed-but-still-on-disk chapter's synopsis must survive, got %v", syn)
	}
	if syn["01-a.md"] != "A edited" {
		t.Fatalf("the edited synopsis should be saved, got %v", syn)
	}
}

// The corkboard 'a' (add new blank chapter) → commit must create the file and list it (preserves
// the applyAdd + commitStructure file-creation coverage from the retired structure_test.go).
func TestCorkboardAddNewChapterCreatesFile(t *testing.T) {
	dir := seedCorkManuscript(t) // 01-a, 02-b, 03-c
	m := model{editor: textarea.New()}
	m.files.dir = dir
	m.enterCorkboard()
	m.structureSel = 0

	mm, _ := m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}) // add picker
	m = mm.(model)
	if !m.structureAdding {
		t.Fatal("a should open the add picker")
	}
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEnter}) // choice 0 = new blank chapter
	m = mm.(model)
	if len(m.structureItems) != 4 {
		t.Fatalf("add should stage a 4th chapter, got %d", len(m.structureItems))
	}
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyEsc}) // dirty → confirm
	m = mm.(model)
	mm, _ = m.updateCorkboard(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = mm.(model)

	mani, _, _ := readManifest(dir)
	if len(mani.Items) != 4 {
		t.Fatalf("commit should list the 4th chapter, got %d", len(mani.Items))
	}
	orig := map[string]bool{"01-a.md": true, "02-b.md": true, "03-c.md": true}
	var newFile string
	for _, it := range mani.Items {
		if !orig[it.File] {
			newFile = it.File
		}
	}
	if newFile == "" {
		t.Fatal("a new chapter should be added")
	}
	if _, err := os.Stat(filepath.Join(dir, newFile)); err != nil {
		t.Fatalf("commit should create the new blank file %s: %v", newFile, err)
	}
}
