# Bascule de statut scène ↔ Ressource, et exclusion par défaut des Ressources à l'export

**Date** : 2026-08-29
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## HARD GATE — décision explicite

Comme pour [2026-08-23-export-selection-design.md](2026-08-23-export-selection-design.md) et
[2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md), ce fork
(`forkashi`) n'a pas d'app compagnon macOS connue partageant `manifest.json` — le HARD GATE de
coordination cross-repo du CLAUDE.md §1 ne s'applique pas ici. Cette spec n'introduit aucun
nouveau champ ni changement de *forme* du manifest v3 : les conversions décrites réutilisent
`Scene: true`/`items[]`/`Texts[]` tels qu'ils existent déjà (retrait/ajout d'items, retrait/ajout
d'entrées `Texts[]`) — des écritures normales déjà couvertes par `writeManifest` et l'atomic
write. Le sidecar `.okashi-export.json` change de *comportement par défaut*, pas de forme.

## Contexte

L'écran « tous les textes » (`v`, [2026-08-23-export-selection-design.md](2026-08-23-export-selection-design.md))
sait déjà convertir une **scène indépendante** en Ressource et inversement
(`convertStandaloneSceneToResource`/`convertResourceToStandaloneScene`, `exportselect.go:290-333`),
en franchissant `shift+↑↓` la frontière entre le groupe des scènes indépendantes et celui des
Ressources. Il ne sait en revanche pas faire sortir une **scène à l'intérieur d'un chapitre
multi-scènes** vers ce même statut — `moveExportSelectSceneWithinChapter` (`exportselect.go:386`)
ne connaît que « rester dans le chapitre » ou « traverser vers le chapitre adjacent » ; en bord de
chapitre sans chapitre adjacent dans la direction demandée, le déplacement est aujourd'hui
silencieusement refusé (`exportselect.go:446-448`).

Par ailleurs, le sidecar `.okashi-export.json` traite aujourd'hui tout fichier absent de sa carte
d'exclusions comme inclus par défaut, **quel que soit son statut** — y compris une Ressource
jamais vue par l'utilisateur. Cette spec inverse ce défaut pour les Ressources spécifiquement :
une Ressource doit être exclue de l'export tant que l'utilisateur ne l'a pas explicitement
incluse, alors qu'un item listé dans le manifest (chapitre, scène de chapitre, scène indépendante)
reste inclus par défaut comme aujourd'hui.

Enfin, cette spec ajoute un second point d'entrée à la conversion scène↔Ressource : un raccourci
direct depuis la sidebar principale (`filelist.go`), en plus de l'écran `v` existant — sans
dupliquer la logique de conversion, qui continue de vivre entièrement dans `exportselect.go`.

## 1. Sortie d'une scène de chapitre vers scène indépendante

**Règle validée avec l'utilisateur** : faire sortir une scène d'un chapitre multi-scènes vers
Ressource se fait en **deux étapes**, jamais directement en une seule :
1. chapitre → scène indépendante (nouveau comportement de cette spec)
2. scène indépendante → Ressource (comportement déjà existant, inchangé)

Ce choix évite d'avoir à faire choisir à l'utilisateur, en un seul geste, à la fois « sortir du
chapitre » et « perdre son statut de texte narrativement ordonné » — et reste symétrique avec le
sens inverse (une Ressource entre toujours d'abord comme scène indépendante avant, éventuellement,
de rejoindre un chapitre — comportement déjà existant, inchangé).

### Implémentation

Nouvelle fonction, miroir de `convertStandaloneSceneToResource`/`convertResourceToStandaloneScene` :

```go
// convertChapterSceneToStandalone drops file from its parent chapter's Texts[] (identified by
// folder) and adds it as a new Scene:true item at the end of items[] — the scene's own title is
// preserved. The file itself is never moved (a standalone scene's file already lives at the
// manuscript root; a chapter-scene's file lives in its chapter folder — so this DOES move the
// file, folder → root, unlike the standalone-scene↔Resource conversions which never move a
// file since both already live at the root).
func (m *model) convertChapterSceneToStandalone(folder, file string) {
    mani, present, err := readManifest(m.files.dir)
    if err != nil || !present {
        m.status = "impossible de déplacer : le manifeste est introuvable ou illisible"
        return
    }
    ch := findChapterByFolder(&mani, folder)
    if ch == nil {
        return
    }
    var moved manifestText
    kept := ch.Texts[:0]
    for _, t := range ch.Texts {
        if t.File == file {
            moved = t
            continue
        }
        kept = append(kept, t)
    }
    ch.Texts = kept

    base := filepath.Base(file)
    src := filepath.Join(m.files.dir, folder, base)
    dst := filepath.Join(m.files.dir, base)
    if _, err := os.Stat(dst); err == nil {
        m.status = "un fichier nommé " + base + " existe déjà à la racine du manuscrit"
        return
    }
    if err := safeMove(src, dst); err != nil {
        m.status = "échec du déplacement : " + err.Error()
        return
    }

    mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
        Title: moved.Title,
        Scene: true,
        Texts: []manifestText{{File: base, Title: moved.Title}},
    }})
    if err := writeManifest(m.files.dir, mani); err != nil {
        m.status = "échec du déplacement : " + err.Error()
        return
    }

    oldKey := filepath.Join(folder, base)
    excluded := loadExportSelection(m.files.dir)
    if excluded[oldKey] {
        delete(excluded, oldKey)
        excluded[base] = true
        known := map[string]bool{base: true}
        for k := range excluded {
            known[k] = true
        }
        if err := saveExportSelection(m.files.dir, excluded, known); err != nil {
            m.status = "scène convertie mais échec de la migration de la sélection d'export : " + err.Error()
            return
        }
    }
    m.reloadExportSelectAfterMove(followFile(base))
}
```

Note : contrairement aux conversions scène-indépendante↔Ressource (qui ne déplacent jamais de
fichier, les deux statuts vivant déjà à la racine), cette conversion **déplace le fichier sur
disque** du dossier du chapitre vers la racine du manuscrit — comme le fait déjà
`moveSceneBetweenChapters` pour un déplacement entre deux chapitres. Collision de nom à la racine
: refusé, aucune écriture (même famille que les collisions déjà gérées ailleurs dans
`exportselect.go`).

### Branchement dans `moveExportSelectSceneWithinChapter`

`exportselect.go:436-448`, le bloc qui aujourd'hui `return`-ne silencieusement en l'absence de
chapitre adjacent :

```go
// Crosses this chapter's boundary — find which adjacent chapter header we crossed into.
var dstHeaderIdx int
if dir < 0 {
    dstHeaderIdx = headerIdx - 1
    for dstHeaderIdx >= 0 && !entries[dstHeaderIdx].isHeader {
        dstHeaderIdx--
    }
} else {
    dstHeaderIdx = chapterEnd
}
if dstHeaderIdx < 0 || dstHeaderIdx >= len(entries) || !entries[dstHeaderIdx].isHeader {
    // No adjacent chapter in that direction — the scene exits its chapter entirely and
    // becomes a standalone scene (never directly a Resource — see design §1).
    srcFolder := m.chapterFolderForHeader(headerIdx)
    if srcFolder == "" {
        return
    }
    m.convertChapterSceneToStandalone(srcFolder, entries[i].file)
    return
}
```

Le reste de la fonction (déplacement vers un chapitre adjacent) est inchangé.

## 2. Défaut d'inclusion à l'export dépendant du type d'entrée

**Règle validée avec l'utilisateur** : une clé absente du sidecar `.okashi-export.json` signifie
désormais :
- **inclus** si l'entrée est un item listé dans le manifest (chapitre, scène de chapitre, scène
  indépendante) — comportement actuel, inchangé.
- **exclu** si l'entrée est une Ressource (fichier `.md` non listé dans `items[]`) — nouveau
  défaut.

Une valeur explicitement présente dans la carte (posée par `espace` sur l'écran `v`) fait
toujours foi, dans les deux sens — ce point ne change pas.

**Rétroactivité** (validée avec l'utilisateur) : ce nouveau défaut s'applique immédiatement à tout
manuscrit existant, dès la prochaine ouverture de l'écran `v` ou tout export du manuscrit entier —
aucune migration de données n'est nécessaire, puisque le sidecar ne stocke que des exclusions et
que le calcul du défaut se fait à la lecture, jamais à l'écriture.

**Effet de la conversion scène→Ressource (§1 ci-dessus et conversion existante)** : une scène qui
vient d'être convertie en Ressource retombe immédiatement sur le nouveau défaut « exclu », même si
elle était incluse juste avant sa conversion (aucune clé n'est écrite pour préserver son ancien
état — décision explicite validée avec l'utilisateur, cohérente avec la règle générale sans
exception pour les conversions récentes).

### Implémentation

`buildEntry` (`exportselect.go:78-90`) gagne un paramètre `isResource bool` :

```go
// buildEntry reads rel's content once to compute words/chars/preview and reports its excluded
// state from the sidecar map. isResource selects the default when rel has no entry in excluded:
// a listed item (chapter/scene) defaults to included; a Resource defaults to EXCLUDED.
func buildEntry(dir, rel, title string, indent, isResource bool, excluded map[string]bool) exportSelectEntry {
    data, _ := os.ReadFile(filepath.Join(dir, rel))
    text := string(data)
    ex, explicit := excluded[rel]
    if !explicit {
        ex = isResource
    }
    return exportSelectEntry{
        file:     rel,
        title:    title,
        preview:  previewOf(data),
        words:    wordCount(text),
        chars:    charCount(text),
        excluded: ex,
        indent:   indent,
    }
}
```

`buildExportSelectEntries` (`exportselect.go:47-74`) passe `false` pour les trois premiers appels
(chapitres/scènes de chapitre, scènes indépendantes) et `true` pour la boucle `v.loose`
(Ressources, ligne 70-72) :

```go
for _, e := range v.loose {
    out = append(out, buildEntry(dir, e.name, sectionTitle(e.name), false, true, excluded))
}
```

`exportselection.go` (le sidecar lui-même : `exportSelectionFile`, `loadExportSelection`,
`saveExportSelection`) **reste structurellement inchangé** — seul le point de lecture
(`buildEntry`) change d'interprétation d'une clé absente.

### Branchement dans `runExport`

`runExport()` (`export.go:110`, voir [2026-08-23-export-selection-design.md](2026-08-23-export-selection-design.md)
§ « Branchement du filtre ») calcule aujourd'hui `excluded[file]` directement sur la carte brute
pour décider d'inclure une Ressource. Ce calcul doit désormais appliquer la même règle de défaut
que `buildEntry` : une Ressource sans clé explicite est exclue. Le point d'appel qui construit la
liste de Ressources à exporter applique donc :

```go
for _, e := range v.loose {
    ex, explicit := excluded[e.name]
    if !explicit {
        ex = true // Resource, no explicit entry → excluded by default
    }
    if ex {
        continue
    }
    // ... lit et ajoute la Section, inchangé
}
```

Les chapitres/scènes (via `manuscriptDocFromChapters`) continuent de tester `excluded[file]`
directement (zéro-valeur `false` = inclus), puisque leur défaut ne change pas.

## 3. Raccourci sidebar `R`

Nouveau binding **`R`** (majuscule — `r` minuscule est déjà le renommage), disponible depuis la
sidebar (`main.go`, bloc `focus == focusSidebar`, à côté des bindings existants `r`/`d`/`M`/`b`/…
en `main.go:1879-1898`) :

```go
case "R":
    m.toggleSelectedSceneResourceStatus()
```

Nouvelle fonction dans `exportselect.go` (routée vers les fonctions de conversion déjà définies —
aucune logique métier dupliquée, seulement la résolution « quelle conversion appliquer à l'entrée
actuellement sélectionnée dans la sidebar ») :

```go
// toggleSelectedSceneResourceStatus advances the sidebar's selected entry one step along the
// chapter-scene → standalone-scene → Resource → standalone-scene cycle (§1 above): a scene
// nested in a chapter exits to standalone; a standalone scene becomes a Resource; a Resource
// becomes standalone again. A chapter header or a non-document entry (folder, Part row) has no
// status to toggle and is a no-op.
func (m *model) toggleSelectedSceneResourceStatus() {
    e, ok := m.files.selectedEntry()
    if !ok {
        return
    }
    switch {
    case e.isChildScene:
        m.convertChapterSceneToStandalone(e.parentFolder, e.name)
    case e.isScene:
        m.convertStandaloneSceneToResource(e.name)
    case !e.isDir && !e.isPartHeader && isResourceFile(m.files.dir, e.name):
        m.convertResourceToStandaloneScene(e.name)
    }
    // Chapter header (isDir, matched by isChapterEntry) or Part header: no-op, nothing to toggle.
    m.files.SetDir(m.files.dir) // refresh sidebar entries + word counts from the rewritten manifest
}
```

`selectedEntry() (fileEntry, bool)` : petit ajout à `filelist.go`, miroir de `selectedEntryName`
(`filelist.go:578-583`) mais renvoyant l'entrée complète (nécessaire pour lire `isChildScene`,
`parentFolder`, `isScene`) :

```go
// selectedEntry returns the full selected fileEntry, or ok=false if nothing is selected.
func (f filelist) selectedEntry() (fileEntry, bool) {
    if f.selected < 0 || f.selected >= len(f.entries) {
        return fileEntry{}, false
    }
    return f.entries[f.selected], true
}
```

`isResourceFile(dir, name string) bool` : petit helper, vrai si `name` est un fichier document
loose à la racine, non listé — réutilise la logique déjà encapsulée par `looseFiles`
(`manuscript.go:241`) plutôt que de la dupliquer :

```go
// isResourceFile reports whether name (a root-level filename) is currently a Resource — i.e.
// present among looseFiles's output for dir.
func isResourceFile(dir, name string) bool {
    v := resolveManuscript(dir, readEntries(dir))
    for _, e := range v.loose {
        if e.name == name {
            return true
        }
    }
    return false
}
```

### Rafraîchissement après bascule

Les fonctions de conversion réécrivent `manifest.json` (et déplacent un fichier pour §1). La
sidebar doit refléter immédiatement le nouveau statut : `m.files.SetDir(m.files.dir)` recharge
`view`/`entries` depuis le disque (le manifest venant d'être réécrit), replaçant naturellement
l'entrée convertie à sa nouvelle position dans l'arbre affiché — pas de mécanisme de « follow »
dédié comme sur l'écran `v` (`exportSelectFollow`) ; le curseur peut se retrouver sur une entrée
différente après un `SetDir`, ce qui est acceptable pour un raccourci sidebar ponctuel (l'écran
`v` reste le lieu pour un enchaînement de conversions/déplacements suivi visuellement).

## Hors périmètre (YAGNI)

- Pas de raccourci direct chapitre-scène → Ressource en une seule touche/un seul geste — le
  parcours en deux étapes (§1) est la seule voie, y compris depuis la sidebar.
- Pas de préservation de l'état d'inclusion lors d'une conversion scène→Ressource — la Ressource
  résultante retombe sur le nouveau défaut « exclu » (§2).
- Pas de migration explicite du sidecar `.okashi-export.json` — le nouveau défaut se calcule à la
  lecture, aucune écriture de migration n'est nécessaire ni souhaitée.
- Pas de confirmation/dialogue avant la bascule `R` dans la sidebar — cohérent avec les autres
  actions à une touche déjà en place (`d` dupliquer, `c` corkboard, etc.), la protection contre
  l'erreur reste les Snapshots existants.

## Impact CLAUDE.md

- **§ Project model** : documenter le raccourci sidebar `R` (bascule scène/Ressource) et le
  nouveau défaut d'export « Ressource exclue par défaut » à côté de la mention existante
  « Resources can now appear in an export when checked on this screen ».
- **§ Shared Contracts §1** : aucun changement de forme du manifest (retrait/ajout dans
  `Texts[]`/`items[]` avec la forme v3 existante, comme pour la spec du 2026-08-23).

## Tests

- `exportselect_test.go` :
  - `convertChapterSceneToStandalone` : retire la scène de `Texts[]` du chapitre source, déplace
    le fichier chapitre→racine, ajoute un item `Scene:true` en fin de `items[]`, migre la clé
    d'exclusion du sidecar si elle existait ; refuse et ne touche rien en cas de collision de nom
    à la racine.
  - `moveExportSelectSceneWithinChapter` sans chapitre adjacent dans la direction demandée déclenche
    `convertChapterSceneToStandalone` au lieu du `return` silencieux actuel.
  - `buildEntry`/`buildExportSelectEntries` : une Ressource sans clé explicite dans `excluded`
    obtient `excluded: true` ; un chapitre/scène/scène-indépendante sans clé explicite obtient
    `excluded: false` (comportement actuel, non régressé) ; une clé explicite (`true` ou absente
    après un `espace` qui réinclut) prime toujours sur le défaut calculé par type.
  - Conversion scène→Ressource suivie d'une lecture de `buildEntry` : la Ressource résultante est
    `excluded: true` sans qu'aucune clé n'ait été écrite dans le sidecar.
- `filelist_test.go` : `selectedEntry` renvoie l'entrée complète (dont `isChildScene`,
  `parentFolder`, `isScene`) pour l'entrée sous le curseur ; `nil`/`ok=false` hors bornes.
- `main_test.go` (ou équivalent des tests de bindings sidebar existants) :
  - `R` sur une scène de chapitre appelle la conversion vers scène indépendante et rafraîchit la
    sidebar.
  - `R` sur une scène indépendante la convertit en Ressource.
  - `R` sur une Ressource la convertit en scène indépendante.
  - `R` sur un en-tête de chapitre/Partie ne fait rien (no-op, pas de crash, pas d'écriture).
- `export_test.go`/`export_wiring_test.go` (le point de branchement de `runExport` cité au §2) :
  une Ressource sans clé explicite est exclue de l'export du manuscrit entier ; une Ressource
  explicitement incluse (`espace` sur l'écran `v`) apparaît toujours dans l'export ; les
  chapitres/scènes ne sont pas affectés par ce changement de défaut.
