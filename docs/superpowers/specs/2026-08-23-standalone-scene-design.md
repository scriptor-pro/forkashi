# Scène indépendante (hors chapitre)

**Date** : 2026-08-23
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## HARD GATE — décision explicite

Le CLAUDE.md classe tout changement de **forme** du manifest (`manifest.json`) comme un
HARD GATE nécessitant coordination avec l'app compagnon macOS partageant le même corpus. Ce
dépôt (`forkashi`, fork de [snackztime/okashi](https://github.com/snackztime/okashi)) n'a pas
d'app compagnon connue — `git remote -v` ne montre que `origin` (scriptor-pro/forkashi) et
`upstream` (snackztime/okashi), aucune app Swift consommant `manifest.json`. L'utilisateur a
confirmé explicitement (2026-08-23) que le gate ne s'applique pas à ce fork : ce changement de
schéma se fait directement ici, sans coordination externe. La section « Shared Contracts » du
CLAUDE.md est mise à jour en conséquence (voir « Impact CLAUDE.md » ci-dessous).

## Contexte

[2026-08-16-scene-creation-design.md](2026-08-16-scene-creation-design.md) a livré la création
de scène via `ctrl+n`, mais uniquement **à l'intérieur d'un chapitre déjà existant** : le picker
n'offre `s scène` que si la sélection sidebar courante résout un chapitre
(`isChapterOf`/`chapterByFolder`). Sans chapitre sélectionné (racine du manuscrit, catégorie,
ressource, rien), seules `c chapitre` et `r ressource` restent proposées.

Le besoin exprimé : pouvoir créer un texte de premier niveau — ordonné dans le manuscrit comme
un chapitre, mais sans regrouper de sous-textes ni avoir de dossier propre — sans devoir d'abord
créer ou sélectionner un chapitre conteneur. Après cadrage, il a été établi que ce n'est
**pas** équivalent à « créer un chapitre à scène unique » (option écartée) : le besoin est un
type d'entrée distinct, visuellement différencié, vivant à la racine du manuscrit.

Portée explicitement exclue de cette spec : pas de conversion scène ↔ chapitre (« promotion »),
pas de sous-dossier pour une scène indépendante.

## Modèle de données

### `manifest.go`

`manifestChapter` gagne un champ discriminant :

```go
// manifestChapter is one chapter — OR, when Scene is true, one standalone scene: a single
// ordered text with no sub-texts and no folder of its own (its file lives at the manuscript
// root, named by Texts[0].File — see manifestScenes below). Folder is "" for a scene.
type manifestChapter struct {
    Folder string         `json:"folder"`
    Title  string         `json:"title"`
    Texts  []manifestText `json:"texts"`
    Scene  bool           `json:"scene,omitempty"`
}
```

Invariant : `Scene == true` ⟺ `Folder == "" && len(Texts) == 1`. Le fichier référencé par
`Texts[0].File` vit directement à la racine du dossier manuscrit (pas de sous-dossier).

`manifestSchemaVersion` passe de **2 à 3**. `readManifest` continue de refuser tout
`schemaVersion` différent de la constante — un manifest v2 existant (sans champ `scene`, donc
`Scene` désérialise à `false` par défaut) doit être migré. Migration : au premier
`writeManifest` réussi sur un manuscrit v2, le champ `schemaVersion` est simplement réécrit à 3
(aucune transformation de données requise, `Scene: false` étant déjà la valeur par défaut
correcte pour tout chapitre existant). Pas de migration en lecture seule : tant qu'aucune
écriture n'a eu lieu, un manuscrit reste marqué v2 sur disque et continue de se lire
normalement (le champ `scene` absent vaut `false`).

### Identification stable d'une scène (remplace le rôle de `Folder`)

`findChapterByFolder` identifie un chapitre par son `Folder` (birth-stable). Une scène
indépendante n'a pas de dossier — son identifiant birth-stable est son **nom de fichier**
(`Texts[0].File`, jamais renommé après création, comme pour un chapitre). Nouvelle fonction
miroir :

```go
// findSceneByFile returns a pointer to the standalone-scene manifestChapter whose Texts[0].File
// matches file, at the manuscript root. Returns nil if no standalone scene with that file exists.
func findSceneByFile(m *manifest, file string) *manifestChapter
```

`findChapterByFolder` n'a pas besoin de changer : une entrée `Scene: true` a toujours
`Folder == ""`, donc un appel `findChapterByFolder(m, "")` resterait ambigu si plusieurs scènes
existent — **les call sites qui manipulent une scène doivent utiliser `findSceneByFile`, jamais
`findChapterByFolder("")`.**

### `manuscript.go` — vue résolue

`chapterRef` gagne le même discriminant :

```go
type chapterRef struct {
    folder string
    title  string
    texts  []textRef
    scene  bool // true: standalone scene (no folder, single root-level text)
}
```

`manifestView`'s `resolve()` : pour une entrée `Scene: true`, au lieu de
`os.Stat(dir/mc.Folder)` + `resolveChapterTexts`, on vérifie directement l'existence du fichier
racine (`os.Stat(filepath.Join(dir, mc.Texts[0].File))`) et on construit
`chapterRef{folder: "", title: mc.Title, texts: []textRef{{file: ..., title: ...}}, scene: true}`
sans lister de dossier. Fichier absent ⇒ entrée omise de l'affichage (cohérent avec la règle
existante §4.2 : une absence constatée est omise, jamais inventée).

`isChapterOf` : une scène indépendante ne doit **pas** être traitée comme un chapitre par la
logique existante qui décide si `ctrl+n` propose « ajouter une scène à ce chapitre ». Ajout
d'un filtre `!c.scene` dans `isChapterOf`, ou fonction séparée si un call site a besoin de
distinguer les deux cas (à trancher en implémentation selon les call sites réels).

`looseFiles` (résolution des Ressources) : doit continuer d'exclure tout fichier référencé par
une scène indépendante, au même titre qu'un texte de chapitre — vérifier/adapter son
implémentation pour qu'elle descende aussi dans les entrées `Scene: true`.

## Affichage (sidebar, corkboard)

Une scène indépendante apparaît **dans la même liste ordonnée que les chapitres** (même niveau
dans `items[]`, même zone de la sidebar / corkboard), mais avec une icône ou un indicateur
visuel distinct signalant l'absence de sous-textes. Le choix précis du glyphe (nerd font vs
plain, cohérence avec `OKASHI_ICONS`) est laissé à l'implémentation — inspection des glyphes
déjà utilisés ailleurs dans okashi pour rester cohérent, pas de nouvelle validation utilisateur
requise sur ce point de détail.

Le corkboard affiche une scène comme une carte sans sous-liste dépliable (puisqu'elle n'a
qu'un seul texte, structurellement identique à un chapitre à une scène pour le rendu de carte,
mais avec l'indicateur visuel de la scène).

Le mode structure (`structure.go`, reorder/insert/remove) traite une entrée scène exactement
comme une entrée chapitre dans `structureItems` (même slice, même réordonnancement) — elle ne
peut simplement pas être « dépliée » pour révéler des sous-textes.

### Correction : `enter` sur une carte corkboard multi-scènes

Bug préexistant découvert en explorant le corkboard pour cette spec, corrigé ici puisqu'il
touche la même zone de code que la distinction visuelle scène/chapitre ci-dessus :
`updateCorkboard`'s `enter` (`corkboard.go`, cas `"enter"`) ouvre toujours
`ch.texts[0].file` — la première scène d'un chapitre — quel que soit le nombre de scènes du
chapitre. Une carte représentant un chapitre à 2+ scènes n'offre aujourd'hui aucun moyen
d'atteindre ses scènes suivantes depuis le corkboard.

Corrigé en réutilisant l'écran de sélection de textes déjà livré
([2026-08-16-scene-creation-design.md](2026-08-16-scene-creation-design.md)) :
- `len(ch.texts) == 1` (y compris une scène indépendante, qui a toujours exactement un
  texte) : comportement inchangé, `enter` ouvre directement ce texte.
- `len(ch.texts) >= 2` : `enter` route vers `screenTextPicker` au lieu d'ouvrir `texts[0]`
  directement — `m.textPickerChapter` posé sur le `chapterRef` déjà résolu par le corkboard
  (`m.structureItems[m.structureSel]`), `m.textPickerDir = m.structureDir`, sortie du
  corkboard (`m.exitCorkboard()`) avant l'entrée sur `screenTextPicker`, symétrique à
  `enterTextPicker()` (`main.go:178-188`) qui pose les mêmes champs depuis la sélection
  sidebar. Pas de nouvelle fonction d'entrée nécessaire — `enterTextPicker()` suppose
  aujourd'hui une résolution via la sidebar (`m.files.selectedEntryName()`) ; le corkboard a
  déjà son `chapterRef` résolu en main et peut poser directement les mêmes champs sans repasser
  par cette résolution.
- `len(ch.texts) == 0` : comportement inchangé (message de statut « ce chapitre n'a pas encore
  de scène »).

## Création (`ctrl+n` sans chapitre présélectionné)

Le picker `ctrl+n` dans un manuscrit propose désormais **toujours** l'option scène, plus
besoin d'avoir présélectionné un chapitre dans la sidebar :

```
nouveau : c chapitre (ordonné) · r ressource (doc libre) · s scène · esc annuler
```

(Le comportement existant — `s` ciblant un chapitre précis quand un chapitre EST sélectionné,
pour ajouter une scène à ses `Texts[]` — reste inchangé et prioritaire : si la sélection
courante résout un chapitre, `s` continue d'ajouter une scène à ce chapitre, comportement livré
par la spec du 2026-08-16. Le nouveau comportement ne s'active que hors contexte chapitre.)

Nouvelle fonction, miroir de `createChapter` :

```go
// createStandaloneScene creates a new blank scene at the manuscript root — not inside any
// chapter — and appends it as a new manifestItem (Scene: true) at the end of items[]. name is
// the scene's display title, slugified into its birth-stable root-level filename.
func (m *model) createStandaloneScene(name string) {
    if strings.Contains(name, "/") {
        m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
        return
    }
    title := name
    file := slugify(title) + ".md"
    dst := filepath.Join(m.files.dir, file)
    if _, err := os.Stat(dst); err == nil {
        m.status = "un fichier nommé " + file + " existe déjà dans ce manuscrit"
        return
    }
    if err := atomicWrite(dst, []byte(""), 0o644); err != nil {
        m.status = "impossible de créer la scène : " + err.Error()
        return
    }
    mani, present, err := readManifest(m.files.dir)
    if err != nil || !present {
        m.status = "scène créée mais le manifeste est introuvable ou illisible"
        return
    }
    mani.Items = append(mani.Items, manifestItem{Chapter: &manifestChapter{
        Title: title,
        Scene: true,
        Texts: []manifestText{{File: file, Title: title}},
    }})
    if werr := writeManifest(m.files.dir, mani); werr != nil {
        m.status = "scène créée mais échec de la mise à jour du manifeste : " + werr.Error()
        return
    }
    m.files.SetDir(m.files.dir)
    m.loadFile(dst)
    m.focus = focusEditor
    m.editor.Focus()
    m.status = "nouvelle scène " + title
}
```

Insérée en fin de `items[]` (même convention que `createChapter`).

### `confirmCreate` — routage

Le routage existant sur `kind == 3` (`main.go`, spec du 2026-08-16) doit distinguer les deux
cas scène :

```go
if kind == 3 {
    if chapterFolder != "" {
        m.createScene(chapterFolder, name) // scène DANS un chapitre existant (2026-08-16)
    } else {
        m.createStandaloneScene(name) // scène indépendante (cette spec)
    }
    return
}
```

Le picker `ctrl+n` pose `m.createChapterFolder` uniquement quand la sélection résout un
chapitre (comportement du 2026-08-16, inchangé) ; une chaîne vide signale donc sans ambiguïté
le cas indépendant.

### Collision de nom

Une scène indépendante et une Ressource partagent le même espace de noms (racine du
manuscrit) — la vérification `os.Stat` avant écriture couvre déjà ce cas, message de statut
identique en cas de collision, pas de création, pas d'écriture.

## Retitrage (`r`)

Une scène indépendante retitrée met à jour `items[].chapter.title`, fichier inchangé
(birth-stable), comme un chapitre. `renameChapterTitle` (ou son appelant) doit chercher la
cible via `findSceneByFile` plutôt que `findChapterByFolder` quand l'entrée ciblée a
`Scene: true` — à brancher selon comment le call site du rename sait déjà s'il vise une scène
(le rename général sur disque, pour un fichier root-level référencé par une scène, doit rester
un no-op de renommage physique, exactement comme pour un chapitre : seul `items[].chapter.title`
change).

## Export

Les exporteurs (`export_ast.go`, `export_rtf.go`, `export_pdf.go`, etc.) parcourent déjà
`manuscriptView.parts[].chapters[]` en traitant chaque `chapterRef` comme une unité de contenu
à concaténer (titre + texte(s)). Une scène indépendante a la même forme (`folder`/`title`/
`texts`, avec `texts` de longueur 1 et `folder == ""`) — elle s'exporte donc **sans changement
de logique**, à condition qu'aucun exporteur ne suppose `folder != ""` pour construire un
chemin de lecture de fichier. Point à auditer en implémentation : tout site qui fait
`filepath.Join(dir, chapterRef.folder, text.file)` doit gérer `folder == ""` en lisant
directement `filepath.Join(dir, text.file)` — c'est déjà le cas pour le fallback legacy
(chapitres à `folder: ""` existent déjà pour les manuscrits sans manifest), donc ce chemin de
code est déjà exercé et ne devrait pas être un point de rupture.

## Hors périmètre (YAGNI)

- Pas de conversion scène ↔ chapitre (« promotion »/« démotion ») — décision explicite,
  reportée à une conception ultérieure si le besoin apparaît.
- Pas de sous-dossier pour une scène indépendante — vit toujours directement à la racine du
  manuscrit.
- Pas de scène indépendante à l'intérieur d'une Partie (`manifestItem.Part`) — cette spec ne
  couvre que le cas racine de manuscrit ; une Partie continue de ne grouper que des chapitres
  (`Chapters []manifestChapter`), inchangé.

## Impact CLAUDE.md

- **§ Shared Contracts §1** : ajouter une note precisant que ce fork (forkashi) n'a pas d'app
  compagnon macOS connue, et que le HARD GATE de coordination cross-repo ne s'applique donc pas
  ici — les changements de forme du manifest se font directement dans ce dépôt. Documenter que
  `manifestSchemaVersion` est passé à 3 avec l'ajout du champ `scene` sur `manifestChapter`
  (2026-08-23), en s'écartant délibérément du schéma amont/companion.
- **§ Project model** : documenter que `ctrl+n` propose désormais l'option scène même sans
  chapitre présélectionné (scène indépendante, premier niveau, sans dossier), en plus du
  comportement existant (scène ajoutée à un chapitre sélectionné).

## Tests

- `manifest_test.go` : round-trip JSON d'un manifest v3 avec une entrée `Scene: true` ;
  migration silencieuse v2→v3 au premier `writeManifest` (un manifest v2 sans champ `scene` se
  lit, se réécrit avec `schemaVersion: 3`, `Scene: false` implicite préservé pour les chapitres
  existants) ; `findSceneByFile` trouve/ne trouve pas selon le nom de fichier.
- `manuscript_test.go` : `resolveManuscript`/`manifestView` sur un manifest avec une scène
  indépendante — fichier présent (apparaît dans la vue, `scene: true`), fichier absent (omise,
  §4.2) ; `isChapterOf` renvoie `false` pour une scène indépendante ; `looseFiles` n'inclut pas
  le fichier d'une scène référencée.
- `main_test.go` : `createStandaloneScene` — création réussie (fichier + manifest mis à jour
  atomiquement, item en fin de liste) ; collision de nom (pas d'écriture, message de statut) ;
  `confirmCreate` route vers `createStandaloneScene` quand `createChapterFolder == ""` et vers
  `createScene` sinon ; picker `ctrl+n` propose toujours `s` (avec ou sans chapitre
  présélectionné), message de statut adapté selon le contexte.
- Export : test qu'un export (RTF ou AST) incluant une scène indépendante produit le contenu
  attendu sans erreur de chemin de fichier.
- `corkboard_test.go` : `enter` sur une carte à 1 texte ouvre directement ce texte (inchangé) ;
  `enter` sur une carte à 2+ textes route vers `screenTextPicker` avec `textPickerChapter` posé
  sur le bon `chapterRef` (au lieu d'ouvrir silencieusement `texts[0]`) ; `enter` sur une carte
  à 0 texte affiche toujours le message de statut existant (inchangé).
