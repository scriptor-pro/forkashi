package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// snapshot is one entry in a file's .okashi-bak/ ring.
type snapshot struct {
	name string    // filename within .okashi-bak (<base>.<YYYYMMDD-HHMMSS>)
	when time.Time // parsed from the timestamp suffix
}

// snapshotsModel backs the Snapshots screen: browse, preview, and restore a file's timestamped
// backups (the same .okashi-bak/ ring the autosave safety net writes).
type snapshotsModel struct {
	file           string
	base           string
	bakDir         string
	snaps          []snapshot
	sel            int
	previewing     bool
	preview        string
	confirmRestore bool
	markA          int // first endpoint for a two-snapshot diff (-1 = none)
}

// listSnapshots reads a file's .okashi-bak/ ring, newest first. Missing dir → nil.
func listSnapshots(file string) []snapshot {
	base := filepath.Base(file)
	bakDir := filepath.Join(filepath.Dir(file), ".okashi-bak")
	entries, err := os.ReadDir(bakDir)
	if err != nil {
		return nil
	}
	const stampLen = len("20060102-150405")
	var snaps []snapshot
	for _, e := range entries {
		name := e.Name()
		// Match only "<base>.<15-char stamp>" (mirrors pruneBackups) so a shorter base can't
		// pick up a longer base's snapshots sharing the dir.
		if e.IsDir() || !strings.HasPrefix(name, base+".") || len(name) != len(base)+1+stampLen {
			continue
		}
		when, perr := time.ParseInLocation("20060102-150405", name[len(base)+1:], time.Local)
		if perr != nil {
			continue
		}
		snaps = append(snaps, snapshot{name: name, when: when})
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].when.After(snaps[j].when) })
	return snaps
}

func newSnapshotsModel(file string) snapshotsModel {
	return snapshotsModel{
		file:   file,
		base:   filepath.Base(file),
		bakDir: filepath.Join(filepath.Dir(file), ".okashi-bak"),
		snaps:  listSnapshots(file),
		markA:  -1,
	}
}

// loadPreview returns the selected snapshot's content (or a placeholder).
func (s *snapshotsModel) loadPreview() string {
	if s.sel < 0 || s.sel >= len(s.snaps) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(s.bakDir, s.snaps[s.sel].name))
	if err != nil {
		return "(snapshot illisible)"
	}
	return string(data)
}

// enterSnapshots opens the Snapshots screen for the selected sidebar file, flushing the buffer
// first when it's the open, dirty file so the snapshots reflect what's on screen.
func (m *model) enterSnapshots() {
	file, ok := m.files.selectedFile()
	if !ok {
		m.status = "sélectionnez un fichier pour voir ses snapshots"
		return
	}
	if file == m.currentFile && m.dirty {
		m.save()
	}
	m.snapshots = newSnapshotsModel(file)
	m.screen = screenSnapshots
	if len(m.snapshots.snaps) == 0 {
		m.status = "aucun snapshot pour l'instant — n pour en créer un"
	} else {
		m.status = ""
	}
}

// restoreSelectedSnapshot backs up the current version, then overwrites the file with the selected
// snapshot; if the file is open, it reloads the buffer.
func (m *model) restoreSelectedSnapshot() {
	s := &m.snapshots
	s.confirmRestore = false
	if s.sel < 0 || s.sel >= len(s.snaps) {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.bakDir, s.snaps[s.sel].name))
	if err != nil {
		m.status = "échec de la restauration : " + err.Error()
		return
	}
	snapshotBackup(s.file) // safety: capture the current version before overwriting
	if err := atomicWrite(s.file, data, 0o644); err != nil {
		m.status = "échec de la restauration : " + err.Error()
		return
	}
	if s.file == m.currentFile {
		m.loadFile(s.file) // reload the live buffer from the restored file
	}
	m.status = "snapshot restauré du " + s.snaps[s.sel].when.Format("2006-01-02 15:04:05")
	m.screen = screenWriting
	m.focus = focusSidebar
}

func (m model) updateSnapshots(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	s := &m.snapshots
	ks := key.String()

	if ks == "ctrl+c" {
		return m, tea.Quit
	}

	if s.confirmRestore {
		switch ks {
		case "y":
			m.restoreSelectedSnapshot()
		case "esc", "n":
			s.confirmRestore = false
		}
		return m, nil
	}

	switch ks {
	case "esc", "b":
		if s.previewing {
			s.previewing = false
			s.preview = ""
		} else if s.markA >= 0 {
			s.markA = -1
			m.status = "marque de diff effacée"
		} else {
			m.screen = screenWriting
			m.focus = focusSidebar
		}
	case "up", "k":
		if s.sel > 0 {
			s.sel--
			if s.previewing {
				s.preview = s.loadPreview()
			}
		}
	case "down", "j":
		if s.sel < len(s.snaps)-1 {
			s.sel++
			if s.previewing {
				s.preview = s.loadPreview()
			}
		}
	case " ":
		if len(s.snaps) > 0 {
			s.previewing = !s.previewing
			if s.previewing {
				s.preview = s.loadPreview()
			}
		}
	case "n":
		snapshotBackup(s.file)
		s.snaps = listSnapshots(s.file)
		s.sel = 0
		m.status = "snapshot créé"
	case "enter":
		if len(s.snaps) > 0 {
			s.confirmRestore = true
		}
	case "d":
		if len(s.snaps) > 0 {
			m.openDiffSnapshotVsCurrent()
		}
	case "D":
		if len(s.snaps) > 0 {
			if s.markA < 0 {
				s.markA = s.sel
				m.status = "marqué A · choisissez un autre snapshot, D pour diff (esc efface)"
			} else {
				m.openDiffTwoSnapshots(s.markA, s.sel)
				s.markA = -1
			}
		}
	}
	return m, nil
}

// openDiffSnapshotVsCurrent diffs the selected snapshot against the file's current on-disk content.
func (m *model) openDiffSnapshotVsCurrent() {
	s := &m.snapshots
	if s.sel < 0 || s.sel >= len(s.snaps) {
		return
	}
	snap := s.snaps[s.sel]
	aContent, err := os.ReadFile(filepath.Join(s.bakDir, snap.name))
	if err != nil {
		m.status = "impossible de lire le snapshot"
		return
	}
	bContent, _ := os.ReadFile(s.file) // current version on disk (buffer was flushed on entry)
	m.diff = newDiffModel(snap.when.Format("2006-01-02 15:04:05"), string(aContent), "current", string(bContent))
	m.screen = screenDiff
}

// openDiffTwoSnapshots diffs two snapshots against each other.
func (m *model) openDiffTwoSnapshots(ai, bi int) {
	s := &m.snapshots
	if ai < 0 || ai >= len(s.snaps) || bi < 0 || bi >= len(s.snaps) {
		return
	}
	aSnap, bSnap := s.snaps[ai], s.snaps[bi]
	aC, err1 := os.ReadFile(filepath.Join(s.bakDir, aSnap.name))
	bC, err2 := os.ReadFile(filepath.Join(s.bakDir, bSnap.name))
	if err1 != nil || err2 != nil {
		m.status = "impossible de lire les snapshots"
		return
	}
	m.diff = newDiffModel(
		aSnap.when.Format("2006-01-02 15:04:05"), string(aC),
		bSnap.when.Format("2006-01-02 15:04:05"), string(bC),
	)
	m.screen = screenDiff
}

// relTime renders a compact "(… ago)" for a snapshot time against now. Empty when now is unset.
func relTime(now, when time.Time) string {
	if now.IsZero() {
		return ""
	}
	d := now.Sub(when)
	switch {
	case d < time.Minute:
		return "(just now)"
	case d < time.Hour:
		return fmt.Sprintf("(%dm ago)", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("(%dh ago)", int(d.Hours()))
	default:
		return fmt.Sprintf("(%dd ago)", int(d.Hours())/24)
	}
}

func (m model) snapshotsView() string {
	s := m.snapshots
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── snapshots · " + s.base + " ")
	var b strings.Builder

	if s.previewing {
		lines := strings.Split(s.preview, "\n")
		maxLines := m.height - 6
		if maxLines < 1 {
			maxLines = 1
		}
		if len(lines) > maxLines {
			lines = lines[:maxLines]
			lines = append(lines, lipgloss.NewStyle().Foreground(subtle).Render("… (tronqué)"))
		}
		stamp := ""
		if s.sel >= 0 && s.sel < len(s.snaps) {
			stamp = s.snaps[s.sel].when.Format("2006-01-02 15:04:05")
		}
		body := header + "\n" + lipgloss.NewStyle().Foreground(subtle).Render(stamp) + "\n\n" + strings.Join(lines, "\n")
		b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Top, body))
		foot := lipgloss.NewStyle().Foreground(subtle).Render("espace / esc retour à la liste · ↑↓ autres snapshots")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
		return b.String()
	}

	var rows []string
	if len(s.snaps) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(subtle).Render("  (aucun snapshot — appuyez sur n pour en créer un)"))
	} else {
		for i, sn := range s.snaps {
			label := "  " + sn.when.Format("2006-01-02 15:04:05")
			if rel := relTime(m.now, sn.when); rel != "" {
				label += "   " + rel
			}
			if i == s.sel {
				label = selectedStyle.Render(label)
			}
			rows = append(rows, label)
		}
	}
	body := header + "\n\n" + strings.Join(rows, "\n")
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))

	if s.confirmRestore {
		bar := lipgloss.NewStyle().Foreground(accent).Render(
			"restaurer ce snapshot ? la version actuelle est d'abord sauvegardée — y restaurer · esc annuler")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	hint := "↑↓ sélection · space aperçu · d diff vs actuel · D diff deux · ⏎ restaurer · n nouveau · esc retour"
	if s.markA >= 0 {
		hint = "A marqué (" + s.snaps[s.markA].when.Format("15:04:05") + ") · D sur un autre pour diff · esc efface"
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render(hint)
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
