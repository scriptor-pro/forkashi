package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"okashi/internal/textarea"
)

// propKind identifies one editable Properties field.
type propKind int

const (
	propTitle propKind = iota
	propAuthor
	propContact
	propWidth
	propSmartquotes
	propCover
)

// propertiesModel backs the Properties screen: editable project + personal metadata for one dir.
// Title is editable only for a manifest manuscript; a plain folder shows it read-only.
type propertiesModel struct {
	dir          string
	isManuscript bool

	title       textinput.Model
	author      textinput.Model
	width       textinput.Model
	contact     textarea.Model
	cover       textinput.Model
	smartquotes bool

	fields      []propKind // editable field order (Title omitted for a non-manuscript dir)
	focus       int        // index into fields
	editing     bool       // the focused field is capturing keys
	confirmExit bool       // esc-with-unsaved-changes prompt is up

	// snapshots of loaded values, for dirty-tracking and per-store save decisions
	origTitle       string
	origAuthor      string
	origContact     string
	origWidth       int
	origSmartquotes bool
	origCover       string
}

func newPropInput(val string, width int) textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Width = width
	ti.SetValue(val)
	return ti
}

// deriveTitle is the display title for dir: the manifest title if present, else the folder name.
func deriveTitle(dir string) string {
	if mani, ok, _ := readManifest(dir); ok && mani.Title != "" {
		return mani.Title
	}
	return filepath.Base(dir)
}

func newPropertiesModel(dir string) propertiesModel {
	eff := resolveSettings(dir)
	isMs := hasManifest(dir)
	title := deriveTitle(dir)

	ca := textarea.New()
	ca.Prompt = ""
	ca.ShowLineNumbers = false
	ca.CharLimit = 0
	ca.MaxHeight = 0
	ca.SetWidth(40)
	ca.SetHeight(4)
	ca.SetValue(eff.Contact)

	p := propertiesModel{
		dir:             dir,
		isManuscript:    isMs,
		title:           newPropInput(title, 40),
		author:          newPropInput(eff.Author, 40),
		width:           newPropInput(strconv.Itoa(eff.Width), 6),
		contact:         ca,
		cover:           newPropInput(eff.Cover, 40),
		smartquotes:     eff.Smartquotes,
		origTitle:       title,
		origAuthor:      eff.Author,
		origContact:     eff.Contact,
		origWidth:       eff.Width,
		origSmartquotes: eff.Smartquotes,
		origCover:       eff.Cover,
	}
	if isMs {
		p.fields = []propKind{propTitle, propAuthor, propContact, propWidth, propCover, propSmartquotes}
	} else {
		p.fields = []propKind{propAuthor, propContact, propWidth, propCover, propSmartquotes}
	}
	return p
}

// dirty reports whether any field differs from its loaded value.
func (p *propertiesModel) dirty() bool {
	if p.isManuscript && p.title.Value() != p.origTitle {
		return true
	}
	if p.author.Value() != p.origAuthor || p.contact.Value() != p.origContact {
		return true
	}
	if p.smartquotes != p.origSmartquotes {
		return true
	}
	if w, err := strconv.Atoi(strings.TrimSpace(p.width.Value())); err != nil || w != p.origWidth {
		return true
	}
	if p.cover.Value() != p.origCover {
		return true
	}
	return false
}

func (p *propertiesModel) focusInput() {
	switch p.fields[p.focus] {
	case propTitle:
		p.title.Focus()
	case propAuthor:
		p.author.Focus()
	case propWidth:
		p.width.Focus()
	case propContact:
		p.contact.Focus()
	case propCover:
		p.cover.Focus()
	}
}

func (p *propertiesModel) blurInputs() {
	p.title.Blur()
	p.author.Blur()
	p.width.Blur()
	p.contact.Blur()
	p.cover.Blur()
}

// save writes only the stores whose fields changed, preserving unrelated on-disk fields. It reports
// whether per-project (width/smartquotes) settings changed, so the caller can re-apply them live.
func (p *propertiesModel) save() (projectChanged bool, err error) {
	// Title → manifest (manuscript only; read-modify-write to keep items/schema intact).
	if p.isManuscript && p.title.Value() != p.origTitle {
		mani, ok, rerr := readManifest(p.dir)
		if rerr != nil {
			return false, rerr
		}
		if ok {
			mani.Title = p.title.Value()
			if werr := writeManifest(p.dir, mani); werr != nil {
				return false, werr
			}
		}
	}
	// Author/contact → personal global config.
	if p.author.Value() != p.origAuthor || p.contact.Value() != p.origContact {
		if werr := saveUserConfig(userConfigPath(), userConfig{Author: p.author.Value(), Contact: p.contact.Value()}); werr != nil {
			return false, werr
		}
	}
	// Width/smartquotes → per-project .okashi.json (overlay onto existing file fields).
	w, werr := strconv.Atoi(strings.TrimSpace(p.width.Value()))
	widthChanged := werr == nil && w != p.origWidth
	sqChanged := p.smartquotes != p.origSmartquotes
	coverChanged := p.cover.Value() != p.origCover
	if widthChanged || sqChanged || coverChanged {
		ps := loadProjectSettings(p.dir)
		if widthChanged {
			wv := clampWidth(w)
			ps.Width = &wv
		}
		if sqChanged {
			sv := p.smartquotes
			ps.Smartquotes = &sv
		}
		if coverChanged {
			cv := p.cover.Value()
			ps.Cover = &cv
		}
		if serr := saveProjectSettings(p.dir, ps); serr != nil {
			return false, serr
		}
		projectChanged = true
	}
	// Reset baselines so dirty clears.
	p.origTitle = p.title.Value()
	p.origAuthor = p.author.Value()
	p.origContact = p.contact.Value()
	if werr == nil {
		p.origWidth = clampWidth(w)
	}
	p.origSmartquotes = p.smartquotes
	p.origCover = p.cover.Value()
	return projectChanged, nil
}

// savePropertiesAndApply saves the Properties form and, if the edited dir is the open project,
// re-applies width/smartquotes and a possibly-retitled manifest to the live view. Returns the save
// error so callers that navigate away (the confirm-exit prompt) can refuse to on failure — leaving
// the user's edits intact rather than discarding them.
func (m *model) savePropertiesAndApply() error {
	if _, err := m.properties.save(); err != nil {
		m.status = "échec de l'enregistrement : " + err.Error()
		return err
	}
	if m.properties.dir == m.files.dir {
		m.files.SetDir(m.files.dir) // reflect a retitled manifest in the sidebar
		m.applyProjectSettings()    // reflect width/smartquotes live
	}
	m.status = "propriétés enregistrées"
	return nil
}

func (m model) updateProperties(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	p := &m.properties
	ks := key.String()

	if ks == "ctrl+c" {
		return m, tea.Quit
	}

	if p.confirmExit {
		switch ks {
		case "s":
			if err := m.savePropertiesAndApply(); err == nil {
				m.screen = screenHome
			} else {
				p.confirmExit = false // stay on the form: edits preserved, error shown in the status
			}
		case "d":
			m.screen = screenHome
		case "esc":
			p.confirmExit = false
		}
		return m, nil
	}

	if p.editing {
		return m.updatePropertiesEditing(key)
	}

	switch ks {
	case "esc":
		if p.dirty() {
			p.confirmExit = true
			m.status = "modifications non enregistrées — s enregistrer · d ignorer · esc annuler"
		} else {
			m.screen = screenHome
		}
	case "up", "shift+tab":
		p.focus = (p.focus - 1 + len(p.fields)) % len(p.fields)
	case "down", "tab":
		p.focus = (p.focus + 1) % len(p.fields)
	case "ctrl+s":
		m.savePropertiesAndApply()
	case "enter", " ":
		if p.fields[p.focus] == propSmartquotes {
			p.smartquotes = !p.smartquotes
		} else {
			p.editing = true
			p.focusInput()
		}
	}
	return m, nil
}

func (m model) updatePropertiesEditing(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.properties
	ks := key.String()
	kind := p.fields[p.focus]

	// Commit: enter/esc for single-line fields; esc only for the multiline contact block (so enter
	// inserts newlines there).
	commit := ks == "esc" || (kind != propContact && ks == "enter")
	if commit {
		p.editing = false
		p.blurInputs()
		if kind == propWidth {
			if n, err := strconv.Atoi(strings.TrimSpace(p.width.Value())); err != nil || n < 20 || n > 200 {
				p.width.SetValue(strconv.Itoa(p.origWidth))
				m.status = "la largeur doit être comprise entre 20 et 200"
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch kind {
	case propTitle:
		p.title, cmd = p.title.Update(key)
	case propAuthor:
		p.author, cmd = p.author.Update(key)
	case propWidth:
		p.width, cmd = p.width.Update(key)
	case propContact:
		p.contact, cmd = p.contact.Update(key)
	case propCover:
		p.cover, cmd = p.cover.Update(key)
	}
	return m, cmd
}

// propRow renders one "  Label   value" row; multiline values indent their continuation lines under
// the value column. A focused (non-editing) row is highlighted.
func propRow(label, val string, focused bool) string {
	const labelCol = 20
	lbl := fmt.Sprintf("  %-*s", labelCol-2, label)
	lines := strings.Split(val, "\n")
	out := []string{lbl + lines[0]}
	for _, l := range lines[1:] {
		out = append(out, strings.Repeat(" ", labelCol)+l)
	}
	s := strings.Join(out, "\n")
	if focused {
		s = selectedStyle.Render(s)
	}
	return s
}

func fieldVal(ti textinput.Model, editing bool) string {
	if editing {
		return ti.View()
	}
	return ti.Value()
}

func (m model) propertiesView() string {
	p := m.properties
	var rows []string

	if !p.isManuscript {
		ro := lipgloss.NewStyle().Foreground(subtle).Render(p.origTitle + "  (dossier — renommer sur le disque)")
		rows = append(rows, propRow("Titre", ro, false))
	}
	for i, kind := range p.fields {
		focused := i == p.focus
		editing := focused && p.editing
		var label, val string
		switch kind {
		case propTitle:
			label, val = "Titre", fieldVal(p.title, editing)
		case propAuthor:
			label, val = "Auteur", fieldVal(p.author, editing)
		case propContact:
			label = "Contact"
			if editing {
				val = p.contact.View()
			} else if v := p.contact.Value(); v != "" {
				val = v
			} else {
				val = lipgloss.NewStyle().Foreground(subtle).Render("(aucun)")
			}
		case propWidth:
			label, val = "Largeur", fieldVal(p.width, editing)
		case propSmartquotes:
			label = "Guillemets typo."
			val = "désactivé"
			if p.smartquotes {
				val = "activé"
			}
		case propCover:
			label = "Couverture"
			if editing {
				val = p.cover.View()
			} else if v := p.cover.Value(); v != "" {
				val = v
			} else {
				val = lipgloss.NewStyle().Foreground(subtle).Render("(aucune)")
			}
		}
		rows = append(rows, propRow(label, val, focused && !editing))
	}

	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("── propriétés · " + p.origTitle + " ")
	body := header + "\n\n" + strings.Join(rows, "\n")

	var b strings.Builder
	b.WriteString(lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, body))
	if p.confirmExit {
		bar := lipgloss.NewStyle().Foreground(accent).Render("modifications non enregistrées — s enregistrer · d ignorer · esc annuler")
		b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, bar))
		return b.String()
	}
	foot := lipgloss.NewStyle().Foreground(subtle).Render("⇥ champ · ⏎ éditer · espace bascule guillemets typo · ctrl+s enregistrer · esc retour · F1 aide")
	b.WriteString("\n" + lipgloss.PlaceHorizontal(m.width, lipgloss.Center, foot))
	return b.String()
}
