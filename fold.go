package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

const foldedName = ".okashi-folded.json"
const foldedSchemaVersion = 1

// foldedFile is the per-manuscript sidebar fold-state sidecar (okashi-owned, NOT the
// manifest — the shared contract HARD GATE stays untriggered). Keyed by corkKey(chapterRef)
// (corkboard.go) → true when the chapter's scenes are expanded inline in the sidebar. A
// chapter with no key is collapsed (the default) — only expanded chapters are persisted, so
// an untouched manuscript never grows this sidecar.
type foldedFile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Folded        map[string]bool `json:"folded"`
}

func foldedPath(dir string) string { return filepath.Join(dir, foldedName) }

// loadFolded reads the sidecar; missing/corrupt/unsupported-schema all yield an empty map —
// never an error (mirrors loadSynopses's tolerant load).
func loadFolded(dir string) map[string]bool {
	out := map[string]bool{}
	data, err := os.ReadFile(foldedPath(dir))
	if err != nil {
		return out
	}
	var ff foldedFile
	if json.Unmarshal(data, &ff) != nil || ff.SchemaVersion != foldedSchemaVersion {
		return out
	}
	if ff.Folded != nil {
		out = ff.Folded
	}
	return out
}

// saveFolded writes the sidecar atomically. It prunes keys not in known (self-healing orphans
// left by a removed/renamed-away chapter) and drops false entries, so the file only ever holds
// live, expanded chapters. An empty result removes the sidecar file entirely rather than
// writing an empty object, mirroring saveNotes's empty-list removal. Serialized like
// writeManifest/saveSynopses: 2-space indent, no HTML escaping, no trailing newline.
func saveFolded(dir string, folded map[string]bool, known map[string]bool) error {
	pruned := map[string]bool{}
	for key, v := range folded {
		if known[key] && v {
			pruned[key] = true
		}
	}
	if len(pruned) == 0 {
		err := os.Remove(foldedPath(dir))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(foldedFile{SchemaVersion: foldedSchemaVersion, Folded: pruned}); err != nil {
		return err
	}
	return atomicWrite(foldedPath(dir), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

// foldChapterSet is the ON-DISK chapter/scene key set for dir — the safe prune target for a
// fold-state write, mirroring corkChapterSet (corkboard.go). Keyed by corkKey, across all
// parts, so a folded chapter inside a real Part is never wrongly pruned.
func foldChapterSet(dir string) map[string]bool {
	s := map[string]bool{}
	v := resolveManuscript(dir, readEntries(dir))
	for _, p := range v.parts {
		for _, ch := range p.chapters {
			s[corkKey(ch)] = true
		}
	}
	return s
}
