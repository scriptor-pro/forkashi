# Notes de révision dans l'inspecteur (onglet Mots) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Afficher, dans l'onglet Mots de l'inspecteur (`ctrl+y`), les notes de révision (`n`) déjà
attachées au fichier ouvert dans l'éditeur — lecture seule, sous les sections existantes.

**Architecture:** Une nouvelle fonction pure `renderNotesSection(notes []note, width int) string` dans
`inspector.go`, suivant le pattern déjà établi par `renderOutline`. `inspectorModel.View()` gagne un
paramètre `notes []note` en dernière position, utilisé uniquement dans la branche `tabWords`. `main.go`
charge les notes du fichier courant via `loadNotes(m.currentFile)` (déjà existant dans `notes.go`,
inchangé) au même endroit où `doc`/`proj` sont déjà calculés, juste avant l'appel à `View()`.

**Tech Stack:** Go, `lipgloss` (styles), `x/ansi` (troncature ANSI-safe). Package `main`, test standard
`go test`.

## Global Constraints

- Aucune interaction (clic/édition/suppression) sur les notes depuis l'inspecteur — lecture seule
  uniquement ; pour éditer, l'utilisateur passe toujours par l'écran Notes (`n`).
- Aucun changement au format du sidecar `.okashi-notes/<base>.json`, ni aux types `note`/`notesFile`
  (`notes.go`).
- Aucun plafond sur le nombre de notes affichées (contrairement à "Surutilisés" qui plafonne à 5).
- La section Notes n'apparaît que si un fichier est ouvert (`doc.words > 0`) — même garde que les
  sections Lisibilité/Surutilisés existantes ; pas de section Notes hors contexte fichier.
- Aucun nouveau raccourci clavier.
- Build : `go build ./...`. Vet : `go vet ./...`. Tests : `go test ./...`.

---

## Task 1: `renderNotesSection` — la fonction de rendu pure

**Files:**
- Modify: `inspector.go` (ajoute la fonction, après `renderOutline`, ~ligne 139)
- Test: `inspector_test.go`

**Interfaces:**
- Consumes: le type `note` déjà défini dans `notes.go:24-33` (champs `ID`, `Scope`, `Text`,
  `CreatedAt`, `Quote`, `Prefix`, `Suffix`, `LineHint`) ; `sectionHeader(label string, width int) string`
  déjà défini dans `inspector.go:18`.
- Produces: `func renderNotesSection(notes []note, width int) string` — consommé par Task 2
  (`inspectorModel.View()`).

- [ ] **Step 1: Write the failing tests**

Ajoute à `inspector_test.go` (à la fin du fichier, avant ou après les tests de lisibilité existants) :

```go
func TestRenderNotesSectionEmpty(t *testing.T) {
	out := renderNotesSection(nil, 28)
	if !strings.Contains(out, "NOTES") {
		t.Fatalf("empty notes section missing header:\n%s", out)
	}
	if !strings.Contains(out, "aucune note") {
		t.Fatalf("empty notes section missing placeholder:\n%s", out)
	}
}

func TestRenderNotesSectionSingleLineNote(t *testing.T) {
	out := renderNotesSection([]note{{ID: "n1", Text: "Vérifier la continuité du prénom"}}, 40)
	if !strings.Contains(out, "Vérifier la continuité du prénom") {
		t.Fatalf("notes section missing note text:\n%s", out)
	}
	if strings.Contains(out, "aucune note") {
		t.Fatalf("notes section should not show the empty placeholder when notes exist:\n%s", out)
	}
}

func TestRenderNotesSectionMultiLineNoteShowsFirstLineOnly(t *testing.T) {
	out := renderNotesSection([]note{{ID: "n1", Text: "Rythme à retravailler\nvoir chapitre 3"}}, 40)
	if !strings.Contains(out, "Rythme à retravailler …") {
		t.Fatalf("multi-line note should show first line + ellipsis marker:\n%s", out)
	}
	if strings.Contains(out, "voir chapitre 3") {
		t.Fatalf("multi-line note should NOT show the second line:\n%s", out)
	}
}

func TestRenderNotesSectionTruncatesToWidth(t *testing.T) {
	long := strings.Repeat("mot ", 30) // far longer than any reasonable inspector width
	out := renderNotesSection([]note{{ID: "n1", Text: long}}, 20)
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 20 {
			t.Fatalf("line exceeds width 20: %q (width %d)", line, lipgloss.Width(line))
		}
	}
}

func TestRenderNotesSectionListsMultipleNotesInOrder(t *testing.T) {
	out := renderNotesSection([]note{
		{ID: "n1", Text: "Première note"},
		{ID: "n2", Text: "Deuxième note"},
	}, 40)
	i1 := strings.Index(out, "Première note")
	i2 := strings.Index(out, "Deuxième note")
	if i1 == -1 || i2 == -1 {
		t.Fatalf("both notes should appear:\n%s", out)
	}
	if i1 > i2 {
		t.Fatalf("notes should appear in storage order (first note first):\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run TestRenderNotesSection -v 2>&1 | tail -40`
Expected: FAIL — `renderNotesSection` undefined (does not compile).

- [ ] **Step 3: Implement `renderNotesSection`**

Ajoute dans `inspector.go`, juste après `renderOutline` (après la ligne `}` qui ferme cette fonction,
~ligne 139) :

```go
// renderNotesSection shows the revision notes attached to the current file, read-only: a
// "NOTES" header, then either a subtle empty-state hint or one line per note (first line of
// its text, "…" appended if the note continues beyond that line), each truncated to width.
func renderNotesSection(notes []note, width int) string {
	var b strings.Builder
	b.WriteString(sectionHeader("Notes", width))
	if len(notes) == 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(subtle).Render("(aucune note — n pour en ajouter)"))
		return b.String()
	}
	for _, nt := range notes {
		first := nt.Text
		if idx := strings.IndexByte(first, '\n'); idx >= 0 {
			first = first[:idx] + " …"
		}
		b.WriteString("\n  " + ansi.Truncate(first, width-2, "…"))
	}
	return b.String()
}
```

Note d'implémentation : la troncature réserve 2 colonnes pour le préfixe `"  "` (indentation à deux
espaces, cohérente avec les autres lignes de contenu de l'inspecteur — voir `kvRow`, qui préfixe aussi
`"  "` avant chaque label), donc `ansi.Truncate(first, width-2, "…")` et non `width`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestRenderNotesSection -v 2>&1 | tail -60`
Expected: all 5 tests PASS.

- [ ] **Step 5: Build and vet**

Run: `go build ./... && go vet ./...`
Expected: clean, no errors, no warnings.

- [ ] **Step 6: Commit**

```bash
git add inspector.go inspector_test.go
git commit -m "$(cat <<'EOF'
Ajoute renderNotesSection() pour afficher les notes de révision dans l'inspecteur

Fonction pure, non encore câblée dans View() — prépare l'onglet Mots à
montrer les notes du fichier ouvert sous les sections existantes.
EOF
)"
```

---

## Task 2: Câbler `notes []note` dans `inspectorModel.View()`

**Files:**
- Modify: `inspector.go:546` (signature et corps de `View`)
- Modify: `main.go:1744` (site d'appel)
- Modify: `inspector_test.go` (les 12 appels existants à `.View(...)`)
- Test: `inspector_test.go`

**Interfaces:**
- Consumes: `renderNotesSection(notes []note, width int) string` de la Task 1 ; `loadNotes(file string)
  []note` déjà existant dans `notes.go:48` (inchangé).
- Produces: nouvelle signature
  `func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string, goals
  goalStats, analysis analysisState, notes []note) string` — c'est la signature finale, plus rien
  ne la modifie dans les tâches suivantes.

- [ ] **Step 1: Write the failing tests**

Ajoute à `inspector_test.go` :

```go
func TestInspectorViewShowsNotesWhenPresent(t *testing.T) {
	in := inspectorModel{visible: true}
	notes := []note{{ID: "n1", Text: "Continuité à vérifier"}}
	out := in.View(28, docStats{words: 10}, projStats{words: 10}, "", goalStats{}, analysisState{}, notes)
	if !strings.Contains(out, "Continuité à vérifier") {
		t.Fatalf("inspector Mots tab should show note text when notes exist:\n%s", out)
	}
}

func TestInspectorViewShowsPlaceholderWhenNoNotes(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{words: 10}, projStats{words: 10}, "", goalStats{}, analysisState{}, nil)
	if !strings.Contains(out, "aucune note") {
		t.Fatalf("inspector Mots tab should show the empty-notes placeholder when a file is open but has no notes:\n%s", out)
	}
}

func TestInspectorViewOmitsNotesSectionWhenNoFileOpen(t *testing.T) {
	in := inspectorModel{visible: true}
	out := in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{}, nil)
	if strings.Contains(out, "NOTES") {
		t.Fatalf("inspector Mots tab should omit the Notes section entirely when no file is open (doc.words == 0):\n%s", out)
	}
}
```

Ces trois tests ne compileront pas encore (mauvais nombre d'arguments), ce qui est attendu à cette
étape — voir Step 2.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go build ./... 2>&1 | head -30`
Expected: compile error — `too many arguments in call to in.View` (les 3 nouveaux tests appellent déjà
la signature à 7 arguments qui n'existe pas encore).

- [ ] **Step 3: Update the `View()` signature and wire in the Notes section**

Dans `inspector.go`, modifie la signature de `View` (ligne 546) :

```go
func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string, goals goalStats, analysis analysisState, notes []note) string {
```

Puis, dans le bloc `default: // tabWords` (qui se termine actuellement par le bloc `if
len(doc.overused) > 0 { ... }`, ~ligne 650), ajoute la section Notes juste avant le `}` qui ferme le
`case` — donc après le bloc `overused`, toujours à l'intérieur du `if doc.words > 0 {` existant :

```go
	default: // tabWords
		b.WriteString(sectionHeader("Document", width) + "\n")
		b.WriteString("  " + kvRow("Mots", doc.words, width-2) + "\n")
		b.WriteString("  " + kvRow("Caractères", doc.chars, width-2) + "\n")
		b.WriteString("  " + kvRow("Paragraphes", doc.paragraphs, width-2) + "\n\n")
		b.WriteString(sectionHeader("Projet", width) + "\n")
		b.WriteString("  " + kvRow("Mots", proj.words, width-2))
		if proj.manuscript {
			b.WriteString("\n  " + kvRow("Chapitres", proj.chapters, width-2))
		}
		if doc.words > 0 {
			b.WriteString("\n\n" + sectionHeader("Lisibilité", width) + "\n")
			b.WriteString("  " + kvStrRow("Temps de lecture", fmtReadTime(doc.readSecs), width-2) + "\n")
			b.WriteString("  " + kvStrRow("Phrase moy.", fmt.Sprintf("%.0f±%.0f mots", doc.sentMean, doc.sentStdDev), width-2))
			if doc.readabilityLabel != "" {
				score := fmt.Sprintf("%.0f · %s", doc.readabilityScore, doc.readabilityLabel)
				b.WriteString("\n  " + kvStrRow("Score", score, width-2))
			}
			if len(doc.overused) > 0 {
				b.WriteString("\n\n" + sectionHeader("Surutilisés", width) + "\n")
				for i, wf := range doc.overused {
					b.WriteString("  " + kvRow(wf.word, wf.n, width-2))
					if i < len(doc.overused)-1 {
						b.WriteString("\n")
					}
				}
			}
			b.WriteString("\n\n" + renderNotesSection(notes, width))
		}
	}
```

(Seul le changement : l'ajout de la ligne `b.WriteString("\n\n" + renderNotesSection(notes, width))`
juste avant le `}` qui ferme le `if doc.words > 0 {` — tout le reste du bloc est identique à
l'existant, reproduit ici pour éviter toute ambiguïté sur l'emplacement exact.)

- [ ] **Step 4: Update the 12 existing `.View(...)` call sites in `inspector_test.go`**

Chacun des appels suivants (repérés avant modification) doit recevoir un 7ᵉ argument `nil` (aucune
note — ces tests ne portent pas sur les notes) :

```
line ~70:  in.View(28, docStats{words: 1204, chars: 6830, paragraphs: 38}, projStats{words: 47032, chapters: 12, manuscript: true}, "", goalStats{}, analysisState{})
line ~77:  in.View(28, docStats{words: 10}, projStats{words: 10, manuscript: false}, "", goalStats{}, analysisState{})
line ~112: in.View(28, docStats{}, projStats{}, "- Top\n  - sub", goalStats{}, analysisState{})
line ~118: in.View(28, docStats{}, projStats{}, "", goalStats{}, analysisState{})
line ~142: in.View(28, docStats{}, projStats{}, "", goalStats{today: 312, dailyGoal: 500, project: 47032, projectGoal: 80000}, analysisState{})
line ~149: in.View(28, docStats{}, projStats{}, "", goalStats{today: 10, dailyGoal: 500, project: 10, projectGoal: 0}, analysisState{})
line ~154: in.View(28, docStats{}, projStats{}, "", goalStats{today: 600, dailyGoal: 500, project: 1, projectGoal: 0}, analysisState{})
line ~182: in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: false})
line ~189: in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{})
line ~210: in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{spell: true, adverb: true})
line ~260: in.View(inspectorInnerWidth(), docStats{}, projStats{}, "", goalStats{}, analysisState{grammar: true})
line ~344: in.View(28, docStats{}, projStats{}, "", goalStats{sessionSecs: 300, todayActiveSecs: 600, sessionGoalMin: 30, idle: true}, analysisState{})
```

Pour chacun, ajoute `, nil` juste avant la parenthèse fermante finale. Exemple pour la ligne 70 :

```go
out := in.View(28, docStats{words: 1204, chars: 6830, paragraphs: 38}, projStats{words: 47032, chapters: 12, manuscript: true}, "", goalStats{}, analysisState{}, nil)
```

Répète ce même ajout (`, nil` avant le `)` final) pour les 11 autres lignes listées ci-dessus. Utilise
la recherche/remplacement de ton éditeur plutôt que de retaper chaque ligne — le seul changement est
l'insertion de `, nil` avant le dernier `)` de chaque appel `.View(`.

- [ ] **Step 5: Update the `main.go` call site**

Dans `main.go`, remplace la ligne 1744 :

```go
		insInner := m.inspector.View(inspectorInnerWidth(), doc, proj, readOutlineDoc(m.files.dir), gs, m.analysis)
```

par (ajoute le chargement des notes juste avant, et le nouvel argument) :

```go
		notes := loadNotes(m.currentFile)
		insInner := m.inspector.View(inspectorInnerWidth(), doc, proj, readOutlineDoc(m.files.dir), gs, m.analysis, notes)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./... -run TestInspector -v 2>&1 | tail -80`
Expected: all inspector tests PASS, including the 3 new ones from Step 1.

- [ ] **Step 7: Build, vet, full test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean build, no vet warnings, all tests PASS (this also catches any other call site to
`inspectorModel.View` outside `inspector_test.go`/`main.go` if one was missed).

- [ ] **Step 8: Commit**

```bash
git add inspector.go inspector_test.go main.go
git commit -m "$(cat <<'EOF'
Affiche les notes de révision du fichier ouvert dans l'onglet Mots de l'inspecteur

inspectorModel.View() prend désormais []note en dernier paramètre ; main.go
charge les notes du fichier courant via loadNotes() (déjà existant,
tolérant aux erreurs) juste avant l'appel. Section NOTES rendue en lecture
seule, sous Document/Projet/Lisibilité/Surutilisés, uniquement quand un
fichier est ouvert — placeholder si aucune note, sinon une ligne par note
(première ligne + « … » si multi-lignes, tronquée à la largeur du panneau).
EOF
)"
```

---

## Task 3: Vérification manuelle

**Files:** none — vérification uniquement, aucun changement de code.

**Interfaces:**
- Consumes: la fonctionnalité complète des Tasks 1–2.
- Produces: rien de nouveau — confirme que tout fonctionne ensemble visuellement.

- [ ] **Step 1: Full build, vet, test suite**

Run: `go build ./... && go vet ./... && go test ./... -count=1 2>&1 | tail -30`
Expected: clean build, no vet warnings, all tests PASS.

- [ ] **Step 2: Manual walkthrough in tmux**

```bash
go build -o /tmp/forkashi-notes-check .
rm -rf /tmp/notes-check-home /tmp/notes-check-proj
mkdir -p /tmp/notes-check-home /tmp/notes-check-proj/Livre
cat > /tmp/notes-check-proj/Livre/manifest.json <<'EOF'
{"schemaVersion":1,"title":"Mon Livre","items":[{"file":"01-un.md","title":"Un"}]}
EOF
echo "Premier chapitre de test." > /tmp/notes-check-proj/Livre/01-un.md
tmux new-session -d -s notescheck -x 120 -y 40 \
  "cd /tmp/notes-check-proj/Livre && HOME=/tmp/notes-check-home XDG_CONFIG_HOME=/tmp/notes-check-home/.config OKASHI_DIR=/tmp/notes-check-proj/Livre /tmp/forkashi-notes-check"
sleep 1
tmux capture-pane -t notescheck -p
```

Walk through, in order:

1. Ouvre `01-un.md` (`⏎` depuis la sidebar), `ctrl+y` pour afficher l'inspecteur — confirme l'onglet
   Mots affiche déjà Document/Projet, et en bas une section NOTES avec le placeholder
   `(aucune note — n pour en ajouter)`.
2. `esc` puis `ctrl+y` pour refermer l'inspecteur (le cycle passe par Plan/Objectifs/Analyse — répète
   `ctrl+y` jusqu'à refermeture, ou navigue directement au fichier).
3. Depuis la sidebar (fichier `01-un.md` sélectionné), `n` pour ouvrir l'écran Notes, `a` pour ajouter
   une note, tape `Vérifier la continuité du prénom du personnage`, `esc` pour valider, `esc` pour
   revenir à la sidebar.
4. `ctrl+y` (potentiellement plusieurs fois pour arriver sur l'onglet Mots) — confirme que la section
   NOTES affiche maintenant `Vérifier la continuité du prénom du personnage` au lieu du placeholder.
5. Retourne à l'écran Notes (`n`), `a` pour ajouter une seconde note multi-lignes : tape
   `Rythme à retravailler`, `⏎` (insère un saut de ligne dans le textarea), tape `voir chapitre 3`,
   `esc` pour valider.
6. `ctrl+y` vers l'onglet Mots — confirme que la section NOTES affiche maintenant DEUX lignes : la
   première note complète, puis `Rythme à retravailler …` (sans `voir chapitre 3` visible).

Documente toute anomalie visuelle (désalignement, débordement) trouvée pendant ce parcours — note
fichier:ligne si la cause est identifiable, sans nécessairement la corriger dans cette tâche si c'est
mineur ; signale-la comme un finding.

- [ ] **Step 3: Clean up**

```bash
tmux send-keys -t notescheck 'C-c'
tmux kill-session -t notescheck 2>/dev/null
rm -f /tmp/forkashi-notes-check
rm -rf /tmp/notes-check-home /tmp/notes-check-proj
```

- [ ] **Step 4: Report findings**

No commit for this task (verification only). Si l'étape 2 a fait apparaître une anomalie visuelle, la
signaler — ne pas la corriger silencieusement sous cette tâche sauf correction triviale et
manifestement sûre ; sinon la traiter comme un suivi.

---

## Self-Review

**Spec coverage:**
- Emplacement/déclenchement (section NOTES en dernier dans l'onglet Mots, garde `doc.words > 0`) →
  Task 2 Step 3.
- Comportement d'affichage (placeholder si vide, première ligne + `…` + troncature si notes présentes,
  pas de plafond) → Task 1.
- Câblage technique (`loadNotes` inchangé, nouveau paramètre `View()`, nouveau call site `main.go`,
  `renderNotesSection` suivant le pattern `renderOutline`) → Task 1 + Task 2.
- Hors périmètre (pas d'interaction, pas de plafond, pas de changement au scope des notes, pas de
  raccourci) — rien dans le plan n'y contrevient.
- Tests (unitaires `renderNotesSection`, tests `View()` avec/sans notes/sans fichier, vérification
  manuelle façon smoke test) → Task 1 Step 1, Task 2 Step 1, Task 3.

**Placeholder scan:** aucun "TBD"/"TODO" ; chaque step de code contient le code réel à écrire, pas une
description vague.

**Type consistency:** `renderNotesSection(notes []note, width int) string` (Task 1) est bien le type
utilisé dans la nouvelle signature de `View()` (Task 2) ; `note` provient de `notes.go` sans changement
de forme.
