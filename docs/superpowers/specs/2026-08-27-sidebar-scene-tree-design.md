# Arborescence sidebar : scènes visibles sous un chapitre

**Date** : 2026-08-27
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

La sidebar affiche aujourd'hui un chapitre multi-scènes (`chapterRef.texts` de longueur ≥ 2)
comme une **ligne unique agrégée** (`sectionRow` — titre + décompte de mots total,
`filelist.go`). Pour ouvrir une scène précise, `enter`/clic sur cette ligne route vers l'écran
séparé `screenTextPicker` (livré par
[2026-08-16-scene-creation-design.md](2026-08-16-scene-creation-design.md)).

Le besoin exprimé : afficher un niveau d'arborescence supplémentaire directement dans la
sidebar — le nom du chapitre, puis le nom de chacune de ses scènes — plutôt que de passer par
un écran intermédiaire. Un chapitre multi-scènes devient un nœud **pliable/dépliable** ; `enter`
sur le chapitre bascule cet état au lieu d'ouvrir le text-picker ; `enter` sur une scène enfant
ouvre directement le fichier. L'état plié/déplié est **persisté par chapitre**, replié par
défaut à l'ouverture d'un manuscrit.

Portée explicitement exclue de cette spec : les chapitres à un seul texte ne gagnent aucun
triangle ni changement de comportement (`enter` ouvre toujours le fichier directement).
`screenTextPicker` n'est pas supprimé — il reste utilisé par les call sites hors sidebar
(corkboard, `ctrl+n` sur un chapitre vide) qui ne sont pas dans le périmètre de cette spec.

## Modèle de données

### `filelist.go`

`fileEntry` gagne un champ pour représenter une ligne de scène enfant, distincte d'un fichier
ordinaire ou d'une scène indépendante (`isScene`, déjà existant, reste réservé aux scènes
standalone de premier niveau) :

```go
type fileEntry struct {
    name         string
    isDir        bool
    isPartHeader bool
    isScene      bool // a standalone scene (manifest v3, chapterRef.scene) — distinct glyph
    isChildScene bool // a scene row nested under an expanded multi-text chapter (this spec)
    parentFolder string // isChildScene only: the owning chapter's folder, for chapterWords/activate
}
```

`filelist` gagne l'état plié/déplié en mémoire, tenu à jour depuis le sidecar :

```go
type filelist struct {
    // ... champs existants ...
    folded       map[string]bool // corkKey(chapterRef) → true si DÉPLIÉ (absence = replié)
    foldedRoot   string          // dossier manuscrit racine pour lequel folded a été chargé
}
```

`folded` ne stocke une clé que pour un chapitre explicitement déplié par l'utilisateur —
cohérent avec le comportement « replié par défaut » et avec le pattern de fichier absent/vide
des sidecars existants (`.okashi-synopsis.json`).

### `SetDir` — construction des lignes enfants

Dans la boucle d'assemblage de `f.entries` (`filelist.go:150-158`), pour un `chapterRef` dont
`ch.folder != "" && len(ch.texts) >= 2` :

```go
f.entries = append(f.entries, fileEntry{name: ch.folder, isDir: true})
if f.folded[corkKey(ch)] {
    for _, t := range ch.texts {
        f.entries = append(f.entries, fileEntry{
            name:         t.file,
            isChildScene: true,
            parentFolder: ch.folder,
        })
    }
}
```

Un chapitre à 0 ou 1 texte, ou une scène standalone (`ch.scene`), suit le chemin existant
sans changement.

`corkKey(ch chapterRef)` est déjà une fonction de package (`corkboard.go`), réutilisée telle
quelle — pas de nouvelle fonction de clé.

### Chargement du sidecar — une fois par manuscrit, pas à chaque `SetDir`

Contrairement à `.okashi-synopsis.json`/`.okashi-notes/` (chargés à l'entrée d'un écran modal
dédié), l'état plié s'affiche dans la sidebar principale, dont `SetDir` est appelé à haute
fréquence (chaque navigation `..`, chaque rename, chaque sortie d'écran). Charger le sidecar à
chaque appel introduirait une I/O par frappe de navigation.

`activate()` (§ ci-dessous) confirme qu'un chapitre-dossier n'est **jamais** atteint par une
descente `SetDir` — `isChapterOf` court-circuite la navigation en dossier et résout le chapitre
via `chapterByFolder` sans que `f.dir` change. Donc, pour toute navigation restant à l'intérieur
d'un même manuscrit, `dir` passé à `SetDir` est toujours soit le dossier manuscrit lui-même,
soit un sous-dossier hors chapitre (rare — okashi confine la navigation utile au niveau
manuscrit une fois résolu). `f.view.ordered()` (vrai pour `sourceManifest`/`sourceLegacy`) est
le signal disponible : quand vrai, le manuscrit racine est `dir` tel quel ; quand faux (racine
workspace ou catégorie plate), il n'y a pas de sidecar de fold pertinent et `f.folded` reste vide
sans chargement :

```go
func (f *filelist) SetDir(dir string) {
    // ... résolution existante de dir, f.view = resolveManuscript(...) ...
    if f.view.ordered() {
        if dir != f.foldedRoot {
            f.folded = loadFolded(dir)
            f.foldedRoot = dir
        }
    } else {
        f.folded = nil
        f.foldedRoot = ""
    }
    // ... suite existante (construction de f.entries) ...
}
```

## Rendu (`View`, `sectionRow`)

Un chapitre à 2+ textes affiche un glyphe pliable avant l'icône existante — ▸ replié, ▾ déplié
(ou l'équivalent plain-glyph selon `OKASHI_ICONS`, à choisir par cohérence avec le reste du
`iconSet` existant, pas de nouvelle validation utilisateur requise sur ce détail précis, comme
pour le précédent de la scène indépendante).

Une ligne `isChildScene` se rend indentée (un espace de préfixe supplémentaire par rapport à
une ligne de fichier ordinaire), avec l'icône de fichier existante, le titre de la scène
(`chapterTitle`-équivalent au niveau texte plutôt que chapitre — probablement directement
`t.title` si non vide, sinon le nom de fichier dé-slugé, à trancher en implémentation selon ce
qu'expose déjà `textRef`).

## Activation (`activate`)

`activate()` (`filelist.go:436-460`) change de branchement pour l'entrée `isDir` qui est un
chapitre :

- **1 texte** (`len(ch.texts) == 1`) : inchangé, `activateFile` direct sur `texts[0]`.
- **0 texte** : inchangé, `activateTextPicker` (chapitre vide, cas déjà géré ailleurs par le
  picker `ctrl+n` — hors périmètre ici).
- **2+ textes** : au lieu de `activateTextPicker`, bascule l'état plié :
  ```go
  key := corkKey(ch)
  f.folded[key] = !f.folded[key]
  if !f.folded[key] {
      delete(f.folded, key) // replié = absence de clé, garde le sidecar minimal
  }
  saveFolded(f.foldedRoot, f.folded, foldChapterSet(f.foldedRoot))
  f.SetDir(f.dir) // reconstruit f.entries avec/sans les lignes enfants
  return "", activateNone
  ```
  Un nouveau type de retour n'est pas nécessaire : `activateNone` convient, le caller
  (`main.go`) n'a rien à ouvrir — seule la sidebar se re-rend.

Une ligne `isChildScene` sélectionnée retourne `activateFile` sur
`filepath.Join(f.dir, e.parentFolder, e.name)`.

### `moveBy` / navigation clavier

Une ligne `isChildScene` est un item normalement sélectionnable (pas un header) — `moveBy`
(`filelist.go:316-383`) n'a besoin d'aucun changement : le curseur descend naturellement dans
les scènes enfants visibles, exactement comme il traverse aujourd'hui n'importe quelle suite de
`fileEntry`.

## Sidecar `.okashi-folded.json`

Nouveau fichier, `fold.go`, calqué sur `synopsis.go` :

```go
const foldedName = ".okashi-folded.json"
const foldedSchemaVersion = 1

type foldedFile struct {
    SchemaVersion int             `json:"schemaVersion"`
    Folded        map[string]bool `json:"folded"` // corkKey(chapterRef) → true (déplié)
}

// loadFolded reads dir's fold-state sidecar. Absent, corrupt, or a schemaVersion mismatch all
// return an empty map — never an error; a chapter with no key is folded (the default).
func loadFolded(dir string) map[string]bool { /* miroir de loadSynopses */ }

// saveFolded writes only true entries, pruned against known (every corkKey currently resolvable
// in the manuscript) — mirrors saveSynopses's prune-on-save so a deleted chapter's key never
// lingers. An empty result removes the sidecar file rather than writing "{}".
func saveFolded(dir string, folded map[string]bool, known map[string]bool) error { /* miroir de saveSynopses */ }
```

`foldChapterSet(dir string) map[string]bool` — miroir de `corkChapterSet` : recalcule depuis
`resolveManuscript(dir, ...)` l'ensemble des `corkKey` actuellement valides, utilisé comme set
de prune par `saveFolded`.

Écriture atomique via `atomicWrite`, comme tous les sidecars existants.

### Rename / delete / move — alignement sur `.okashi-synopsis.json`

- **Retitrage** (`r` sur un chapitre) : aucune migration active nécessaire — la clé
  (`corkKey` = `ch.folder`) ne change jamais au retitrage (`Folder` est birth-stable,
  `renameChapterTitle` ne touche que `Title`).
- **Suppression** : aucun hook actif ajouté dans `confirmDelete` — la clé orpheline disparaît
  au prochain `saveFolded` via le prune contre `foldChapterSet` (identique au comportement
  actuel de `.okashi-synopsis.json`, constaté à l'exploration : pas de suppression active non
  plus pour les synopsis).
- **Déplacement cross-container** (mover `M`, export-selection `v`) : aucune migration active
  ajoutée dans cette spec — comportement aligné sur le vide actuel de `.okashi-synopsis.json`
  (le mover et `moveSceneBetweenChapters` ne migrent aujourd'hui aucune clé de synopsis). Un
  chapitre déplacé perd simplement son état déplié et retrouve le défaut (replié) — aucune perte
  de données de fond, juste un état d'affichage réinitialisé. Si ce comportement s'avère gênant
  à l'usage, une migration explicite (sur le modèle de `moveSceneBetweenChapters`'s migration de
  clé d'exclusion export) pourra être ajoutée dans une itération ultérieure — hors périmètre ici
  (YAGNI, cohérent avec le fait que synopsis ne le fait pas non plus aujourd'hui).

## Devenir de `screenTextPicker` depuis la sidebar

Le chemin `activate()` → `activateTextPicker` pour un chapitre à 2+ textes n'est plus jamais
emprunté depuis la sidebar principale après cette spec (remplacé par le toggle plié/déplié
ci-dessus). `screenTextPicker` lui-même **n'est pas supprimé** : il reste le chemin utilisé par
le corkboard (`enter` sur une carte à 2+ scènes, cf.
[2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md)) et par
`ctrl+n` sur un chapitre vide. Seul l'appelant sidebar de ce chemin disparaît.

## Hors périmètre (YAGNI)

- Pas de triangle/pliage pour un chapitre à 1 texte — comportement inchangé.
- Pas de pliage pour une Partie (`isPartHeader`) — déjà non repliable, hors périmètre.
- Pas de migration active de clé au déplacement cross-container (mover/export-selection) — voir
  ci-dessus, aligné sur le comportement actuel de synopsis.
- Pas de raccourci « tout plier / tout déplier » — un chapitre à la fois, cohérent avec
  l'absence de ce besoin exprimé.
- `screenTextPicker` n'est pas retiré ni refactoré au-delà de la suppression de son unique
  call site sidebar.

## Impact CLAUDE.md

- **§ Project model** : documenter qu'un chapitre multi-scènes affiche désormais ses scènes
  inline dans la sidebar (pliable/dépliable, `enter` bascule l'état, état persisté par chapitre
  dans `.okashi-folded.json`, replié par défaut) — le text-picker sidebar-side est remplacé par
  ce mécanisme ; `screenTextPicker` reste utilisé ailleurs (corkboard, `ctrl+n` sur chapitre
  vide).

## Tests

- `fold_test.go` : round-trip `loadFolded`/`saveFolded` ; fichier absent/corrompu/mauvaise
  version → map vide (jamais d'erreur) ; `saveFolded` prune une clé absente de `known` ; un
  résultat vide supprime le fichier plutôt que d'écrire `{}` (miroir exact de
  `synopsis_test.go`).
- `filelist_test.go` : `SetDir` sur un chapitre 2+ textes NON déplié n'insère aucune ligne
  enfant ; DÉPLIÉ (via `f.folded[key] = true` préposé) insère une ligne `isChildScene` par
  texte, dans l'ordre de `ch.texts` ; un chapitre 1 texte ou une scène standalone n'insère
  jamais de ligne enfant quel que soit l'état de `f.folded`. `SetDir` ne recharge le sidecar
  que lorsque le manuscrit racine change (deux appels successifs sur le même manuscrit avec un
  sidecar modifié entre les deux ne voient pas le changement tant que `foldedRoot` ne change
  pas — comportement voulu, à documenter explicitement dans le test plutôt que corrigé).
  `activate()` sur un chapitre 2+ textes bascule `f.folded`, sauvegarde, et reconstruit
  `f.entries` sans ouvrir de fichier (`activateNone`) ; sur une ligne `isChildScene`, retourne
  `activateFile` avec le chemin joint `parentFolder/name`. `moveBy` traverse normalement les
  lignes enfants insérées (pas de saut, contrairement aux `isPartHeader`).
- Test d'intégration (ou `main_test.go`) : plier un chapitre, quitter et rouvrir le manuscrit
  (nouveau `SetDir` sur un `foldedRoot` différent puis retour) — l'état déplié survit ;
  supprimer le chapitre déplié puis modifier un autre chapitre (déclenchant un `saveFolded`) —
  la clé orpheline disparaît du fichier sur disque.
