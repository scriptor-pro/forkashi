package main

import (
	"os"
	"path/filepath"
	"strconv"
)

// structAdd is one row in the add pick: the "new blank chapter" sentinel (file == "") or an
// existing loose Resource file.
type structAdd struct {
	file  string // "" = new blank chapter
	label string
}

// structureAddChoices is [new blank chapter] followed by the manuscript's loose Resources (on-disk
// root .md files not currently staged as a bare chapter), de-slug-titled. listed is keyed by
// chapter FOLDER (v2 identity), built from the staged buffer — not from disk — so an
// uncommitted x-remove immediately reveals the file as promotable again and an uncommitted
// a-add immediately hides it.
func (m model) structureAddChoices() []structAdd {
	out := []structAdd{{file: "", label: "＋ nouveau chapitre vide"}}
	listed := map[string]bool{}
	for _, ch := range m.structureItems {
		for _, t := range ch.texts {
			listed[t.file] = true
		}
	}
	for _, e := range readEntries(m.structureDir) { // non-hidden root document files
		if listed[e.name] {
			continue
		}
		out = append(out, structAdd{file: e.name, label: "◦ " + sectionTitle(e.name)})
	}
	return out
}

// uniqueChapterFolder returns a chapter folder slug not present on disk in dir and not already
// taken by the buffer/pending set — "untitled", "untitled-2", … (mirrors uniqueChapterFile's old
// v1 filename-uniqueness scheme, but a chapter is now a folder, not a direct file).
func uniqueChapterFolder(dir string, taken map[string]bool) string {
	for n := 1; ; n++ {
		name := "untitled"
		if n > 1 {
			name = "untitled-" + strconv.Itoa(n)
		}
		if taken[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// applyAdd inserts the chosen add option after the cursor: either a brand-new blank chapter
// (folder + one empty text, created on commit via structurePendingNew) or an existing loose
// Resource promoted to a chapter (its file will be moved into a new same-slug folder on
// commit — see commitStructure).
func (m *model) applyAdd(c structAdd) {
	at := m.structureSel + 1
	if at > len(m.structureItems) {
		at = len(m.structureItems)
	}
	var ch chapterRef
	if c.file == "" { // new blank chapter
		taken := map[string]bool{}
		for _, x := range m.structureItems {
			taken[x.folder] = true
		}
		for f := range m.structurePendingNew {
			taken[f] = true
		}
		folder := uniqueChapterFolder(m.structureDir, taken)
		m.structurePendingNew[folder] = true
		file := folder + ".md"
		ch = chapterRef{folder: folder, title: "Pas encore de titre", texts: []textRef{{file: file, title: "Pas encore de titre"}}}
	} else { // promote an existing loose Resource: folder slug derived from its title
		title := sectionTitle(c.file)
		folder := slugify(title)
		ch = chapterRef{folder: folder, title: title, texts: []textRef{{file: c.file, title: title}}}
	}
	m.structureItems = append(m.structureItems[:at], append([]chapterRef{ch}, m.structureItems[at:]...)...)
	m.structureSel = at
	m.structureDirty = true
}

// structureTitle is the manuscript's display title (from the on-disk manifest, falling back to the
// folder name).
func (m model) structureTitle() string {
	if sm, present, err := readManifest(m.structureDir); present && err == nil && sm.Title != "" {
		return sm.Title
	}
	return projectTitle(filepath.Base(m.structureDir))
}

// toManifestChapters converts the staged bare-chapter buffer to manifest wire shape. A
// staged standalone scene (ch.scene) must round-trip Scene:true — otherwise committing a
// reorder that merely passes a scene through the buffer would silently demote it back to an
// ordinary (legacy-shaped) chapter on the next manifest read.
func (m model) toManifestChapters() []manifestChapter {
	out := make([]manifestChapter, 0, len(m.structureItems))
	for _, ch := range m.structureItems {
		texts := make([]manifestText, 0, len(ch.texts))
		for _, t := range ch.texts {
			texts = append(texts, manifestText{File: t.file, Title: t.title})
		}
		out = append(out, manifestChapter{Folder: ch.folder, Title: ch.title, Texts: texts, Scene: ch.scene})
	}
	return out
}

// commitStructure persists the staged bare-chapter buffer via the atomic writeManifest
// (re-reading the on-disk title first). Only bare chapters (items with Chapter != nil) are
// replaced by the staged buffer, in the staged order; any real Part items are read straight
// off the current on-disk manifest and carried through UNTOUCHED, in their original relative
// position among the bare-chapter items — this plan's structure mode only edits bare chapters
// (see enterCorkboard); full 3-level (Part-aware) structure mode is the next plan's job.
// Ordering rule: since v1 items[] interleaves bare chapters and Parts in one array, and the
// staged buffer only ever reorders/adds/removes bare chapters (never Parts), the simplest
// correct rewrite is: bare-chapter items become the staged buffer, in staged order, placed
// BEFORE any Part items, in the same relative order Part items already had — this exactly
// matches how manifestView (manuscript.go) already presents them (the synthetic untitled part
// always comes first), so nothing this plan can observe distinguishes it from a true
// interleave-preserving rewrite.
func (m *model) commitStructure() error {
	// Write the manifest BEFORE creating the new blank/promoted files. If a file creation then
	// fails, the manifest lists a not-yet-created file (a listed-but-missing chapter — benign,
	// shown as a missing chapter). The reverse order would leave an orphan blank file (present
	// but unlisted → a silent Resource) if the manifest write failed.
	title := m.structureTitle() // re-reads the on-disk manifest title (read-modify-write)
	var partItems []manifestItem
	if cur, present, err := readManifest(m.structureDir); err == nil && present {
		for _, it := range cur.Items {
			if it.Chapter == nil { // a real Part — preserve untouched
				partItems = append(partItems, it)
			}
		}
	}
	items := make([]manifestItem, 0, len(m.structureItems)+len(partItems))
	for _, mc := range m.toManifestChapters() {
		mc := mc
		items = append(items, manifestItem{Chapter: &mc})
	}
	items = append(items, partItems...)
	if err := writeManifest(m.structureDir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         items,
	}); err != nil {
		return err
	}
	// Only create/move files that survived to the final staged buffer.
	inBuf := map[string]chapterRef{}
	for _, ch := range m.structureItems {
		inBuf[ch.folder] = ch
	}
	for folder := range m.structurePendingNew {
		ch, ok := inBuf[folder]
		if !ok || len(ch.texts) == 0 {
			continue
		}
		chDir := filepath.Join(m.structureDir, folder)
		if err := os.MkdirAll(chDir, 0o755); err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(chDir, ch.texts[0].file), []byte(""), 0o644); err != nil {
			return err
		}
	}
	// Promoted Resources: a staged chapter whose folder doesn't exist yet but whose text file
	// exists loose at the manuscript root needs that file moved into its new chapter folder.
	for _, ch := range m.structureItems {
		if m.structurePendingNew[ch.folder] || len(ch.texts) == 0 {
			continue // new-blank already handled above
		}
		chDir := filepath.Join(m.structureDir, ch.folder)
		if _, err := os.Stat(chDir); err == nil {
			continue // already a real chapter folder — nothing to promote
		}
		loose := filepath.Join(m.structureDir, ch.texts[0].file)
		if _, err := os.Stat(loose); err != nil {
			continue // not a loose file either (already resolved) — nothing to do
		}
		if err := os.MkdirAll(chDir, 0o755); err != nil {
			return err
		}
		if err := safeMove(loose, filepath.Join(chDir, ch.texts[0].file)); err != nil {
			return err
		}
	}
	return nil
}

// fmtNum formats n as a zero-padded 2-digit string.
func fmtNum(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
