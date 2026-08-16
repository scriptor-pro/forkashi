# Création de scène via ctrl+n

**Date** : 2026-08-16
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

Pas de HARD GATE contrat partagé : le manifest v2 (`manifestChapter.Texts []manifestText`) est
déjà en place depuis [2026-08-04-parts-and-multi-text-chapters-design.md](2026-08-04-parts-and-multi-text-chapters-design.md)
et n'est pas modifié ici — cette spec ajoute uniquement l'UX de création manquante, plus un
renommage terminologique.

## Contexte

Le spec du 2026-08-04 a introduit le modèle « chapitre = dossier de textes ordonnés » et
spécifiait déjà un picker `ctrl+n` à 4 choix (Chapitre / **Texte** / Ressource / Partie). Le
modèle de données (`manifestText`, `Texts[]`), la migration v1→v2, la Partie, et le
sélecteur de lecture d'un chapitre multi-textes (`screenTextPicker` / `textPickerView`,
`main.go:174-260`) ont été implémentés. **L'option de création « Texte » du picker `ctrl+n`
n'a jamais été codée** : `confirmCreate` (`main.go:2371-2394`) ne connaît que `kind == 1`
(chapitre) et `kind == 2` (ressource) ; il n'existe aucune fonction `createText`/`createScene`
dans le code actuel, et rien ne permet d'ajouter un texte à un chapitre existant.

Cette spec ferme cet écart, et au passage renomme le concept.

### Renommage : Texte → Scène

Le spec du 2026-08-04 et les commentaires de code (`manifest.go:14` « one ordered text
(scene) », `manuscript.go:16`) utilisent « texte »/« text » comme nom du concept. Cette spec
retient **« Scène »** comme terme définitif exposé à l'utilisateur (cohérent avec le
vocabulaire d'écriture narrative déjà commenté en second dans le code — Scrivener, Ulysses).
Le renommage porte sur :

- La surface UI déjà livrée : `corkboard.go:337` (`"ce chapitre n'a pas encore de texte"` →
  `"ce chapitre n'a pas encore de scène"`), `main.go:235` (`"(aucun texte dans ce chapitre)"`
  → `"(aucune scène dans ce chapitre)"`).
- Le vocabulaire du picker `ctrl+n` prévu par le 2026-08-04 (« Texte » → « Scène »).
- Les noms de code Go (`manifestText`, `textRef`, `Texts`, `texts`) : **non renommés**. Ce
  sont des identifiants internes déjà stables, référencés dans le JSON du manifest
  (`json:"texts"`) et donc dans le contrat partagé avec l'app companion — les renommer
  toucherait la forme du schéma pour un gain cosmétique nul (le JSON reste `"texts"` de toute
  façon, la question ne se pose qu'à l'affichage). Seul le libellé montré à l'utilisateur
  change ; les commentaires Go peuvent continuer à dire « text (scene) » sans que ce soit une
  incohérence.

## UX de création

### Point d'entrée 1 : picker `ctrl+n`, contextuel

`hasManifest(m.files.dir)` déclenche aujourd'hui un picker à 2 choix (`main.go:1658-1667`).
Il passe à 3 choix **uniquement quand la sélection sidebar courante est un chapitre** :

- Détection : au moment de `ctrl+n`, résoudre `m.files.selectedEntryName()` puis
  `chapterByFolder(m.files.view, folder)` — même paire d'appels que `enterTextPicker`
  (`main.go:178-188`) utilise déjà pour la même détection. Si la résolution réussit, la
  sélection est un chapitre ; sinon (racine, catégorie, ressource, rien sélectionné), pas de
  3e option.
- **Hors contexte chapitre** : picker inchangé, `c chapitre · r ressource · esc annuler`.
- **Contexte chapitre** : `c chapitre · r ressource · s scène · esc annuler`. Le chapitre
  ciblé par `s` est celui résolu ci-dessus — stocké le temps du prompt de nommage (nouveau
  champ `m.createChapterFolder string` sur le modèle, lu par `createScene` puis remis à `""`
  après usage, symétrique à la remise à zéro de `createKind` dans `confirmCreate`).

### Point d'entrée 2 : depuis `screenTextPicker`

`updateTextPicker` (`main.go:193-224`) gère déjà `KeyUp`/`KeyDown`/`KeyEnter`/`KeyEsc` sur
l'écran qui liste les scènes d'un chapitre. Ajout d'un cas `ctrl+n` : ouvre le même prompt de
nommage que le point d'entrée 1, ciblant `m.textPickerChapter.folder` (déjà résolu et déjà en
mémoire — pas de nouvelle résolution nécessaire ici).

### Création (`createScene`)

Nouvelle fonction dans `main.go`, miroir direct de `createChapter` (`main.go:2294-2331`) mais
sans créer de dossier ni de nouvel item de manifest — elle ajoute un texte à un
`manifestChapter` existant :

```go
// createScene adds a new blank scene to an existing chapter — appends a manifestText to
// its Texts and opens the new file. folder identifies the target chapter (birth-stable,
// resolved by the caller at the ctrl+n picker or from the open text-picker screen).
func (m *model) createScene(folder, name string) {
    if strings.Contains(name, "/") {
        m.status = "un nom de scène ne peut pas contenir de séparateur de chemin"
        return
    }
    title := name
    file := slugify(title) + ".md"
    chDir := filepath.Join(m.files.dir, folder)
    dst := filepath.Join(chDir, file)
    if _, err := os.Stat(dst); err == nil {
        m.status = "une scène nommée " + file + " existe déjà dans ce chapitre"
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
    ch := findChapterByFolder(mani, folder) // parcourt Items (Parties incluses) par Folder
    if ch == nil {
        m.status = "scène créée mais le chapitre est introuvable dans le manifeste"
        return
    }
    ch.Texts = append(ch.Texts, manifestText{File: file, Title: title})
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

`findChapterByFolder(mani *manifest, folder string) *manifestChapter` : petit helper à ajouter
dans `manifest.go`, parcourt `mani.Items` en descendant dans `Chapters[]` (Partie) et `Chapter`
(hors-partie) — nécessaire ici car `chapterByFolder` existant (`manuscript.go`, utilisé par
`enterTextPicker`) opère sur `manuscriptView`/`chapterRef` (vue résolue, lecture seule), pas
sur `*manifest` (écriture) ; read-modify-write atomique du manifest, cohérent avec
`createChapter`.

**Collision de fichier** : même famille d'erreur que `createChapter`/`createResource` —
message de statut, pas de création, pas d'écriture.

### Post-création

Ouverture immédiate dans l'éditeur, focus éditeur, statut `"nouvelle scène " + title` —
identique au comportement de `createChapter`.

### `confirmCreate` — routage

`confirmCreate` (`main.go:2371-2394`) gagne un 3e branchement :

```go
if kind == 3 {
    m.createScene(m.createChapterFolder, name)
    m.createChapterFolder = ""
    return
}
```

`m.createKind = 3` posé par le picker (`case "s":` dans le bloc `m.createPicker`,
`main.go:1224-1243`) et par le nouveau cas `ctrl+n` de `updateTextPicker`.

## Hors périmètre (YAGNI)

- Pas de réorganisation des scènes à la création (la nouvelle scène s'ajoute toujours en fin
  de `Texts[]` — la réorganisation existe déjà via le structure mode, inchangé ici).
- Pas de renommage groupé, pas de changement de `textPickerView` au-delà du nouveau cas
  `ctrl+n` — navigation/suppression/renommage existants ne changent pas.
- Pas de création de scène depuis l'éditeur (scène actuellement ouverte) — uniquement depuis
  la sidebar (chapitre sélectionné) ou depuis `screenTextPicker`, conformément au choix
  explicite d'écarter ce 3e contexte d'activation.

## Impact CLAUDE.md

Section « Project model (the shipped reality) » : ajouter que `ctrl+n` propose
chapitre/ressource/**scène** (scène contextuelle à un chapitre sélectionné), et que
`screenTextPicker` gagne une action de création de scène. Description de l'existant enrichie
— aucun changement de contrat partagé (le manifest n'a pas de nouveau champ ; `Texts[]` existe
déjà en v2 depuis le 2026-08-04).

## Tests

- `main_test.go` (ou fichier de test existant couvrant `createChapter`) : `createScene` sur un
  chapitre à 1 texte (devient 2, ordre préservé, fichier créé, manifest mis à jour
  atomiquement) ; collision de nom de fichier (pas d'écriture, message de statut) ; chapitre
  introuvable dans le manifest (cas défensif).
- Picker `ctrl+n` : test que l'option `s` n'apparaît que quand la sélection sidebar résout un
  chapitre (via `chapterByFolder`), absente sinon (racine/catégorie/ressource).
- `updateTextPicker` : nouveau cas `ctrl+n` ouvre le prompt de nommage avec
  `m.textPickerChapter.folder` comme cible ; annulation (`esc`) laisse le texte-picker intact,
  n'ajoute rien au manifest.
- Renommage : `corkboard_test.go`/`main_test.go` mis à jour pour les 2 messages de statut
  renommés (« texte » → « scène »).
