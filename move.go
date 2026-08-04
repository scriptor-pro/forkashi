package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// moveDocument relocates a loose document `file` from srcDir to dstDir (the mover only ever
// offers loose filesystem files — an on-disk chapter FOLDER moves via moveFolder instead). It
// moves the file, removes it from the source manifest if srcDir/file was itself a chapter's
// sole text (a chapter folder browsed directly and its text file picked — folder left behind,
// now an empty/orphaned dir), and — if dstDir is a manuscript and asChapter is true — wraps the
// moved file into a brand-new same-slug chapter folder and appends it to the destination
// manifest as a bare chapter; otherwise it lands loose as a Resource. Refuses a no-op (same
// folder), a destination name collision, and an unreadable manifest on either side.
func moveDocument(srcDir, file, dstDir string, asChapter bool) error {
	if srcDir == dstDir {
		return fmt.Errorf("source and destination are the same folder")
	}
	// Refuse to touch a manuscript whose manifest is present-but-unreadable (don't guess).
	if _, present, err := readManifest(srcDir); present && err != nil {
		return fmt.Errorf("source manifest unreadable: %w", err)
	}
	if _, present, err := readManifest(dstDir); present && err != nil {
		return fmt.Errorf("destination manifest unreadable: %w", err)
	}
	dst := filepath.Join(dstDir, file)
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", file)
	}

	// Was srcDir itself a chapter folder whose sole text is this file? (reachable by drilling the
	// mover into a chapter folder and picking its text file directly.)
	wasChapterFolder := ""
	if sm, present, err := readManifest(filepath.Dir(srcDir)); err == nil && present {
		folder := filepath.Base(srcDir)
		for _, it := range sm.Items {
			if it.Chapter != nil && it.Chapter.Folder == folder {
				for _, t := range it.Chapter.Texts {
					if t.File == file {
						wasChapterFolder = folder
					}
				}
			}
		}
	}

	// Move the file first, so a failed move never leaves dangling manifest edits.
	if err := safeMove(filepath.Join(srcDir, file), dst); err != nil {
		return err
	}

	// Source manifest: drop the chapter item whose folder held this file (read-modify-write). The
	// file has already moved, so a re-read failure here is an inconsistent state — propagate it
	// rather than report success. The (now textless) chapter folder itself is left on disk.
	if wasChapterFolder != "" {
		srcManiDir := filepath.Dir(srcDir)
		sm, present, err := readManifest(srcManiDir)
		if err != nil {
			return fmt.Errorf("moved %s but could not update the source manifest: %w", file, err)
		}
		if present {
			var kept []manifestItem
			for _, it := range sm.Items {
				if it.Chapter != nil && it.Chapter.Folder == wasChapterFolder {
					continue
				}
				kept = append(kept, it)
			}
			sm.Items = kept
			if err := writeManifest(srcManiDir, sm); err != nil {
				return err
			}
		}
	}

	// Destination manifest: wrap the moved file into a new chapter folder and append it as a bare
	// chapter, when requested and the dest is a manuscript.
	if asChapter && hasManifest(dstDir) {
		dm, present, err := readManifest(dstDir)
		if err != nil {
			return err
		}
		if present {
			title := sectionTitle(file)
			// The title-derived slug can already be a chapter folder present in the
			// destination manifest — the os.Stat above only guards the manuscript
			// root, not this chapter subfolder, so a clear dst-root collision check
			// doesn't mean the derived chapter folder is free. Left unchecked,
			// MkdirAll below silently no-ops into the existing folder and two
			// manifest items end up sharing one folder on disk. Dedupe against every
			// chapter folder already in the destination manifest, "-2", "-3", …,
			// same numeric-suffix scheme as uniqueChapterFolder (structure.go) and
			// migratePlan's slug dedup (migration.go), via the shared
			// uniqueSlugAgainst helper.
			taken := map[string]bool{}
			for _, it := range dm.Items {
				if it.Chapter != nil {
					taken[it.Chapter.Folder] = true
				}
			}
			folder := uniqueSlugAgainst(slugify(title), taken)
			chDir := filepath.Join(dstDir, folder)
			if err := os.MkdirAll(chDir, 0o755); err != nil {
				return err
			}
			if err := safeMove(dst, filepath.Join(chDir, file)); err != nil {
				return err
			}
			dm.Items = append(dm.Items, manifestItem{Chapter: &manifestChapter{
				Folder: folder,
				Title:  title,
				Texts:  []manifestText{{File: file, Title: title}},
			}})
			if err := writeManifest(dstDir, dm); err != nil {
				return err
			}
		}
	}
	return nil
}

// safeMove moves a file from src to dst. It tries os.Rename (fast, same-volume) and, only on a
// cross-device error, falls back to copy-then-remove. dst's parent directory must already exist.
func safeMove(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// Cross-volume: copy the bytes atomically to dst, then remove the source.
	data, rerr := os.ReadFile(src)
	if rerr != nil {
		return rerr
	}
	if werr := atomicWrite(dst, data, 0o644); werr != nil {
		return werr
	}
	return os.Remove(src)
}

// moveFolder moves the directory srcDir INTO dstParent (as dstParent/base(srcDir)) via os.Rename.
// A manuscript's manifest.json rides along inside the folder. Refuses a name collision, moving a
// folder into itself or a descendant, and a no-op (it already lives in dstParent).
func moveFolder(srcDir, dstParent string) error {
	base := filepath.Base(srcDir)
	dst := filepath.Join(dstParent, base)
	if srcDir == dst {
		return fmt.Errorf("%s already lives there", base)
	}
	if withinRoot(dstParent, srcDir) {
		return fmt.Errorf("can't move a folder into itself")
	}
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("%s already exists in the destination", base)
	}
	if err := os.Rename(srcDir, dst); err != nil {
		if errors.Is(err, syscall.EXDEV) {
			return fmt.Errorf("can't move a folder across volumes yet — move its files individually")
		}
		return err
	}
	return nil
}
