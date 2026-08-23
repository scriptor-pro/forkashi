package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const manifestName = "manifest.json"
const manifestSchemaVersion = 3

// manifestText is one ordered text (scene) inside a chapter: a bare filename (slug,
// no numeric prefix — order lives in the manifest, not the filename) plus a display title.
type manifestText struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// manifestChapter is one chapter — OR, when Scene is true, one standalone scene: a single
// ordered text with no sub-texts and no folder of its own (its file lives at the manuscript
// root, named by Texts[0].File). Folder is "" for a scene. Invariant: Scene == true iff
// Folder == "" && len(Texts) == 1.
type manifestChapter struct {
	Folder string         `json:"folder"`
	Title  string         `json:"title"`
	Texts  []manifestText `json:"texts"`
	Scene  bool           `json:"scene,omitempty"`
}

// manifestItem is one top-level manuscript entry: EITHER a bare chapter (Chapter set,
// Part empty, Chapters nil) OR a Part grouping its own ordered chapters (Part set,
// Chapters set, Chapter nil). Mutually exclusive — never both.
type manifestItem struct {
	Part     string            `json:"part,omitempty"`
	Chapters []manifestChapter `json:"chapters,omitempty"`
	Chapter  *manifestChapter  `json:"chapter,omitempty"`
}

// manifest is the shared per-manuscript order/membership/title file. okashi reads it AND
// writes it — create + chapter-title retitle today, structural edits later (see design §0).
type manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Title         string         `json:"title"`
	Items         []manifestItem `json:"items"`
}

// hasManifest reports whether dir contains a manifest.json — the companion app's manuscript
// marker (design §4: folder with manifest = manuscript).
func hasManifest(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, manifestName))
	return err == nil
}

// readManifest loads dir/manifest.json. When the file is absent it returns
// present=false, err=nil. A present-but-unreadable manifest (malformed JSON or an
// unsupported schemaVersion) returns present=true with a non-nil err: okashi
// REFUSES to guess structure and NEVER writes the file back (design §4.1).
func readManifest(dir string) (m manifest, present bool, err error) {
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(readErr) {
		return manifest{}, false, nil
	}
	if readErr != nil {
		return manifest{}, true, readErr
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, true, err
	}
	if m.SchemaVersion != manifestSchemaVersion {
		return manifest{}, true, fmt.Errorf(
			"unsupported manifest schemaVersion %d (okashi supports %d)",
			m.SchemaVersion, manifestSchemaVersion)
	}
	return m, true, nil
}

// writeManifest serializes m to dir/manifest.json atomically. okashi owns manifest writes for
// its own AND the shared corpus (design §0); the schema is forced to EXACTLY
// manifestSchemaVersion so the companion app reads it verbatim. The serialization matches the companion app's
// JSONEncoder(.prettyPrinted, .sortedKeys): alphabetically-sorted keys, 2-space indent, no
// trailing newline — so when the two apps alternate writes the NSFileVersion diff stays small
// and legible instead of a whole-file reformat (storage-spine §67-69). Go sorts map keys
// alphabetically, yielding the same items/schemaVersion/title (and file/title) order the companion app emits.
func writeManifest(dir string, m manifest) error {
	m.SchemaVersion = manifestSchemaVersion
	// Force a non-nil slice so an empty manuscript serializes as `[]`, not `null` (which
	// the companion app's [ManifestItem] decode would reject).
	items := m.Items
	if items == nil {
		items = []manifestItem{}
	}
	top := map[string]any{
		"schemaVersion": m.SchemaVersion,
		"title":         m.Title,
		"items":         items,
	}
	// SetEscapeHTML(false): Swift's JSONEncoder emits &, <, > literally; Go's default escapes
	// them to their \uXXXX forms, which would churn the whole file whenever a title contains one
	// ("Tom & Jerry"). Encoder also appends a trailing '\n'; trim it to match Swift (no newline).
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(top); err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, manifestName), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

// createManuscript makes a brand-new manuscript at dir: the folder, a first chapter folder
// holding one empty text file, and a v2 manifest listing it as a bare chapter (no Part).
// firstChapter is that chapter's display title. It refuses to clobber an existing manifest
// and returns the first chapter's text filename (relative to its chapter folder) so the
// caller can build the full path (filepath.Join(dir, folder, file)) to open it.
func createManuscript(dir, title, firstChapter string) (string, error) {
	if hasManifest(dir) {
		return "", fmt.Errorf("a manuscript already exists at %s", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	folder := slugify(firstChapter)
	file := folder + ".md"
	chDir := filepath.Join(dir, folder)
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		return "", err
	}
	if err := atomicWrite(filepath.Join(chDir, file), []byte(""), 0o644); err != nil {
		return "", err
	}
	err := writeManifest(dir, manifest{
		SchemaVersion: manifestSchemaVersion,
		Title:         title,
		Items: []manifestItem{{Chapter: &manifestChapter{
			Folder: folder,
			Title:  firstChapter,
			Texts:  []manifestText{{File: file, Title: firstChapter}},
		}}},
	})
	return filepath.Join(folder, file), err
}

// renameChapterTitle edits ONLY the items[].chapter.title (or items[].chapters[].title, for a
// chapter inside a Part) of the chapter whose folder matches in dir's manifest, preserving
// order and membership; the folder is birth-stable (design §5.7 — filenames are birth-stable
// in v1, folders play that role in v2). It read-modify-writes (re-reads immediately before
// writing, §0) and refuses a folder that is not a listed chapter or a dir without a readable
// manifest.
func renameChapterTitle(dir, folder, newTitle string) error {
	m, present, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("no manifest in %s", dir)
	}
	found := false
	for i := range m.Items {
		if m.Items[i].Chapter != nil && m.Items[i].Chapter.Folder == folder {
			m.Items[i].Chapter.Title = newTitle
			found = true
			break
		}
		for j := range m.Items[i].Chapters {
			if m.Items[i].Chapters[j].Folder == folder {
				m.Items[i].Chapters[j].Title = newTitle
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return fmt.Errorf("%s is not a chapter of %s", folder, dir)
	}
	return writeManifest(dir, m)
}

// renameSceneTitle edits ONLY the items[].chapter.title of the standalone scene whose
// Texts[0].File matches file — the scene counterpart of renameChapterTitle, keyed by
// filename instead of folder (a scene has no folder). The file itself is never renamed
// (birth-stable, mirroring a chapter's folder). Read-modify-writes like renameChapterTitle.
func renameSceneTitle(dir, file, newTitle string) error {
	m, present, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("no manifest in %s", dir)
	}
	sc := findSceneByFile(&m, file)
	if sc == nil {
		return fmt.Errorf("%s is not a standalone scene in %s", file, dir)
	}
	sc.Title = newTitle
	return writeManifest(dir, m)
}

// findChapterByFolder returns a pointer to the manifestChapter matching folder — across the
// root and every Part's Chapters — so the caller can mutate it (e.g. append a Texts entry)
// before a single writeManifest. Returns nil if no chapter with that folder exists.
func findChapterByFolder(m *manifest, folder string) *manifestChapter {
	for i := range m.Items {
		if m.Items[i].Chapter != nil && m.Items[i].Chapter.Folder == folder {
			return m.Items[i].Chapter
		}
		for j := range m.Items[i].Chapters {
			if m.Items[i].Chapters[j].Folder == folder {
				return &m.Items[i].Chapters[j]
			}
		}
	}
	return nil
}

// findSceneByFile returns a pointer to the standalone-scene manifestChapter whose
// Texts[0].File matches file, at the manuscript root — the scene identity lookup mirroring
// findChapterByFolder (a scene has no Folder, so its birth-stable identity is its filename
// instead). Returns nil if no standalone scene with that file exists.
func findSceneByFile(m *manifest, file string) *manifestChapter {
	for i := range m.Items {
		if m.Items[i].Chapter != nil && m.Items[i].Chapter.Scene &&
			len(m.Items[i].Chapter.Texts) > 0 && m.Items[i].Chapter.Texts[0].File == file {
			return m.Items[i].Chapter
		}
	}
	return nil
}
