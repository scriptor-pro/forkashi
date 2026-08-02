package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type moverEntryKind int

const (
	moverPickSource = 0
	moverPickDest   = 1
)

const (
	moverMoveHere moverEntryKind = iota // "→ move into <current folder>"
	moverUp                             // "‹ .."
	moverFolder                         // "▸ name/"
	moverFile                           // "› name" (a document — selectable as the source)
	moverMoveThis                       // "→ move this folder (<current>)" (pick the browsed folder as the source)
	moverSource                         // a library source root (destination target)
)

type moverEntry struct {
	name, path string
	kind       moverEntryKind
}

// moverBoundingSource returns the reachable library source whose root contains (or equals) dir.
func (m model) moverBoundingSource(dir string) (source, bool) {
	var best source
	found := false
	for _, s := range m.sources {
		if !s.reachable() {
			continue
		}
		r := s.root()
		if dir == r || withinRoot(dir, r) {
			// Prefer the most-specific (longest-root) match, so a source nested inside
			// another source's tree binds to the nested one, not the outer.
			if !found || len(r) > len(best.root()) {
				best, found = s, true
			}
		}
	}
	return best, found
}

// enterMover opens the file mover for the file pane's selected entry (contextual entry). The
// destination browser starts at the active source root.
func (m *model) enterMover() {
	if len(m.files.entries) == 0 {
		return
	}
	e := m.files.entries[m.files.selected]
	if e.name == ".." {
		return
	}
	m.moverSource = filepath.Join(m.files.dir, e.name)
	m.moverIsDir = e.isDir
	m.moverFromDir = m.files.dir
	m.moverDestDir = m.activeSourceRoot()
	m.moverSel = 0
	m.moverConfirm = false
	m.moverAsChapter = true
	m.moverError = ""
	m.moverReturn = screenWriting
	m.moverPhase = moverPickDest
	m.moverReload()
	m.screen = screenMover
}

// enterMoverStandalone opens the mover with no source chosen — the left pane browses the active
// source so the user picks the file/folder to move (home action / global entry).
func (m *model) enterMoverStandalone() {
	m.moverPhase = moverPickSource
	m.moverSrcDir = m.activeSourceRoot()
	m.moverSrcSel = 0
	m.moverConfirm = false
	m.moverAsChapter = true
	m.moverError = ""
	m.moverReturn = screenWriting
	m.moverSrcReload()
	m.screen = screenMover
}

// moverSrcReload rebuilds the source-picker rows for moverSrcDir: a "move this folder" row (when
// below the source root), a ".." row (bounded to the root), subfolders (drillable), then files.
func (m *model) moverSrcReload() {
	root := m.activeSourceRoot()
	var rows []moverEntry
	below := m.moverSrcDir != root && withinRoot(m.moverSrcDir, root)
	if below {
		rows = append(rows, moverEntry{name: filepath.Base(m.moverSrcDir), path: m.moverSrcDir, kind: moverMoveThis})
		rows = append(rows, moverEntry{name: "..", path: filepath.Dir(m.moverSrcDir), kind: moverUp})
	}
	if ents, err := os.ReadDir(m.moverSrcDir); err == nil {
		var dirs, files []moverEntry
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			p := filepath.Join(m.moverSrcDir, e.Name())
			if e.IsDir() {
				dirs = append(dirs, moverEntry{name: e.Name(), path: p, kind: moverFolder})
			} else if m.files.allowed[strings.ToLower(filepath.Ext(e.Name()))] {
				files = append(files, moverEntry{name: e.Name(), path: p, kind: moverFile})
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
		sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
		rows = append(rows, dirs...)
		rows = append(rows, files...)
	}
	m.moverSrcEntries = rows
	if m.moverSrcSel >= len(rows) {
		m.moverSrcSel = len(rows) - 1
	}
	if m.moverSrcSel < 0 {
		m.moverSrcSel = 0
	}
}

// pickMoverSource selects e as the thing to move (a file, or a folder via "move this folder") and
// advances to the destination phase.
func (m *model) pickMoverSource(e moverEntry) {
	switch e.kind {
	case moverFile:
		m.moverSource = e.path
		m.moverIsDir = false
		m.moverFromDir = filepath.Dir(e.path)
	case moverMoveThis:
		m.moverSource = e.path
		m.moverIsDir = true
		m.moverFromDir = filepath.Dir(e.path)
	default:
		return
	}
	m.moverError = "" // a fresh source pick starts clean — don't carry a prior failure's error
	m.moverPhase = moverPickDest
	m.moverDestDir = m.activeSourceRoot()
	m.moverSel = 0
	m.moverReload()
}

// moverReload rebuilds the destination browser rows. When moverDestDir == "" the list shows all
// reachable library sources (moverSource rows). Otherwise it shows the usual move-here / .. /
// subfolders list, with ".." ascending to the sources list when at a source root.
func (m *model) moverReload() {
	var rows []moverEntry
	if m.moverDestDir == "" {
		for _, s := range m.sources {
			if s.reachable() {
				rows = append(rows, moverEntry{name: s.Name, path: s.root(), kind: moverSource})
			}
		}
	} else {
		rows = append(rows, moverEntry{name: filepath.Base(m.moverDestDir), path: m.moverDestDir, kind: moverMoveHere})
		if src, ok := m.moverBoundingSource(m.moverDestDir); ok {
			up := filepath.Dir(m.moverDestDir)
			if m.moverDestDir == src.root() {
				up = "" // step up to the sources list
			}
			rows = append(rows, moverEntry{name: "..", path: up, kind: moverUp})
		}
		if ents, err := os.ReadDir(m.moverDestDir); err == nil {
			var dirs []moverEntry
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, moverEntry{name: e.Name(), path: filepath.Join(m.moverDestDir, e.Name()), kind: moverFolder})
				}
			}
			sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
			rows = append(rows, dirs...)
		}
	}
	m.moverEntries = rows
	if m.moverSel >= len(rows) {
		m.moverSel = len(rows) - 1
	}
	if m.moverSel < 0 {
		m.moverSel = 0
	}
}

func (m model) updateMover(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.moverConfirm {
			fileIntoManuscript := !m.moverIsDir && hasManifest(m.moverDestDir)
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "left", "right", "tab":
				if fileIntoManuscript {
					m.moverAsChapter = !m.moverAsChapter
				}
			case "y", "enter":
				if err := m.applyMove(); err != nil {
					m.moverError = "échec du déplacement : " + err.Error()
					m.moverConfirm = false
					m.moverReload()
					return m, nil
				}
				m.moverError = ""
				m.status = "déplacé " + filepath.Base(m.moverSource)
				m.moverConfirm = false
				m.files.SetDir(m.files.dir) // refresh the pane (source may have left it)
				m.screen = m.moverReturn
				return m, nil
			case "esc", "n":
				m.moverConfirm = false
				return m, nil
			}
			return m, nil
		}
		if m.moverPhase == moverPickSource {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.screen = m.moverReturn
				return m, nil
			case "up", "k":
				if m.moverSrcSel > 0 {
					m.moverSrcSel--
				}
			case "down", "j":
				if m.moverSrcSel < len(m.moverSrcEntries)-1 {
					m.moverSrcSel++
				}
			case "enter":
				if m.moverSrcSel < 0 || m.moverSrcSel >= len(m.moverSrcEntries) {
					return m, nil
				}
				e := m.moverSrcEntries[m.moverSrcSel]
				switch e.kind {
				case moverUp, moverFolder:
					m.moverError = ""
					m.moverSrcDir = e.path
					m.moverSrcSel = 0
					m.moverSrcReload()
				case moverFile, moverMoveThis:
					m.pickMoverSource(e)
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = m.moverReturn
			return m, nil
		case "up", "k":
			if m.moverSel > 0 {
				m.moverSel--
			}
		case "down", "j":
			if m.moverSel < len(m.moverEntries)-1 {
				m.moverSel++
			}
		case "enter":
			if m.moverSel < 0 || m.moverSel >= len(m.moverEntries) {
				return m, nil
			}
			e := m.moverEntries[m.moverSel]
			switch e.kind {
			case moverUp, moverFolder, moverSource:
				m.moverError = ""
				m.moverDestDir = e.path
				m.moverSel = 0
				m.moverReload()
			case moverMoveHere:
				m.moverAsChapter = true // default to "chapter" each time the confirm opens
				m.moverConfirm = true
			}
			return m, nil
		}
	}
	return m, nil
}

// applyMove performs the chosen move via the chunk-1 engine. A folder → moveFolder; a file →
// moveDocument (asChapter only when the destination is a manuscript AND the user chose chapter).
func (m *model) applyMove() error {
	if m.moverIsDir {
		return moveFolder(m.moverSource, m.moverDestDir)
	}
	asChapter := m.moverAsChapter && hasManifest(m.moverDestDir)
	return moveDocument(m.moverFromDir, filepath.Base(m.moverSource), m.moverDestDir, asChapter)
}

func (m model) moverView() string {
	// LEFT pane: the source browser (pick phase) or the chosen source (dest phase).
	var leftInner string
	var leftTitle string
	if m.moverPhase == moverPickSource {
		leftTitle = "DÉPLACER · choisir un fichier"
		visRows := m.height - 8
		if visRows < 1 {
			visRows = 1
		}
		off := homeWindowOffset(len(m.moverSrcEntries), m.moverSrcSel, visRows)
		var lines []string
		for i := off; i < len(m.moverSrcEntries) && len(lines) < visRows; i++ {
			e := m.moverSrcEntries[i]
			var text string
			switch e.kind {
			case moverMoveThis:
				text = "→ déplacer ce dossier"
			case moverUp:
				text = "‹ .."
			case moverFolder:
				text = "▸ " + e.name + "/"
			default:
				text = "› " + e.name
			}
			if i == m.moverSrcSel {
				text = selectedStyle.Render(text)
			}
			lines = append(lines, text)
		}
		leftInner = strings.Join(lines, "\n")
	} else {
		leftTitle = "DÉPLACER"
		kindLabel := "fichier"
		article := "du "
		if m.moverIsDir {
			kindLabel = "dossier"
		}
		leftInner = "déplacement " + article + kindLabel + " :\n" + filepath.Base(m.moverSource) + "\n\ndepuis : " + filepath.Base(m.moverFromDir)
	}
	leftPanel := framedPanel(leftTitle, leftInner, max(26, min(m.width-34, 40)), max(len(strings.Split(leftInner, "\n"))+2, 8), "")

	// RIGHT pane: dim placeholder in pick-source phase; destination browser in pick-dest phase.
	var rightPanel string
	rightW := max(30, min(m.width-30, 44))
	if m.moverPhase == moverPickSource {
		rightInner := homeDim("choisissez d'abord une source →")
		rightPanel = framedPanel("VERS", rightInner, rightW, 4, "")
	} else {
		visRows := m.height - 8
		if visRows < 1 {
			visRows = 1
		}
		off := homeWindowOffset(len(m.moverEntries), m.moverSel, visRows)
		var rows []string
		for i := off; i < len(m.moverEntries) && len(rows) < visRows; i++ {
			e := m.moverEntries[i]
			var text string
			switch e.kind {
			case moverMoveHere:
				text = "→ déplacer dans " + e.name + "/"
			case moverUp:
				text = "‹ .."
			case moverSource:
				text = "◆ " + e.name
			default:
				text = "▸ " + e.name + "/"
			}
			if i == m.moverSel {
				text = selectedStyle.Render(text)
			}
			rows = append(rows, text)
		}
		toTitle := "VERS · SOURCES"
		if m.moverDestDir != "" {
			toTitle = "VERS · " + filepath.Base(m.moverDestDir)
			if src, ok := m.moverBoundingSource(m.moverDestDir); ok {
				if m.moverDestDir == src.root() {
					toTitle = "VERS · " + src.Name
				} else {
					toTitle = "VERS · " + src.Name + "/" + filepath.Base(m.moverDestDir)
				}
			}
		}
		rightPanel = framedPanel(toTitle, strings.Join(rows, "\n"), rightW, len(rows)+2, "")
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, "  ", rightPanel)
	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	if m.moverConfirm {
		dst := filepath.Base(m.moverDestDir)
		var line string
		if !m.moverIsDir && hasManifest(m.moverDestDir) {
			chapter, resource := "( ) chapitre", "( ) ressource"
			if m.moverAsChapter {
				chapter = "(•) chapitre"
			} else {
				resource = "(•) ressource"
			}
			line = "déplacer " + filepath.Base(m.moverSource) + " → " + dst + " comme  " + chapter + "  " + resource + "   ←→ basculer · y déplacer · esc annuler"
		} else {
			line = "déplacer " + filepath.Base(m.moverSource) + " → " + dst + "?   y déplacer · esc annuler"
		}
		bar := lipgloss.NewStyle().Foreground(accent).Render(line)
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	if m.moverError != "" {
		errLine := lipgloss.NewStyle().Foreground(errColor).Render("⚠ " + m.moverError + "   (parcourir pour fermer · esc annuler)")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, errLine))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("↑↓ parcourir · entrée ouvrir/sélectionner · .. → sources · esc annuler")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
