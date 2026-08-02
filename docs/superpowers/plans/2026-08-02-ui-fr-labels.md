# Interface en français — labels, menus, aide Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Traduire en français toutes les chaînes littérales anglaises visibles à l'utilisateur (labels, menus, aide, messages de statut) dans les 12 fichiers du cœur de l'interface de forkashi, sans modifier aucun raccourci clavier ni casser le build.

**Architecture:** Un fichier = une tâche. Chaque tâche remplace les chaînes littérales anglaises identifiées par leur équivalent français, directement dans le code (pas de système i18n). Les combinaisons de touches (`case "y":`, `case "n":`, etc.) ne sont **jamais** modifiées — seul le texte affiché à côté change.

**Tech Stack:** Go 1.25, Bubble Tea/lipgloss (TUI) — aucune nouvelle dépendance.

## Global Constraints

- Le module Go reste `okashi` dans `go.mod` — ne pas le modifier.
- **Aucune combinaison de touche (`case "x":`, `key.String() == "y"`, etc.) n'est modifiée dans ce plan.** Seul le texte qui accompagne un raccourci change ; la lettre affichée reste la même lettre que celle testée dans le code, même quand elle ne correspond plus à l'initiale du mot français (ex. `y confirmer`, `d supprimer`, `x supprimer`).
- Le mot « diff » reste tel quel (non traduit) partout où il apparaît — décision produit actée lors du brainstorming, préserve la cohérence visuelle des raccourcis `d`/`D` dans `snapshots.go`.
- « Session », « Document », « Contact », « Score », « notes », « synopsis », « Tufte » restent inchangés (mots identiques ou noms propres en français).
- Chaque tâche doit se terminer avec `go build ./...` et `go vet ./...` propres, et `go test ./...` vert. Utiliser Go 1.25+ (`export PATH="$HOME/.local/go/bin:$PATH"` — le `go` système par défaut est 1.19, trop ancien).
- Après traduction, vérifier visuellement (lancement manuel du binaire, cf. Tâche 12) que l'alignement des colonnes/tableaux n'est pas cassé par des libellés français plus longs que l'anglais — ajuster les espaces de padding en dur si nécessaire, sans changer la largeur totale calculée dynamiquement (`width`, `lipgloss.Width(...)`) sauf si explicitement indiqué dans une tâche.
- Commits fréquents, un commit par tâche minimum.

---

## File Structure

| Fichier | Tâche | Nb. chaînes | Priorité |
|---|---|---|---|
| `main.go` | 1 | ~75 | Très haute (bloc d'aide F1, tous messages de statut de l'app) |
| `home.go` | 2 | ~24 | Haute (écran d'accueil, premier contact) |
| `inspector.go` | 3 | ~40 | Haute (onglets et sections toujours visibles) |
| `corkboard.go` | 4 | ~25 | Haute (vue manuscrit principale) |
| `structure.go` | 5 | 2 | Moyenne (dépendance de corkboard.go, découverte lors de l'exploration) |
| `outline.go` | 6 | 12 | Moyenne |
| `mover.go` | 7 | ~22 | Moyenne |
| `search.go` | 8 | 13 | Moyenne |
| `snapshots.go` | 9 | ~20 | Moyenne |
| `properties.go` | 10 | ~15 | Moyenne |
| `notes.go` | 11 | ~10 | Basse |
| `goals.go` | (inclus tâche 3 ou dédiée) | 4 | Basse |

`goals.go` n'a que 4 chaînes, toutes dans une seule fonction (`paceLine`) — traité comme Tâche 12 séparée pour rester dans l'esprit "un fichier = une tâche testable isolément", malgré sa petite taille.

Tâche 13 : vérification manuelle finale (lancement du binaire, parcours des écrans principaux).

---

## Task 1: Traduction de main.go

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: rien (première tâche du plan)
- Produces: aucune nouvelle fonction/type — remplacement de chaînes littérales en place, les signatures existantes ne changent pas.

- [ ] **Step 1: Vérifier l'état actuel du fichier autour de chaque zone avant modification**

```bash
export PATH="$HOME/.local/go/bin:$PATH"
grep -n "NAVIGATE\|FILES  (sidebar\|MANUSCRIPT  (cork\|OUTLINE  (ctrl\|^WRITE\|REVIEW &\|GOALS &\|^APP" main.go
```
Expected: confirme les numéros de ligne du bloc `helpText` (doivent être proches de 45-72, à ajuster si le fichier a légèrement bougé depuis l'exploration).

- [ ] **Step 2: Traduire le bloc d'aide `helpText`**

Remplacer le contenu du template littéral `helpText` (zone `NAVIGATE` à `APP`, ~lignes 45-72) :

```go
const helpText = `NAVIGATE
  ctrl+o home   ctrl+b sidebar   ctrl+y inspector
  ctrl+k corkboard   ctrl+l outline   esc back/focus

FILES  (sidebar focus)
  ctrl+n new · chapter|resource   r rename (F2)
  d duplicate   M move   del delete

MANUSCRIPT  (corkboard: ctrl+k or c)
  J/K reorder (shift+↑↓)   e synopsis   a add   x remove   r retitle
  m pager   b snapshots/diff   n notes

OUTLINE  (ctrl+l)
  shift+↑/↓ move beat   ctrl+p promote → chapter   (alt works too)

WRITE
  ctrl+s save   ctrl+z undo   ⇥/⇧⇥ indent
  ctrl+t typewriter   ctrl+d focus-dim   ctrl+x select
  ctrl+r spelling   ⌥/⇧+drag select · ⌘C copy

REVIEW & OUTPUT
  ctrl+f search   ctrl+p preview   ctrl+e export

GOALS & TIME
  ctrl+g goals   ctrl+u sprint   g history (sidebar)

APP
  i properties (home)   F1/? help   ctrl+c quit`
```

par :

```go
const helpText = `NAVIGUER
  ctrl+o accueil   ctrl+b panneau   ctrl+y inspecteur
  ctrl+k tableau   ctrl+l plan   esc retour/focus

FICHIERS  (focus panneau)
  ctrl+n nouveau · chapitre|ressource   r renommer (F2)
  d dupliquer   M déplacer   suppr supprimer

MANUSCRIT  (tableau : ctrl+k ou c)
  J/K réordonner (maj+↑↓)   e synopsis   a ajouter   x retirer   r retitrer
  m lecture   b sauvegardes/diff   n notes

PLAN  (ctrl+l)
  maj+↑/↓ déplacer le beat   ctrl+p promouvoir → chapitre   (alt marche aussi)

ÉCRIRE
  ctrl+s enregistrer   ctrl+z annuler   ⇥/⇧⇥ indenter
  ctrl+t machine à écrire   ctrl+d focus atténué   ctrl+x sélection
  ctrl+r orthographe   ⌥/⇧+glisser sélectionner · ⌘C copier

RELECTURE & SORTIE
  ctrl+f rechercher   ctrl+p aperçu   ctrl+e exporter

OBJECTIFS & TEMPS
  ctrl+g objectifs   ctrl+u sprint   g historique (panneau)

APPLICATION
  i propriétés (accueil)   F1/? aide   ctrl+c quitter`
```

Note : « corkboard » est traduit en « tableau » (plutôt que « tableau de liège », trop long pour tenir dans la largeur de colonne du bloc d'aide) — ce choix de terme court est repris identiquement dans toutes les tâches suivantes qui touchent ce mot (corkboard.go notamment).

- [ ] **Step 3: Vérifier que le build compile après le Step 2**

Run: `go build ./... 2>&1`
Expected: aucune erreur (un template littéral Go multi-lignes ne peut pas casser la compilation par un changement de texte, mais vérifie qu'aucun backtick n'a été introduit par erreur dans le texte français).

- [ ] **Step 4: Traduire les placeholders et labels des champs de saisie (lignes ~358-378)**

Remplacer :
```go
ta.Placeholder = "Start writing…"
```
par :
```go
ta.Placeholder = "Commencez à écrire…"
```

- [ ] **Step 5: Traduire le titre et pied de l'overlay d'aide (lignes ~1615, 1617)**

Remplacer :
```go
card := framedPanel("Keys", helpText, 58, min(hH, lipgloss.Height(helpText)+2), "")
```
par :
```go
card := framedPanel("Touches", helpText, 58, min(hH, lipgloss.Height(helpText)+2), "")
```

Remplacer :
```go
statusStyle.Width(m.width).Render("F1 · ? · esc  close")
```
par :
```go
statusStyle.Width(m.width).Render("F1 · ? · esc  fermer")
```

- [ ] **Step 6: Traduire les messages de statut — grammaire/orthographe**

Remplacer chacune de ces lignes (chercher le texte anglais exact avec `grep -n` avant de remplacer, les numéros de ligne peuvent avoir légèrement bougé après les Steps précédents) :

| Avant | Après |
|---|---|
| `m.status = "added '" + m.suggestWord + "' to dictionary"` | `m.status = "« " + m.suggestWord + " » ajouté au dictionnaire"` |
| `m.status = "'" + m.suggestWord + "' is already known"` | `m.status = "« " + m.suggestWord + " » est déjà connu"` |
| `m.status = "'" + m.suggestWord + "' → '" + chosen + "'"` | `m.status = "« " + m.suggestWord + " » → « " + chosen + " »"` |
| `m.status = "1 grammar note"` | `m.status = "1 remarque grammaticale"` |
| `m.status = fmt.Sprintf("%d grammar notes", n)` | `m.status = fmt.Sprintf("%d remarques grammaticales", n)` |
| `m.status = "grammar check failed"` | `m.status = "échec de la vérification grammaticale"` |
| `m.status = "suggestion cancelled"` | `m.status = "suggestion annulée"` |
| `m.status = "checking grammar…"` | `m.status = "vérification grammaticale…"` |
| `m.status = "no word under cursor"` | `m.status = "aucun mot sous le curseur"` |
| `m.status = "'" + w + "' looks correct"` | `m.status = "« " + w + " » semble correct"` |

- [ ] **Step 7: Traduire les messages de statut — création/renommage/suppression de fichiers**

| Avant | Après |
|---|---|
| `m.status = "create cancelled"` (les 2 occurrences) | `m.status = "création annulée"` |
| `m.status = "rename cancelled"` | `m.status = "renommage annulé"` |
| `m.status = "delete cancelled"` | `m.status = "suppression annulée"` |
| `m.status = "export cancelled"` | `m.status = "export annulé"` |
| `m.status = "deadline must be YYYY-MM-DD (or blank) — try again"` | `m.status = "échéance au format AAAA-MM-JJ (ou vide) — réessayez"` |
| `m.status = "goals saved"` | `m.status = "objectifs enregistrés"` |
| `m.status = "new: c chapter (ordered) · r resource (loose doc) · esc cancel"` | `m.status = "nouveau : c chapitre (ordonné) · r ressource (doc libre) · esc annuler"` |
| `m.status = "-- SELECT -- · drag to select, copy with your terminal · ctrl+x exits"` | `m.status = "-- SÉLECTION -- · glissez pour sélectionner, copiez avec votre terminal · ctrl+x quitte"` |
| `m.status = "select mode off"` | `m.status = "mode sélection désactivé"` |
| `m.status = "typewriter on"` | `m.status = "machine à écrire activée"` |
| `m.status = "typewriter off"` | `m.status = "machine à écrire désactivée"` |
| `m.status = "export: m manuscript · t tufte · esc cancel"` | `m.status = "export : m manuscrit · t tufte · esc annuler"` |
| `m.status = "dim on"` | `m.status = "atténuation activée"` |
| `m.status = "dim off"` | `m.status = "atténuation désactivée"` |
| `m.status = "sprint stopped"` | `m.status = "sprint arrêté"` |
| `m.status = fmt.Sprintf("sprint started — %d min", mins)` | `m.status = fmt.Sprintf("sprint démarré — %d min", mins)` |

- [ ] **Step 8: Traduire les messages de statut — ouverture/chargement de fichier**

| Avant | Après |
|---|---|
| `m.status = "save failed — staying on " + filepath.Base(m.currentFile)` | `m.status = "échec de l'enregistrement — reste sur " + filepath.Base(m.currentFile)` |
| `m.status = "couldn't open: " + filepath.Base(path)` | `m.status = "impossible d'ouvrir : " + filepath.Base(path)` |
| `m.status = "opened " + filepath.Base(path)` | `m.status = "ouvert " + filepath.Base(path)` |
| `m.status = "⚠ " + filepath.Base(path) + " isn't valid UTF-8 — editing then saving will re-encode it"` | `m.status = "⚠ " + filepath.Base(path) + " n'est pas un UTF-8 valide — modifier puis enregistrer le ré-encodera"` |

- [ ] **Step 9: Traduire les messages de statut — création chapitre/ressource/projet**

| Avant | Après |
|---|---|
| `m.status = "a chapter name can't contain a path separator"` | `m.status = "un nom de chapitre ne peut pas contenir de séparateur de chemin"` |
| `m.status = "a file named " + name + " already exists"` (les 2 occurrences) | `m.status = "un fichier nommé " + name + " existe déjà"` |
| `m.status = "couldn't create chapter: " + err.Error()` | `m.status = "impossible de créer le chapitre : " + err.Error()` |
| `m.status = "chapter created but manifest update failed: " + werr.Error()` | `m.status = "chapitre créé mais échec de la mise à jour du manifeste : " + werr.Error()` |
| `m.status = "new chapter " + name` | `m.status = "nouveau chapitre " + name` |
| `m.status = "a resource name can't be empty or contain '/' (use Folder/name to file it in a subfolder)"` | `m.status = "un nom de ressource ne peut pas être vide ni contenir '/' (utilisez Dossier/nom pour le classer dans un sous-dossier)"` |
| `m.status = "couldn't create folder: " + err.Error()` (les 2 occurrences) | `m.status = "impossible de créer le dossier : " + err.Error()` |
| `m.status = "couldn't create resource: " + err.Error()` | `m.status = "impossible de créer la ressource : " + err.Error()` |
| `m.status = "new resource " + name` | `m.status = "nouvelle ressource " + name` |
| `m.status = "create cancelled (no name)"` | `m.status = "création annulée (aucun nom)"` |
| `m.status = "name can't contain a path separator"` (les 2 occurrences, une aussi dans confirmRename) | `m.status = "le nom ne peut pas contenir de séparateur de chemin"` |
| `m.status = "manifest.json can't be renamed or removed — it's how okashi tracks chapter order"` (les 3 occurrences identiques) | `m.status = "manifest.json ne peut être ni renommé ni supprimé — c'est ainsi qu'okashi suit l'ordre des chapitres"` |
| `m.status = "couldn't create project: " + err.Error()` | `m.status = "impossible de créer le projet : " + err.Error()` |
| `m.status = "new project " + name + " — start writing"` | `m.status = "nouveau projet " + name + " — commencez à écrire"` |
| `m.status = "created folder " + name` | `m.status = "dossier créé " + name` |
| `m.status = "new file: " + name + " — ctrl+s to save"` | `m.status = "nouveau fichier : " + name + " — ctrl+s pour enregistrer"` |

- [ ] **Step 10: Traduire les messages de statut — renommage/suppression/duplication**

| Avant | Après |
|---|---|
| `m.status = "manifest unreadable — structure is read-only (external manifest)"` | `m.status = "manifeste illisible — structure en lecture seule (manifeste externe)"` |
| `m.status = "chapter files are read-only (external manifest)"` | `m.status = "les fichiers de chapitre sont en lecture seule (manifeste externe)"` |
| `m.status = "delete '" + e.name + "'? [y]es · esc cancel"` | `m.status = "supprimer « " + e.name + " » ? [y]es · esc annuler"` (garder `[y]es` tel quel — cf. Global Constraints, ne pas franciser en `[o]ui`) |
| `m.status = "couldn't delete: " + err.Error()` | `m.status = "impossible de supprimer : " + err.Error()` |
| `m.status = "deleted"` | `m.status = "supprimé"` |
| `m.status = "duplicate: files only"` | `m.status = "duplication : fichiers uniquement"` |
| `m.status = "duplicate failed: " + err.Error()` (les 2 occurrences) | `m.status = "échec de la duplication : " + err.Error()` |
| `m.status = "duplicated → " + target` | `m.status = "dupliqué → " + target` |
| `m.status = "rename cancelled (empty)"` | `m.status = "renommage annulé (vide)"` |
| `m.status = "retitle failed: " + err.Error()` | `m.status = "échec du retitrage : " + err.Error()` |
| `m.status = "retitled to " + typed` | `m.status = "retitré en " + typed` |
| `m.status = "unchanged"` | `m.status = "inchangé"` |
| `m.status = "rename failed: " + err.Error()` | `m.status = "échec du renommage : " + err.Error()` |
| `m.status = "renamed to " + newName` | `m.status = "renommé en " + newName` |

- [ ] **Step 11: Traduire l'aperçu Markdown (preview) et le pager**

| Avant | Après |
|---|---|
| `name = "untitled"` | `name = "sans titre"` |
| `style := "Default"` | `style := "Standard"` |
| `"▌ PREVIEW · "` (dans le header du preview) | `"▌ APERÇU · "` |
| `m.status = "editing"` | `m.status = "édition"` |
| `m.status = "preview (read-only) · ctrl+p edit · t style · ↑/↓ scroll"` | `m.status = "aperçu (lecture seule) · ctrl+p éditer · t style · ↑/↓ défiler"` |
| `m.status = "preview unavailable: " + err.Error()` | `m.status = "aperçu indisponible : " + err.Error()` |
| `m.status = "preview failed: " + err.Error()` | `m.status = "échec de l'aperçu : " + err.Error()` |
| `m.status = "manuscript · ↑↓ scroll · enter edit here · esc editor"` | `m.status = "manuscrit · ↑↓ défiler · entrée éditer ici · esc éditeur"` |
| `return "loading…"` | `return "chargement…"` |

- [ ] **Step 12: Traduire les messages de statut — sauvegarde**

| Avant | Après |
|---|---|
| `m.status = "no file open — pick one from the sidebar first"` | `m.status = "aucun fichier ouvert — choisissez-en un dans le panneau d'abord"` |
| `m.status = "save failed (conflict): " + werr.Error()` | `m.status = "échec de l'enregistrement (conflit) : " + werr.Error()` |
| `m.status = "⚠ " + filepath.Base(m.currentFile) + " changed on disk — your edits saved to " + filepath.Base(confl)` | `m.status = "⚠ " + filepath.Base(m.currentFile) + " a changé sur le disque — vos modifications ont été enregistrées dans " + filepath.Base(confl)` |
| `m.status = "save failed: " + err.Error()` | `m.status = "échec de l'enregistrement : " + err.Error()` |
| `m.status = "saved " + filepath.Base(m.currentFile)` | `m.status = "enregistré " + filepath.Base(m.currentFile)` |

- [ ] **Step 13: Traduire la barre de statut composée**

| Avant | Après |
|---|---|
| `label := "new file ▸ "` | `label := "nouveau fichier ▸ "` |
| `label = "new folder ▸ "` | `label = "nouveau dossier ▸ "` |
| `"end with / for a folder"` | `"terminez par / pour un dossier"` |
| `return "suggest ▸ " + strings.Join(parts, " · ")` | `return "suggestion ▸ " + strings.Join(parts, " · ")` |
| `return "rename ▸ " + m.nameInput.View()` | `return "renommer ▸ " + m.nameInput.View()` |
| `return "daily goal ▸ " + m.nameInput.View()` | `return "objectif quotidien ▸ " + m.nameInput.View()` |
| `return "project goal ▸ " + m.nameInput.View()` | `return "objectif du projet ▸ " + m.nameInput.View()` |
| `return "daily minutes ▸ " + m.nameInput.View()` | `return "minutes quotidiennes ▸ " + m.nameInput.View()` |
| `return "deadline YYYY-MM-DD (blank clears) ▸ " + m.nameInput.View()` | `return "échéance AAAA-MM-JJ (vide efface) ▸ " + m.nameInput.View()` |
| `return "export: m manuscript · t tufte · esc cancel"` | `return "export : m manuscrit · t tufte · esc annuler"` |
| `return "new: c chapter · r resource · esc cancel"` | `return "nouveau : c chapitre · r ressource · esc annuler"` |
| `status = "c corkboard · m read · b backups · F1 keys"` | `status = "c tableau · m lecture · b sauvegardes · F1 touches"` |

- [ ] **Step 14: Vérifier le build, vet, et lancer la suite de tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: build et vet propres, tous les tests PASS (les tests unitaires ne devraient pas dépendre du texte exact de ces messages de statut — si un test échoue en comparant une chaîne littérale anglaise, lire ce test et l'adapter au nouveau texte français, en documentant le changement dans le rapport).

- [ ] **Step 15: Commit**

```bash
git add main.go
git commit -m "$(cat <<'EOF'
Traduit main.go en français (aide, messages de statut)

Bloc d'aide F1 complet, tous les messages de statut de l'app
(sauvegarde, création/suppression/renommage, grammaire/orthographe,
preview, export). Aucun raccourci clavier modifié — seul le texte
descriptif change.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Traduction de home.go

**Files:**
- Modify: `home.go`

**Interfaces:**
- Consumes: rien de nouveau créé par Task 1.
- Produces: aucune nouvelle fonction/type.

- [ ] **Step 1: Traduire les messages de statut**

| Avant | Après |
|---|---|
| `m.status = "add source cancelled"` | `m.status = "ajout de source annulé"` |
| `m.status = "removal cancelled"` | `m.status = "suppression annulée"` |
| `m.status = "remove source \"" + m.sources[m.activeSource].Name + "\"? y = remove · re-add with a"` | `m.status = "supprimer la source \"" + m.sources[m.activeSource].Name + "\"? y = supprimer · ré-ajouter avec a"` |
| `m.status = "the primary source can't be removed"` (les 2 occurrences) | `m.status = "la source principale ne peut pas être supprimée"` |
| `m.status = "properties"` | `m.status = "propriétés"` |
| `m.status = "source: " + m.sources[nxt].Name` | `m.status = "source : " + m.sources[nxt].Name` |
| `m.status = "not a folder: " + path` | `m.status = "pas un dossier : " + path` |
| `m.status = "source added: " + s.Name` | `m.status = "source ajoutée : " + s.Name` |
| `m.status = "source removed: " + s.Name` | `m.status = "source supprimée : " + s.Name` |

Note sur `"y = remove · re-add with a"` : les lettres `y` et `a` restent inchangées (`y` teste `case "y", "enter":`, `a` teste `case "a":` — ni l'une ni l'autre ne dépend de l'initiale du mot anglais affiché, aucune modification de code nécessaire, cf. Global Constraints).

- [ ] **Step 2: Traduire le placeholder et le prompt d'ajout de source**

| Avant | Après |
|---|---|
| `m.nameInput.Placeholder = "/path/to/folder"` | `m.nameInput.Placeholder = "/chemin/vers/dossier"` |
| `prompt := "add source ▸ " + m.nameInput.View()` | `prompt := "ajouter une source ▸ " + m.nameInput.View()` |

- [ ] **Step 3: Traduire les titres de sections — colonne LIBRARY**

| Avant | Après |
|---|---|
| `rows = append(rows, lrow{header: true, text: "PROJECTS"})` | `rows = append(rows, lrow{header: true, text: "PROJETS"})` |
| `rows = append(rows, lrow{header: true, text: "FOLDERS"})` | `rows = append(rows, lrow{header: true, text: "DOSSIERS"})` |
| `rows = append(rows, lrow{header: true, text: "OTHER"})` | `rows = append(rows, lrow{header: true, text: "AUTRE"})` |
| `libTitle := "LIBRARY"` | `libTitle := "BIBLIOTHÈQUE"` |
| `libTitle = "LIBRARY · " + m.sources[m.activeSource].Name + " ▾"` | `libTitle = "BIBLIOTHÈQUE · " + m.sources[m.activeSource].Name + " ▾"` |
| `[]string{libTitle, "FILES"}` (les 2 occurrences) | `[]string{libTitle, "FICHIERS"}` |
| `pinnedBox := framedPanel("PINNED", ...)` | `pinnedBox := framedPanel("ÉPINGLÉS", ...)` |
| `strip := framedPanel("RECENT", ...)` | `strip := framedPanel("RÉCENTS", ...)` |

- [ ] **Step 4: Traduire les messages "vide"**

| Avant | Après |
|---|---|
| `return []string{homeDim("no projects — + to create")}, nil` | `return []string{homeDim("aucun projet — + pour créer")}, nil` |
| `return []string{homeDim("no files — ctrl+n for a doc")}, nil` | `return []string{homeDim("aucun fichier — ctrl+n pour un doc")}, nil` |
| `return []string{homeDim("(no recent files)")}, nil` | `return []string{homeDim("(aucun fichier récent)")}, nil` |

- [ ] **Step 5: Traduire les labels d'actions statiques**

| Avant | Après |
|---|---|
| `homeItem{kind: homeMoveFiles, label: "Move files"},` | `homeItem{kind: homeMoveFiles, label: "Déplacer des fichiers"},` |
| `homeItem{kind: homeOpenOther, label: "Browse all files"},` | `homeItem{kind: homeOpenOther, label: "Parcourir tous les fichiers"},` |

Ne PAS modifier `label: "◦ Notes"` (ligne ~185) — "Notes" est identique en français.

- [ ] **Step 6: Traduire le rappel d'aide et le texte d'accueil premier lancement**

| Avant | Après |
|---|---|
| `hint := statusStyle...Render("F1 · ?  keybindings")` | `hint := statusStyle...Render("F1 · ?  raccourcis")` |
| `tagline := ...Render("write a whole book in plain Markdown — chapters, outline, export")` | `tagline := ...Render("écrivez un livre entier en Markdown — chapitres, plan, export")` |
| `primer := ...Render("manuscript  ordered chapters (a book)   ·   category  a plain folder of notes\n" + "+  new manuscript or folder   ·   ctrl+n  new doc   ·   open Demo/ to explore")` | `primer := ...Render("manuscrit  chapitres ordonnés (un livre)   ·   catégorie  un simple dossier de notes\n" + "+  nouveau manuscrit ou dossier   ·   ctrl+n  nouveau doc   ·   ouvrez Demo/ pour explorer")` |

Ne PAS traduire `"Demo/"` — c'est un nom de dossier réel généré par le programme, pas du texte UI.

- [ ] **Step 7: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 8: Commit**

```bash
git add home.go
git commit -m "$(cat <<'EOF'
Traduit home.go en français (écran d'accueil)

Sections PROJECTS/FILES/FOLDERS/RECENT/PINNED/LIBRARY/OTHER, messages
de statut de gestion des sources, texte d'accueil premier lancement.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Traduction de inspector.go et goals.go

**Files:**
- Modify: `inspector.go`, `goals.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

**Note** : `goals.go` est inclus dans cette tâche plutôt qu'une tâche séparée — il n'a que 4 chaînes, toutes liées au calcul de rythme d'écriture (`paceLine`) qui est affiché dans l'onglet Goals de l'inspecteur, donc thématiquement proche. Le fichier reste testable indépendamment (Step 5 le teste séparément).

- [ ] **Step 1: Traduire les onglets de l'inspecteur**

Remplacer :
```go
[]string{"Words", "Outline", "Goals", "Analysis"}
```
par :
```go
[]string{"Mots", "Plan", "Objectifs", "Analyse"}
```

(fonction `inspectorTabLabels()`, ligne ~91)

- [ ] **Step 2: Traduire l'onglet Analysis et Syntax**

| Avant | Après |
|---|---|
| `sectionHeader("Analysis", width)` | `sectionHeader("Analyse", width)` |
| `"Spellcheck"` | `"Orthographe"` |
| `grammarStyle.Render(...)` avec texte `"Grammar"` | `"Grammaire"` |
| `sectionHeader("Syntax", width)` | `sectionHeader("Syntaxe", width)` |
| `adverbStyle.Render(...)` avec texte `"Adverb"` | `"Adverbe"` |
| `adjStyle.Render(...)` avec texte `"Adjective"` | `"Adjectif"` |
| `passiveStyle.Render(...)` avec texte `"Passive/weak"` | `"Passif/faible"` |
| `"▸ Check grammar"` | `"▸ Vérifier la grammaire"` |
| `"checking grammar…"` | `"vérification…"` |
| `"Auto-recheck"` | `"Revérification auto"` |

- [ ] **Step 3: Traduire les onglets Outline et Goals**

| Avant | Après |
|---|---|
| `sectionHeader("Outline", width)` | `sectionHeader("Plan", width)` |
| `"(empty — ctrl+l to edit)"` (fonction `renderOutline`) | `"(vide — ctrl+l pour éditer)"` |
| `sectionHeader("Daily", width)` | `sectionHeader("Aujourd'hui", width)` |
| `"✓ goal met"` | `"✓ objectif atteint"` |
| `" to go"` (suffixe de `commafy(...)+" to go"`) | `" restants"` |
| `sectionHeader("Project", width)` (dans l'onglet Goals) | `sectionHeader("Projet", width)` |
| `sectionHeader("Time", width)` | `sectionHeader("Temps", width)` |
| `"  Today\n"` | `"  Aujourd'hui\n"` |
| `"✓ time goal met"` | `"✓ objectif de temps atteint"` |
| `"%d min to go"` | `"%d min restantes"` |
| `"  Today     "` | `"  Aujourd'hui  "` (ajuster le nombre d'espaces pour préserver l'alignement avec la ligne "Session" au-dessus — vérifier visuellement à la Tâche 13) |
| `sectionHeader("History", width)` | `sectionHeader("Historique", width)` |
| `"  %d-day streak"` | `"  série de %d jours"` |
| `"g → full history"` | `"g → historique complet"` |

Ne PAS traduire `"  Session   "` (identique/anglicisme accepté en français).

- [ ] **Step 4: Traduire l'onglet Words (par défaut)**

| Avant | Après |
|---|---|
| `kvRow("Words", ...)` (les 2 occurrences, Document et Project) | `kvRow("Mots", ...)` |
| `kvRow("Characters", ...)` | `kvRow("Caractères", ...)` |
| `kvRow("Paragraphs", ...)` | `kvRow("Paragraphes", ...)` |
| `sectionHeader("Project", width)` (dans l'onglet Words) | `sectionHeader("Projet", width)` |
| `kvRow("Chapters", ...)` | `kvRow("Chapitres", ...)` |
| `sectionHeader("Readability", width)` | `sectionHeader("Lisibilité", width)` |
| `kvStrRow("Reading time", ...)` | `kvStrRow("Temps de lecture", ...)` |
| `kvStrRow("Avg sentence", ...)` | `kvStrRow("Phrase moy.", ...)` |
| `"%.0f±%.0f wd"` | `"%.0f±%.0f mots"` |
| `sectionHeader("Overused", width)` | `sectionHeader("Surutilisés", width)` |

Ne PAS traduire `sectionHeader("Document", width)` ni `kvStrRow("Score", ...)` — identiques en français. Ne PAS toucher aux labels du score Kandel-Moles (`readabilityLabel`, déjà en français depuis un sous-projet précédent).

- [ ] **Step 5: Traduire goals.go (fonction paceLine)**

Remplacer :
```go
return "✓ target met", true
```
par :
```go
return "✓ objectif atteint", true
```

Remplacer :
```go
return "deadline passed · " + commafy(remaining) + " to go", true
```
par :
```go
return "date limite dépassée · " + commafy(remaining) + " restants", true
```

Remplacer :
```go
return "due today · " + commafy(remaining) + " to go", true
```
par :
```go
return "échéance aujourd'hui · " + commafy(remaining) + " restants", true
```

Remplacer :
```go
return "≈" + commafy(perDay) + "/day to hit " + commafy(pg.ProjectGoal) + " by " + pg.Deadline + " (" + strconv.Itoa(daysLeft) + "d)", true
```
par :
```go
return "≈" + commafy(perDay) + "/jour pour atteindre " + commafy(pg.ProjectGoal) + " avant le " + pg.Deadline + " (" + strconv.Itoa(daysLeft) + "j)", true
```

- [ ] **Step 6: Build, vet, tests — porter une attention particulière aux tests existants de goals.go**

Run: `grep -n "target met\|deadline passed\|due today\|/day to hit" goals_test.go 2>&1`
Si des tests comparent le texte exact retourné par `paceLine` à une chaîne anglaise en dur, les lire et les adapter au nouveau texte français (même principe que Task 1 Step 14).

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL (après adaptation des tests si nécessaire).

- [ ] **Step 7: Commit**

```bash
git add inspector.go goals.go
git commit -m "$(cat <<'EOF'
Traduit inspector.go et goals.go en français

Onglets (Mots/Plan/Objectifs/Analyse), en-têtes de section
(Document/Projet/Lisibilité/Surutilisés/Syntaxe...), ligne de rythme
d'écriture (paceLine). Score Kandel-Moles déjà en français, non
retouché.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Traduction de corkboard.go

**Files:**
- Modify: `corkboard.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut d'ouverture**

| Avant | Après |
|---|---|
| `m.status = "can't open the corkboard — this manuscript's manifest.json is unreadable (corrupt or a newer version)"` | `m.status = "impossible d'ouvrir le tableau — le manifest.json de ce manuscrit est illisible (corrompu ou version plus récente)"` |
| `m.status = "the corkboard only works inside a manuscript (a project with ordered chapters)"` | `m.status = "le tableau fonctionne uniquement dans un manuscrit (un projet avec chapitres ordonnés)"` |

Note terminologique : « corkboard » → « tableau » (cohérent avec la traduction déjà utilisée dans `main.go` Task 1 Step 2).

- [ ] **Step 2: Traduire les messages de statut — synopsis**

| Avant | Après |
|---|---|
| `m.status = "synopsis save failed: " + err.Error()` | `m.status = "échec de l'enregistrement du synopsis : " + err.Error()` |
| `m.status = "synopsis saved"` | `m.status = "synopsis enregistré"` |

- [ ] **Step 3: Traduire le prompt d'export et les confirmations de structure**

| Avant | Après |
|---|---|
| `m.status = "export cancelled"` | `m.status = "export annulé"` |
| `m.status = "export: m manuscript · t tufte · esc cancel"` | `m.status = "export : m manuscrit · t tufte · esc annuler"` |
| `m.status = "changes saved"` | `m.status = "modifications enregistrées"` |
| `m.status = "changes discarded"` | `m.status = "modifications annulées"` |
| `m.status = "apply changes first — y apply · esc discard"` | `m.status = "appliquer les modifications d'abord — y appliquer · esc annuler"` |
| `bar := ...Render("apply changes? y apply · esc discard")` | `bar := ...Render("appliquer les modifications ? y appliquer · esc annuler")` |

Note : `y` reste `y` dans les deux dernières lignes (cf. Global Constraints — décision actée de ne jamais franciser `y`→`o`).

- [ ] **Step 4: Traduire corkboardStatusLine (ligne de statut au-dessus des cartes)**

Remplacer :
```go
unit := "chapters"
if len(items) == 1 {
	unit = "chapter"
}
line := strconv.Itoa(len(items)) + " " + unit + " · " + commafy(total) + " words"
```
par :
```go
unit := "chapitres"
if len(items) == 1 {
	unit = "chapitre"
}
line := strconv.Itoa(len(items)) + " " + unit + " · " + commafy(total) + " mots"
```

Remplacer :
```go
line += " by " + pg.Deadline
```
par :
```go
line += " avant " + pg.Deadline
```

- [ ] **Step 5: Traduire les placeholders et panneaux de corkboardView**

| Avant | Après |
|---|---|
| `body = ...Render("(no synopsis — e to add)")` | `body = ...Render("(pas de synopsis — e pour éditer)")` |
| `cards = append(cards, ...Render("(no chapters)"))` | `cards = append(cards, ...Render("(aucun chapitre)"))` |
| `edit := framedPanel("synopsis · "+..., ..., "esc save")` | `edit := framedPanel("synopsis · "+..., ..., "esc enregistrer")` |
| `field := "retitle ▸ " + m.nameInput.View()` | `field := "renommer ▸ " + m.nameInput.View()` |
| `picks = append(picks, ...Render("(no resources to promote)"))` | `picks = append(picks, ...Render("(aucune ressource à promouvoir)"))` |
| `pick := framedPanel("add", ...)` | `pick := framedPanel("ajouter", ...)` |

- [ ] **Step 6: Traduire le pied de page principal**

Remplacer :
```go
foot := ...Render("J/K/alt reorder · e synopsis · a add · x remove · r retitle · ⏎ open · esc")
```
par :
```go
foot := ...Render("J/K/alt réordonner · e synopsis · a ajouter · x retirer · r retitrer · ⏎ ouvrir · esc")
```

- [ ] **Step 7: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 8: Commit**

```bash
git add corkboard.go
git commit -m "$(cat <<'EOF'
Traduit corkboard.go en français

Messages de statut (ouverture, synopsis, export, structure), ligne
de statut du tableau, pied de page principal. "corkboard" traduit en
"tableau" pour cohérence avec main.go.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Traduction de structure.go

**Files:**
- Modify: `structure.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les deux chaînes visibles à l'utilisateur**

Remplacer :
```go
out := []structAdd{{file: "", label: "＋ new blank chapter"}}
```
par :
```go
out := []structAdd{{file: "", label: "＋ nouveau chapitre vide"}}
```

Remplacer :
```go
it = manifestItem{File: f, Title: "Untitled"}
```
par :
```go
it = manifestItem{File: f, Title: "Sans titre"}
```

- [ ] **Step 2: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL. Si un test compare le titre par défaut `"Untitled"` littéralement (chercher `grep -n "Untitled" structure_test.go *_test.go 2>&1`), l'adapter en `"Sans titre"`.

- [ ] **Step 3: Commit**

```bash
git add structure.go
git commit -m "$(cat <<'EOF'
Traduit structure.go en français

Label "nouveau chapitre vide" dans le picker d'ajout du corkboard,
titre par défaut "Sans titre" pour un nouveau chapitre.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Traduction de outline.go

**Files:**
- Modify: `outline.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut**

| Avant | Après |
|---|---|
| `m.status = "couldn't create outline: " + werr.Error()` | `m.status = "impossible de créer le plan : " + werr.Error()` |
| `m.status = "move: put the cursor on a beat"` | `m.status = "déplacer : placez le curseur sur un beat"` |
| `m.status = "promote: put the cursor on a beat"` | `m.status = "promouvoir : placez le curseur sur un beat"` |
| `m.status = "already promoted"` | `m.status = "déjà promu"` |
| `m.status = "promote: the beat has no title"` | `m.status = "promouvoir : ce beat n'a pas de titre"` |
| `m.status = "can't promote — this manuscript's manifest.json is unreadable (corrupt or a newer version)"` | `m.status = "promotion impossible — le manifest.json de ce manuscrit est illisible (corrompu ou version plus récente)"` |
| `m.status = "promote only works inside a manuscript — this is a plain folder"` | `m.status = "la promotion ne fonctionne que dans un manuscrit — ceci est un simple dossier"` |
| `m.status = "promote failed: " + werr.Error()` (les 2 occurrences) | `m.status = "échec de la promotion : " + werr.Error()` |

- [ ] **Step 2: Traduire le message de promotion réussie (réordonnancement syntaxique)**

Le code source utilise des guillemets typographiques anglais (`“`/`”`, caractères Unicode, pas des guillemets droits ASCII). Remplacer :
```go
m.status = "promoted “" + title + "”"
```
par (guillemets français `«`/`»`, cohérent avec Task 1 Step 6 qui utilise le même style ailleurs dans `main.go`) :
```go
m.status = "« " + title + " » promu"
```

- [ ] **Step 3: Traduire l'en-tête et le pied de page de la vue Outline**

Remplacer :
```go
header := sectionHeader("OUTLINE · "+title, m.width)
```
par :
```go
header := sectionHeader("PLAN · "+title, m.width)
```

Remplacer :
```go
foot := ...Render("shift/alt+↑↓ move beat · ctrl+p promote · esc done")
```
par :
```go
foot := ...Render("shift/alt+↑↓ déplacer beat · ctrl+p promouvoir · esc terminé")
```

Note : `ctrl+p` reste cohérent avec « promouvoir » (les deux commencent par P) — aucune modification de code nécessaire, coïncidence favorable déjà confirmée lors de l'exploration.

- [ ] **Step 4: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 5: Commit**

```bash
git add outline.go
git commit -m "$(cat <<'EOF'
Traduit outline.go en français

Messages de statut (création, déplacement, promotion de beat),
en-tête PLAN et pied de page. ctrl+p reste cohérent avec "promouvoir".

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Traduction de mover.go

**Files:**
- Modify: `mover.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut et titres de panneaux**

| Avant | Après |
|---|---|
| `m.moverError = "move failed: " + err.Error()` | `m.moverError = "échec du déplacement : " + err.Error()` |
| `m.status = "moved " + filepath.Base(m.moverSource)` | `m.status = "déplacé " + filepath.Base(m.moverSource)` |
| `leftTitle = "MOVE · pick a file"` | `leftTitle = "DÉPLACER · choisir un fichier"` |
| `leftTitle = "MOVE"` | `leftTitle = "DÉPLACER"` |
| `toTitle := "TO · SOURCES"` | `toTitle := "VERS · SOURCES"` |
| `toTitle = "TO · " + filepath.Base(m.moverDestDir)` | `toTitle = "VERS · " + filepath.Base(m.moverDestDir)` |
| `toTitle = "TO · " + src.Name` | `toTitle = "VERS · " + src.Name` |
| `toTitle = "TO · " + src.Name + "/" + filepath.Base(m.moverDestDir)` | `toTitle = "VERS · " + src.Name + "/" + filepath.Base(m.moverDestDir)` |
| `rightPanel = framedPanel("TO", rightInner, rightW, 4, "")` | `rightPanel = framedPanel("VERS", rightInner, rightW, 4, "")` |

- [ ] **Step 2: Traduire le contenu du panneau gauche**

Remplacer :
```go
text = "→ move this folder"
```
par :
```go
text = "→ déplacer ce dossier"
```

Remplacer (reconstruction de phrase avec accord — attention, `kindLabel` détermine "du fichier" vs "du dossier") :
```go
kindLabel := "file"
if m.moverIsDir {
	kindLabel = "folder"
}
leftInner = "moving " + kindLabel + ":\n" + filepath.Base(m.moverSource) + "\n\nfrom: " + filepath.Base(m.moverFromDir)
```
par :
```go
kindLabel := "fichier"
article := "du "
if m.moverIsDir {
	kindLabel = "dossier"
}
leftInner = "déplacement " + article + kindLabel + " :\n" + filepath.Base(m.moverSource) + "\n\ndepuis : " + filepath.Base(m.moverFromDir)
```

(`article` vaut toujours `"du "` ici puisque "fichier" et "dossier" sont tous deux masculins — variable ajoutée pour rendre l'intention explicite plutôt que de coder en dur "déplacement du " + kindLabel, mais fonctionnellement équivalent ; simplifier en supprimant la variable `article` si le relecteur préfère `"déplacement du " + kindLabel + " :\n"` directement).

- [ ] **Step 3: Traduire le placeholder pane droit et la liste de destinations**

Remplacer :
```go
rightInner := homeDim("pick a source first →")
```
par :
```go
rightInner := homeDim("choisissez d'abord une source →")
```

Remplacer :
```go
text = "→ move into " + e.name + "/"
```
par :
```go
text = "→ déplacer dans " + e.name + "/"
```

- [ ] **Step 4: Traduire la barre de confirmation**

Remplacer :
```go
chapter, resource := "( ) chapter", "( ) resource"
```
par :
```go
chapter, resource := "( ) chapitre", "( ) ressource"
```

Remplacer :
```go
chapter = "(•) chapter"
```
par :
```go
chapter = "(•) chapitre"
```

Remplacer :
```go
resource = "(•) resource"
```
par :
```go
resource = "(•) ressource"
```

Remplacer :
```go
line = "move " + filepath.Base(m.moverSource) + " → " + dst + " as  " + chapter + "  " + resource + "   ←→ toggle · y move · esc cancel"
```
par :
```go
line = "déplacer " + filepath.Base(m.moverSource) + " → " + dst + " comme  " + chapter + "  " + resource + "   ←→ basculer · y déplacer · esc annuler"
```

Remplacer :
```go
line = "move " + filepath.Base(m.moverSource) + " → " + dst + "?   y move · esc cancel"
```
par :
```go
line = "déplacer " + filepath.Base(m.moverSource) + " → " + dst + "?   y déplacer · esc annuler"
```

Note : `y` reste `y` (cf. Global Constraints).

- [ ] **Step 5: Traduire le message d'erreur et le pied de page**

Remplacer :
```go
errLine := ...Render("⚠ " + m.moverError + "   (browse to dismiss · esc cancel)")
```
par :
```go
errLine := ...Render("⚠ " + m.moverError + "   (parcourir pour fermer · esc annuler)")
```

Remplacer :
```go
foot := ...Render("↑↓ browse · enter drill/select · .. → sources · esc cancel")
```
par :
```go
foot := ...Render("↑↓ parcourir · entrée ouvrir/sélectionner · .. → sources · esc annuler")
```

- [ ] **Step 6: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 7: Commit**

```bash
git add mover.go
git commit -m "$(cat <<'EOF'
Traduit mover.go en français

Titres de panneaux (DÉPLACER/VERS), contenu et confirmations de
déplacement de fichier/dossier, pied de page. Raccourci y préservé
tel quel.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Traduction de search.go

**Files:**
- Modify: `search.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire le champ de saisie et le nom de document par défaut**

| Avant | Après |
|---|---|
| `ti.Placeholder = "search…"` | `ti.Placeholder = "rechercher…"` |
| `name = "this document"` | `name = "ce document"` |

- [ ] **Step 2: Traduire les libellés de portée de recherche**

| Avant | Après |
|---|---|
| `scope := "Project"` | `scope := "Projet"` |
| `scope = "This document"` | `scope = "Ce document"` |
| `scope = "All sources"` | `scope = "Toutes les sources"` |

- [ ] **Step 3: Traduire les en-têtes d'écran**

| Avant | Après |
|---|---|
| `head := "Search ▸ " + m.searchInput.View()` | `head := "Recherche ▸ " + m.searchInput.View()` |
| `b.WriteString("Replace ▸ " + m.replaceInput.View() + "\n")` | `b.WriteString("Remplacer ▸ " + m.replaceInput.View() + "\n")` |

- [ ] **Step 4: Traduire les messages de statut de remplacement (avec interpolation, accord non géré pour l'instant)**

| Avant | Après |
|---|---|
| `return text, "no exact-case matches for \"" + q + "\" (search ignores case; replace matches it)", false` | `return text, "aucune correspondance exacte (casse) pour « " + q + " » (la recherche ignore la casse ; le remplacement en tient compte)", false` |
| `return text, "no matches for \"" + q + "\"", false` | `return text, "aucune correspondance pour « " + q + " »", false` |
| `status = fmt.Sprintf("replaced %d × \"%s\" → \"%s\"", n, q, r)` | `status = fmt.Sprintf("%d remplacement(s) × « %s » → « %s »", n, q, r)` |
| `status += fmt.Sprintf(" · %d case-variant(s) left (replace matches case)", ci-n)` | `status += fmt.Sprintf(" · %d variante(s) de casse restante(s) (le remplacement respecte la casse)", ci-n)` |

- [ ] **Step 5: Traduire le pied de page (compteur de résultats et aide)**

| Avant | Après |
|---|---|
| `note := fmt.Sprintf("%d matches in %d files", len(m.searchHits), len(files))` | `note := fmt.Sprintf("%d résultats dans %d fichiers", len(m.searchHits), len(files))` |
| `note += " (capped)"` | `note += " (limité)"` |
| `note = "type to search"` | `note = "tapez pour rechercher"` |
| `note = "(no matches)"` | `note = "(aucun résultat)"` |
| `footText := note + " · ↑↓ select · ⏎ open · Tab scope · ctrl+r replace · esc back"` | `footText := note + " · ↑↓ sélectionner · ⏎ ouvrir · Tab portée · ctrl+r remplacer · esc retour"` |
| `footText = "⏎ replace all in this chapter · esc cancel"` | `footText = "⏎ remplacer tout dans ce chapitre · esc annuler"` |

- [ ] **Step 6: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 7: Commit**

```bash
git add search.go
git commit -m "$(cat <<'EOF'
Traduit search.go en français

Champ de recherche, portées (Projet/Ce document/Toutes les sources),
messages de remplacement, pied de page. Accords pluriel non gérés
(approximation acceptée, cf. spec §1).

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Traduction de snapshots.go

**Files:**
- Modify: `snapshots.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut de sélection/restauration**

| Avant | Après |
|---|---|
| `m.status = "select a file to view its snapshots"` | `m.status = "sélectionnez un fichier pour voir ses snapshots"` |
| `m.status = "no snapshots yet — n to take one"` | `m.status = "aucun snapshot pour l'instant — n pour en créer un"` |
| `m.status = "restore failed: " + err.Error()` (les 2 occurrences) | `m.status = "échec de la restauration : " + err.Error()` |
| `m.status = "restored snapshot from " + s.snaps[s.sel].when.Format("2006-01-02 15:04:05")` | `m.status = "snapshot restauré du " + s.snaps[s.sel].when.Format("2006-01-02 15:04:05")` |
| `m.status = "diff mark cleared"` | `m.status = "marque de diff effacée"` |
| `m.status = "snapshot taken"` | `m.status = "snapshot créé"` |
| `m.status = "marked A · pick another snapshot, D to diff (esc clears)"` | `m.status = "marqué A · choisissez un autre snapshot, D pour diff (esc efface)"` |
| `m.status = "couldn't read snapshot"` | `m.status = "impossible de lire le snapshot"` |
| `m.status = "couldn't read snapshots"` | `m.status = "impossible de lire les snapshots"` |

Note : "diff" reste "diff" (cf. Global Constraints), ainsi que "snapshot(s)" (anglicisme technique déjà utilisé partout dans le fichier, non traduit).

- [ ] **Step 2: Traduire le placeholder de contenu illisible et les listes**

| Avant | Après |
|---|---|
| `return "(unreadable snapshot)"` | `return "(snapshot illisible)"` |
| `lines = append(lines, ...Render("… (truncated)"))` | `lines = append(lines, ...Render("… (tronqué)"))` |
| `rows = append(rows, ...Render("  (no snapshots — press n to take one)"))` | `rows = append(rows, ...Render("  (aucun snapshot — appuyez sur n pour en créer un)"))` |

- [ ] **Step 3: Traduire le pied de page en mode preview et la barre de confirmation**

Remplacer :
```go
foot := ...Render("space / esc back to list · ↑↓ other snapshots")
```
par :
```go
foot := ...Render("espace / esc retour à la liste · ↑↓ autres snapshots")
```

Remplacer :
```go
bar := ...Render(
	"restore this snapshot? the current version is backed up first — y restore · esc cancel")
```
par :
```go
bar := ...Render(
	"restaurer ce snapshot ? la version actuelle est d'abord sauvegardée — y restaurer · esc annuler")
```

- [ ] **Step 4: Traduire le pied de page principal (raccourcis, garder "diff" et lettres inchangées)**

Remplacer :
```go
hint := "↑↓ select · space preview · d diff vs current · D diff two · ⏎ restore · n new · esc back"
```
par :
```go
hint := "↑↓ sélection · space aperçu · d diff vs actuel · D diff deux · ⏎ restaurer · n nouveau · esc retour"
```

Remplacer :
```go
hint = "A marked (" + s.snaps[s.markA].when.Format("15:04:05") + ") · D on another to diff · esc clears"
```
par :
```go
hint = "A marqué (" + s.snaps[s.markA].when.Format("15:04:05") + ") · D sur un autre pour diff · esc efface"
```

- [ ] **Step 5: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 6: Commit**

```bash
git add snapshots.go
git commit -m "$(cat <<'EOF'
Traduit snapshots.go en français

Messages de statut (sélection, restauration, création), pieds de
page. "diff" et "snapshot(s)" restent en anglais (anglicismes
techniques conservés, cf. spec).

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Traduction de properties.go

**Files:**
- Modify: `properties.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut**

| Avant | Après |
|---|---|
| `m.status = "properties save failed: " + err.Error()` | `m.status = "échec de l'enregistrement : " + err.Error()` |
| `m.status = "properties saved"` | `m.status = "propriétés enregistrées"` |
| `m.status = "unsaved changes — s save · d discard · esc cancel"` (les 2 occurrences, ligne 251 et 374) | `m.status = "modifications non enregistrées — s enregistrer · d ignorer · esc annuler"` |
| `m.status = "width must be 20–200"` | `m.status = "la largeur doit être comprise entre 20 et 200"` |

Note : `s` et `d` restent inchangés (cf. Global Constraints).

- [ ] **Step 2: Traduire les labels de champs du formulaire**

| Avant | Après |
|---|---|
| `rows = append(rows, propRow("Title", ro, false))` | `rows = append(rows, propRow("Titre", ro, false))` |
| `label, val = "Title", fieldVal(p.title, editing)` | `label, val = "Titre", fieldVal(p.title, editing)` |
| `label, val = "Author", fieldVal(p.author, editing)` | `label, val = "Auteur", fieldVal(p.author, editing)` |
| `label, val = "Width", fieldVal(p.width, editing)` | `label, val = "Largeur", fieldVal(p.width, editing)` |
| `label = "Smart quotes"` | `label = "Guillemets typo."` |

Ne PAS traduire `label = "Contact"` — identique en français.

**Vérification post-traduction requise** : `propRow` utilise `labelCol = 15` avec un format `%-*s`. "Guillemets typo." (17 caractères) dépasse cette largeur — vérifier le rendu à la Tâche 13 et augmenter `labelCol` si nécessaire (chercher `labelCol` dans `properties.go`, ajuster sa valeur à 18 ou 20 si le débordement est visible).

- [ ] **Step 3: Traduire les valeurs/états affichés**

| Avant | Après |
|---|---|
| `ro := ...Render(p.origTitle + "  (folder — retitle on disk)")` | `ro := ...Render(p.origTitle + "  (dossier — renommer sur le disque)")` |
| `val = ...Render("(none)")` | `val = ...Render("(aucun)")` |
| `val = "off"` | `val = "désactivé"` |
| `val = "on"` | `val = "activé"` |

- [ ] **Step 4: Traduire l'en-tête et le pied de page**

Remplacer :
```go
header := ...Render("── properties · " + p.origTitle + " ")
```
par :
```go
header := ...Render("── propriétés · " + p.origTitle + " ")
```

Remplacer :
```go
foot := ...Render("⇥ field · ⏎ edit · space toggles smart quotes · ctrl+s save · esc back")
```
par :
```go
foot := ...Render("⇥ champ · ⏎ éditer · espace bascule guillemets typo · ctrl+s enregistrer · esc retour")
```

- [ ] **Step 5: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 6: Commit**

```bash
git add properties.go
git commit -m "$(cat <<'EOF'
Traduit properties.go en français

Labels de champs (Titre/Auteur/Largeur/Guillemets typo.), messages
de statut, en-tête et pied de page. Raccourcis s/d préservés.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Traduction de notes.go

**Files:**
- Modify: `notes.go`

**Interfaces:**
- Consumes: rien.
- Produces: rien de nouveau.

- [ ] **Step 1: Traduire les messages de statut**

| Avant | Après |
|---|---|
| `m.status = "select a file to add notes"` | `m.status = "sélectionnez un fichier pour ajouter des notes"` |
| `m.status = "notes save failed: " + err.Error()` | `m.status = "échec de l'enregistrement des notes : " + err.Error()` |

- [ ] **Step 2: Traduire la liste vide et les libellés du panneau d'édition**

| Avant | Après |
|---|---|
| `rows = append(rows, ...Render("  (no notes — press a to add one)"))` | `rows = append(rows, ...Render("  (aucune note — appuyez sur a pour en ajouter une)"))` |
| `lbl := "new note"` | `lbl := "nouvelle note"` |
| `lbl = "edit note"` | `lbl = "modifier la note"` |
| `edit := framedPanel(lbl, n.area.View(), ..., "esc save")` | `edit := framedPanel(lbl, n.area.View(), ..., "esc enregistrer")` |

- [ ] **Step 3: Traduire la confirmation de suppression et le pied de page**

Remplacer :
```go
bar := ...Render("delete this note? y delete · esc cancel")
```
par :
```go
bar := ...Render("supprimer cette note ? y supprimer · esc annuler")
```

Remplacer :
```go
foot := ...Render("↑↓ select · a add · e edit · d delete · esc back")
```
par :
```go
foot := ...Render("↑↓ sélectionner · a ajouter · e éditer · d supprimer · esc retour")
```

Note : `y`, `a`, `e`, `d` restent inchangés — `y` cf. Global Constraints, les autres coïncident naturellement avec leur mot français ("éditer" plutôt que "modifier" dans ce pied de page précis pour rester visuellement cohérent avec la lettre `e`, même si le libellé du panneau lui-même dit "modifier la note" — les deux formulations sont correctes en français dans leur contexte respectif).

- [ ] **Step 4: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 5: Commit**

```bash
git add notes.go
git commit -m "$(cat <<'EOF'
Traduit notes.go en français

Messages de statut, liste vide, panneau d'édition, confirmation de
suppression, pied de page. Raccourcis y/a/e/d préservés.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Vérification et complément du rappel d'aide sur tous les écrans

**Files:**
- Modify: potentiellement `main.go`, `home.go`, ou tout autre fichier de vue où le rappel manque (à déterminer au Step 1)

**Interfaces:**
- Consumes: le texte du rappel d'aide déjà traduit en Task 1/2 (`"F1 · ?  raccourcis"`, `"F1 · ? · esc  fermer"`).
- Produces: rien de nouveau — ajout du rappel existant sur les écrans qui en manquent, en réutilisant le texte déjà traduit.

- [ ] **Step 1: Recenser tous les écrans et vérifier la présence du rappel F1**

```bash
grep -rn "F1 · ?\|F1/?\|F1 keys\|keybindings" --include="*.go" . | grep -v _test.go
```

Comparer la liste des occurrences trouvées à la liste des écrans principaux : accueil (home.go), éditeur (main.go, déjà vérifié Task 1), sidebar/fichiers, corkboard (corkboard.go), outline (outline.go), search (search.go), mover (mover.go), snapshots (snapshots.go), properties (properties.go), notes (notes.go), inspecteur (inspector.go).

- [ ] **Step 2: Pour chaque écran principal sans rappel F1 trouvé au Step 1, l'ajouter**

Cette étape est conditionnelle au résultat du Step 1 — si le rappel F1 n'existe déjà que sur l'écran d'accueil (comme observé lors d'un test manuel antérieur du fork) et sur l'overlay d'aide lui-même, ajouter une mention courte et cohérente sur les écrans qui en manquent. Le texte exact à utiliser, pour rester cohérent avec l'existant : `"F1 · ? aide"` (version courte, pour les pieds de page déjà denses en raccourcis) ou `"F1 · ?  raccourcis"` (version longue, si la place le permet, comme sur l'écran d'accueil).

Pour chaque pied de page de vue identifié comme manquant au Step 1, ajouter `" · F1 aide"` à la fin de la chaîne de pied de page correspondante (déjà traduite dans les tâches 1-11) — lire le fichier concerné avant de modifier pour choisir le point d'insertion cohérent avec le style existant (certains pieds de page sont déjà denses, privilégier la fin de ligne après un `·` supplémentaire).

**Cette étape ne peut pas être entièrement pré-écrite dans ce plan** : le Step 1 doit d'abord établir la liste réelle des écrans manquants avant de savoir quels fichiers modifier concrètement. Documenter dans le rapport de tâche la liste des écrans où le rappel a été ajouté et pourquoi, avec les lignes exactes modifiées.

- [ ] **Step 3: Build, vet, tests**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: propre, 0 FAIL.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
Ajoute le rappel F1/aide sur les écrans qui en manquaient

Vérifié quels écrans principaux n'affichaient pas de rappel de la
touche d'aide (F1/?) et complété les pieds de page concernés, en
réutilisant le texte déjà traduit.

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Vérification manuelle finale

**Files:** aucun — tâche de vérification uniquement, pas de modification de code.

**Interfaces:**
- Consumes: l'ensemble du travail des Tasks 1-12.
- Produces: rien de nouveau, valide que l'ensemble fonctionne ensemble visuellement.

- [ ] **Step 1: Build complet et suite de tests**

Run: `export PATH="$HOME/.local/go/bin:$PATH" && go build ./... && go vet ./... && go test ./... -count=1 -v 2>&1 | tail -50`
Expected: build propre, vet sans avertissement, tous les tests PASS.

- [ ] **Step 2: Lancement manuel du binaire et parcours des écrans principaux**

```bash
go build -o /tmp/forkashi-fr-check .
```

Suivre le pattern tmux déjà utilisé lors des vérifications précédentes de ce fork (`send-keys`/`capture-pane`) pour parcourir, dans l'ordre :
1. Écran d'accueil (`home.go`) — vérifier PROJETS/DOSSIERS/AUTRE/BIBLIOTHÈQUE/FICHIERS/ÉPINGLÉS/RÉCENTS, tagline, rappel F1.
2. Ouvrir/créer un document, taper du texte, vérifier la barre de statut (« nouveau fichier », « enregistré »).
3. `ctrl+y` inspecteur — parcourir les 4 onglets (Mots/Plan/Objectifs/Analyse), vérifier l'alignement des colonnes (notamment "Aujourd'hui"/"Session", "Caractères"/"Paragraphes" qui sont plus longs qu'en anglais).
4. `ctrl+k` corkboard — vérifier le pied de page, les cartes, le picker d'ajout.
5. `ctrl+l` outline — vérifier l'en-tête PLAN et le pied de page.
6. `ctrl+f` search — vérifier les portées et le pied de page.
7. `F1`/`?` — vérifier l'intégralité du bloc d'aide traduit, en particulier l'alignement des colonnes (le texte français étant plus long, vérifier qu'aucune ligne ne déborde de la largeur du panneau `58` définie dans `main.go`).
8. Properties (`i` depuis l'accueil) — vérifier l'alignement des labels de champs (`labelCol`, cf. Task 10 Step 2).

Documenter dans le rapport toute anomalie visuelle trouvée (débordement de texte, désalignement) avec le fichier/ligne concerné, sans nécessairement la corriger dans cette tâche si c'est mineur — noter comme finding pour la review.

- [ ] **Step 3: Nettoyer le binaire de test**

```bash
rm -f /tmp/forkashi-fr-check
```

---

## Self-Review

**Couverture du spec** :
- §1 (périmètre — remplacement direct des chaînes dans les 11 fichiers listés + `goals.go` + `structure.go` découvert lors de l'exploration) : Tasks 1-11.
- §2 (raccourcis clavier inchangés) : appliqué dans chaque tâche via les Global Constraints, vérifié fichier par fichier lors de l'exploration (aucun cas trouvé nécessitant une modification de code — tous les raccourcis `y`, `a`, `e`, `d`, `x`, `r`, `m`, `t`, `n` restent valides tels quels).
- §3 (rappel F1) : Task 12.
- §4 (méthode fichier par fichier) : appliqué (13 tâches, une par fichier + une de complément aide + une de vérification finale).
- Hors périmètre du spec (contenu démo, `stopWords`/tagger POS, accords pluriels fins) : aucune tâche n'y touche, conforme.

**Cohérence des traductions transverses** : « corkboard » → « tableau » utilisé identiquement dans Task 1 (main.go) et Task 4 (corkboard.go). « diff » non traduit, cohérent dans Task 9 (seul fichier concerné). `y` jamais francisé en `o`, appliqué dans Tasks 1, 2, 4, 7, 9, 10, 11 (tous les fichiers où `y` apparaissait dans l'exploration). Le message dupliqué « manifest.json can't be renamed or removed... » (3 occurrences dans main.go) est traduit identiquement aux 3 endroits dans Task 1 Step 9.

**Point d'incertitude assumé** : Task 12 ne peut pas lister à l'avance les fichiers/lignes exacts à modifier, car cela dépend du résultat de son propre Step 1 (recensement des écrans sans rappel F1) — c'est un cas où l'information nécessaire n'existe qu'après une étape d'investigation dans le fichier réel, documenté explicitement plutôt que deviné.
