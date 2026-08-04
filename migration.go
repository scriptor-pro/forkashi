package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// manifestV1 is the pre-v2 flat shape: one item per chapter, File pointing
// directly at a manuscript-root .md file. Decoded independently of the current
// manifest type so migration can inspect a v1 file without readManifest's
// v2-only schemaVersion gate rejecting it first.
type manifestV1 struct {
	SchemaVersion int              `json:"schemaVersion"`
	Title         string           `json:"title"`
	Items         []manifestV1Item `json:"items"`
}

type manifestV1Item struct {
	File  string `json:"file"`
	Title string `json:"title"`
}

// readManifestV1 loads dir/manifest.json as the v1 shape, regardless of its
// schemaVersion field's actual value — the caller (needsMigration) decides
// whether migration applies.
func readManifestV1(dir string) (manifestV1, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if os.IsNotExist(err) {
		return manifestV1{}, false, nil
	}
	if err != nil {
		return manifestV1{}, true, err
	}
	var m manifestV1
	if err := json.Unmarshal(data, &m); err != nil {
		return manifestV1{}, true, err
	}
	return m, true, nil
}

// needsMigration reports whether dir has a manifest.json whose schemaVersion is
// exactly 1 — the one supported migration source. Any other version (including
// unreadable/malformed) is NOT this function's concern; readManifest's normal
// refuse-to-guess path handles those.
func needsMigration(dir string) bool {
	v1, present, err := readManifestV1(dir)
	return present && err == nil && v1.SchemaVersion == 1
}

// frenchOrdinalWords covers the first 20 chapters with a written-out French
// ordinal ("Chapitre un".."Chapitre vingt") for the empty-title safety net; a
// v1 manifest with more than 20 untitled chapters is not expected in practice
// (a valid v1 manifest already has items[].title filled in), so beyond this the
// title falls back to "Chapitre N" numerically rather than growing this table.
var frenchOrdinalWords = []string{
	"", "un", "deux", "trois", "quatre", "cinq", "six", "sept", "huit", "neuf", "dix",
	"onze", "douze", "treize", "quatorze", "quinze", "seize", "dix-sept", "dix-huit",
	"dix-neuf", "vingt",
}

func defaultChapterTitle(n int) string {
	if n >= 1 && n < len(frenchOrdinalWords) {
		return "Chapitre " + frenchOrdinalWords[n]
	}
	return fmt.Sprintf("Chapitre %d", n)
}

// migrationMove is one file relocation: a v1 root file into its new v2 chapter
// folder.
type migrationMove struct {
	fromFile string
	toFolder string
	toFile   string
}

// migrationStep is a fully-computed migration: the file moves to perform, and
// the v2 manifest to write once every move has succeeded.
type migrationStep struct {
	moves []migrationMove
	out   manifest
}

// migratePlan computes the migration without touching disk: for each v1 item,
// the folder/file slug is derived from its title (falling back to a default
// "Chapitre N" title only if the v1 title is empty — the safety net documented
// in the design; a valid v1 manifest already has titles filled in).
//
// v1 never enforced title uniqueness (its identity was the filename, not the
// title), so two v1 items can slugify to the same folder. Left undetected,
// the second migrationMove's os.Rename would land in the same folder as the
// first and silently overwrite its content. uniqueSlugAgainst below dedupes
// against every slug already claimed earlier in this same plan —
// deterministically, in items[] order — appending "-2", "-3", … the same way
// uniqueChapterFolder (structure.go) dedupes against disk for the normal
// add-chapter path; here nothing is on disk yet, so uniqueness is tracked
// purely against the in-progress plan.
func migratePlan(dir string, v1 manifestV1) (migrationStep, error) {
	var step migrationStep
	step.out.Title = v1.Title
	taken := map[string]bool{}
	for i, it := range v1.Items {
		title := it.Title
		if title == "" {
			title = defaultChapterTitle(i + 1)
		}
		slug := uniqueSlugAgainst(slugify(title), taken)
		taken[slug] = true
		toFile := slug + ".md"
		step.moves = append(step.moves, migrationMove{
			fromFile: it.File,
			toFolder: slug,
			toFile:   toFile,
		})
		step.out.Items = append(step.out.Items, manifestItem{
			Chapter: &manifestChapter{
				Folder: slug,
				Title:  title,
				Texts:  []manifestText{{File: toFile, Title: title}},
			},
		})
	}
	return step, nil
}

// uniqueSlugAgainst returns base unchanged if it's not yet in taken, otherwise
// the first "base-2", "base-3", … not in taken. Shared by migratePlan (taken =
// slugs already claimed earlier in the same plan; nothing is on disk yet) and
// moveDocument's promote-to-chapter path (taken = chapter folders already in
// the destination manifest).
func uniqueSlugAgainst(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// migrateV1ToV2 executes the migration for dir: moves each v1 chapter file into
// its own new folder, then writes the v2 manifest ONCE at the end — only if
// every move succeeded. If any move fails partway through, migration stops and
// returns the error without writing the manifest, so the original v1 manifest
// (and whatever files were already moved) is the only state change on disk; the
// caller is expected to surface the error rather than retry automatically.
func migrateV1ToV2(dir string, v1 manifestV1) error {
	plan, err := migratePlan(dir, v1)
	if err != nil {
		return err
	}
	for _, mv := range plan.moves {
		fromPath := filepath.Join(dir, mv.fromFile)
		if _, err := os.Stat(fromPath); err != nil {
			return fmt.Errorf("migration: %s: %w", mv.fromFile, err)
		}
		toDir := filepath.Join(dir, mv.toFolder)
		if err := os.MkdirAll(toDir, 0o755); err != nil {
			return fmt.Errorf("migration: mkdir %s: %w", mv.toFolder, err)
		}
		if err := os.Rename(fromPath, filepath.Join(toDir, mv.toFile)); err != nil {
			return fmt.Errorf("migration: move %s: %w", mv.fromFile, err)
		}
	}
	return writeManifest(dir, plan.out)
}
