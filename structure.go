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
// .md not currently listed in the buffer nor pending-new), de-slug-titled.
func (m model) structureAddChoices() []structAdd {
	out := []structAdd{{file: "", label: "＋ nouveau chapitre vide"}}
	listed := map[string]bool{}
	for _, it := range m.structureItems {
		listed[it.File] = true
	}
	for _, e := range readEntries(m.structureDir) { // non-hidden document files
		if listed[e.name] {
			continue
		}
		out = append(out, structAdd{file: e.name, label: "◦ " + sectionTitle(e.name)})
	}
	return out
}

// uniqueChapterFile returns a filename not present on disk in dir and not already taken by the
// buffer/pending set — "untitled.md", "untitled-2.md", …
func uniqueChapterFile(dir string, taken map[string]bool) string {
	for n := 1; ; n++ {
		name := "untitled.md"
		if n > 1 {
			name = "untitled-" + strconv.Itoa(n) + ".md"
		}
		if taken[name] {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// applyAdd inserts the chosen add option after the cursor.
func (m *model) applyAdd(c structAdd) {
	at := m.structureSel + 1
	if at > len(m.structureItems) {
		at = len(m.structureItems)
	}
	var it manifestItem
	if c.file == "" { // new blank chapter
		taken := map[string]bool{}
		for _, x := range m.structureItems {
			taken[x.File] = true
		}
		for f := range m.structurePendingNew {
			taken[f] = true
		}
		f := uniqueChapterFile(m.structureDir, taken)
		m.structurePendingNew[f] = true
		it = manifestItem{File: f, Title: "Pas encore de titre"}
	} else { // promote an existing loose Resource
		it = manifestItem{File: c.file, Title: sectionTitle(c.file)}
	}
	m.structureItems = append(m.structureItems[:at], append([]manifestItem{it}, m.structureItems[at:]...)...)
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

// persists the whole buffer via the atomic writeManifest (re-reading the on-disk title first).
func (m *model) commitStructure() error {
	// Write the manifest BEFORE creating the new blank files. If a file creation then fails, the
	// manifest lists a not-yet-created file (a listed-but-missing chapter — benign, shown as a
	// missing chapter). The reverse order would leave an orphan blank file (present but unlisted →
	// a silent Resource) if the manifest write failed.
	title := m.structureTitle() // re-reads the on-disk manifest title (read-modify-write)
	if err := writeManifest(m.structureDir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items:         m.structureItems,
	}); err != nil {
		return err
	}
	for f := range m.structurePendingNew {
		// only create files that survived to the final buffer
		inBuf := false
		for _, it := range m.structureItems {
			if it.File == f {
				inBuf = true
				break
			}
		}
		if !inBuf {
			continue
		}
		if err := atomicWrite(filepath.Join(m.structureDir, f), []byte(""), 0o644); err != nil {
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
