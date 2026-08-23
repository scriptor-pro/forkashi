# Écran « tous les textes » : sélection d'export + réordonnancement libre

**Date** : 2026-08-23
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## HARD GATE — décision explicite

Comme pour [2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md), ce
fork (`forkashi`) n'a pas d'app compagnon macOS connue partageant `manifest.json` — le HARD GATE
de coordination cross-repo du CLAUDE.md §1 ne s'applique pas ici. Le sidecar introduit par cette
spec (`.okashi-export.json`) ne touche de toute façon pas la forme du manifest ; le
réordonnancement/déplacement de fichiers, en revanche, écrit `manifest.json` (retrait/ajout dans
`Texts[]`), au même titre que le mode structure existant — pas de changement de *forme* du
schéma, seulement des écritures normales déjà couvertes par les mécanismes existants
(`writeManifest`, atomic write).

## Contexte

Okashi n'offre aujourd'hui aucune vue unique listant tous les textes d'un manuscrit
indépendamment de leur statut (chapitre ordonné, scène de chapitre, scène indépendante — voir
[2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md) —, ou Ressource).
L'export (`ctrl+e`) exporte inconditionnellement toute la structure ordonnée ; il n'existe aucun
moyen d'exclure un texte précis (par exemple un brouillon ou une scène abandonnée qu'on veut
garder dans le projet sans qu'elle sorte dans le PDF final) sans le déplacer hors structure.

Cette spec introduit un écran unique remplissant trois rôles : **lister** tous les textes,
**cocher/décocher** leur inclusion dans l'export, et **réordonner/déplacer** n'importe quel
texte — y compris entre chapitres, ou entre chapitre et racine (scène indépendante/Ressource) —
sans passer par le mode structure existant (qui ne réordonne que des chapitres entiers).

Point de départ structurel : l'écran « toutes les notes » existant (`notes.go:230-324`,
commit `a1c6f02`) établit le pattern suivi ici — résolution complète en un seul passage à
l'entrée sur l'écran (I/O), puis une vue pure.

Un bug préexistant est corrigé dans le cadre de cette spec (nécessaire à sa cohérence, voir
« Correction : export multi-scènes » ci-dessous) : `manuscriptDocFromChapters`
(`export_ast.go:241`) n'exporte aujourd'hui que la **première** scène de chaque chapitre.

## Persistance : sélection d'export (`.okashi-export.json`)

Nouveau sidecar par manuscrit, même famille que `.okashi-synopsis.json` (`synopsis.go`) —
okashi-owned, hors manifest partagé :

```go
const exportSelectionName = ".okashi-export.json"
const exportSelectionSchemaVersion = 1

// exportSelectionFile is the per-manuscript export-inclusion sidecar (okashi-owned, not the
// manifest). Keyed by file path relative to the manuscript dir. A key present means the file
// is EXCLUDED from export; absence means included — default-included, so an untouched
// manuscript exports exactly as it does today.
type exportSelectionFile struct {
    SchemaVersion int             `json:"schemaVersion"`
    Excluded      map[string]bool `json:"excluded"`
}
```

- Seules les exclusions sont stockées (valeurs toujours `true` — la présence de la clé suffit,
  mais `bool` plutôt qu'un `[]string` reste cohérent avec le reste du code Go du projet pour un
  lookup O(1)).
- `loadExportSelection(dir) map[string]bool` : lecture tolérante, miroir de `loadSynopses` —
  fichier absent/corrompu/`schemaVersion` non supporté ⇒ carte vide, jamais d'erreur.
- `saveExportSelection(dir string, excluded map[string]bool, knownFiles map[string]bool) error` :
  écriture atomique, purge les clés dont `knownFiles[key]` est faux (fichier supprimé/renommé —
  auto-guérison, miroir de `saveSynopses`).
- Clé = chemin relatif au dossier manuscrit (ex. `chapitre-1/scene-2.md`, ou `notes.md` pour une
  Ressource/scène indépendante à la racine). Stable tant qu'un déplacement (voir plus bas) ne
  change pas ce chemin — un déplacement doit donc migrer la clé d'exclusion avec le fichier.

## Écran « tous les textes »

### Modèle d'affichage

Liste hiérarchique à plat, dans l'ordre du manifest puis les Ressources :

1. Pour chaque `manifestItem` racine (ou dans une Partie) qui est un chapitre non-scène : une
   **ligne-en-tête chapitre**, suivie des **lignes scène** de son `Texts[]`, indentées.
2. Pour chaque scène indépendante (`Scene: true`,
   [2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md)) : une ligne
   de premier niveau, non indentée, sans enfant.
3. Pour chaque Ressource (fichier `.md` non listé dans le manifest) : une ligne de premier
   niveau, à la suite des chapitres/scènes indépendantes, triée alphabétiquement (ordre de
   `looseFiles`/`docEntries` existant).

Chaque ligne affiche sur deux lignes de terminal :
```
[x] Titre du texte                                    1 234 mots
    Les 111 premiers caractères du texte, espaces…
```
- `[x]`/`[ ]` : état coché (inclus) / décoché (exclu) de l'export.
- Titre : `chapterRef.title` pour un chapitre/scène, titre de la scène pour une scène de
  chapitre, nom de fichier dé-slugifié pour une Ressource.
- Aperçu (nouvelle ligne, indentée, atténuée) : les 111 premiers caractères du contenu du
  fichier, espaces et sauts de ligne internes compressés en espace simple, tronqué avec `…` si
  le texte dépasse 111 caractères (comportement naturel de troncature — un texte plus court
  s'affiche intégralement sans `…`).

### Types

```go
type exportSelectEntry struct {
    file     string // chemin relatif au dossier manuscrit — clé du sidecar
    title    string
    preview  string // 111 premiers caractères, normalisés
    words    int
    chars    int  // runes, espaces comprises
    excluded bool
    indent   bool // true pour une scène à l'intérieur d'un chapitre (affichage indenté)
    isHeader bool // true pour une ligne-en-tête de chapitre (ne compte pas dans les totaux mots/chars propres — ses scènes comptent individuellement)
}

type exportSelectModel struct {
    entries []exportSelectEntry
    sel     int
}
```

Une ligne-en-tête de chapitre a elle-même une case à cocher : la décocher exclut **le chapitre
entier** (implémentation : décoche/coche en cascade toutes ses lignes-scènes enfants — un
chapitre n'a pas de contenu propre en dehors de ses scènes, donc son état coché est dérivé,
recalculé après toute bascule d'une scène enfant : coché si au moins une scène enfant est
incluse, décoché si toutes sont exclues).

### Totaux (pied de page)

Somme de `words`/`chars` de toutes les entrées **non-header, non exclues** (une ligne-en-tête
ne compte pas elle-même — ses scènes comptent), affichée avec `commafy` :
```
1 234 mots · 6 789 caractères (espaces comprises) inclus dans l'export
```
Recalculée en mémoire à chaque bascule (`espace`) ou déplacement, sans relecture disque.

### Navigation et actions

| Touche | Action |
|---|---|
| `↑`/`↓` (ou `k`/`j`) | déplace la sélection |
| `espace` | bascule coché/décoché sur la ligne sélectionnée ; sauvegarde immédiate atomique du sidecar |
| `shift+↑`/`shift+↓` | déplace la ligne sélectionnée (voir « Déplacement » ci-dessous) ; sauvegarde immédiate (fichier + manifest) |
| `esc` | retour à l'écran précédent (sidebar/corkboard) |

Point d'entrée : touche **`v`** depuis la sidebar ou le corkboard (lettres déjà prises à ce
niveau : `b c d g m n r s y` — `v` est libre).

## Déplacement (réordonnancement + changement de conteneur)

`shift+↑`/`shift+↓` déplace la ligne sélectionnée d'une position dans la liste affichée, avec
un comportement qui dépend du type de ligne :

- **Ligne-en-tête chapitre** : déplace le bloc entier (chapitre + toutes ses scènes) — modifie
  uniquement l'ordre de `items[]`, jamais de changement de conteneur (un chapitre n'a pas de
  parent). Équivalent, au niveau donnée, à un reorder du mode structure existant, déclenché
  depuis ce nouvel écran.
- **Ligne scène (dans un chapitre)** : si le déplacement reste à l'intérieur des limites de son
  chapitre actuel, réordonne simplement dans `Texts[]`. Si le déplacement franchit la frontière
  d'un autre chapitre (ou d'une scène indépendante/Ressource, voir plus bas), la scène **change
  de conteneur** :
  - Retirée du `Texts[]` du chapitre source.
  - Le fichier `.md` est déplacé physiquement sur disque du dossier du chapitre source vers le
    dossier du chapitre cible (`os.Rename` — même volume garanti, manuscrit = un seul dossier
    arborescent).
  - Ajoutée au `Texts[]` du chapitre cible, à la position traversée.
  - La clé du sidecar `.okashi-export.json` (si le fichier avait une exclusion) migre avec lui
    (ancienne clé → nouvelle clé, même valeur).
  - `manifest.json` réécrit atomiquement (un seul `writeManifest` couvrant retrait + ajout).
- **Ligne scène indépendante ou Ressource** : peut être réordonnée librement parmi les lignes de
  premier niveau (scènes indépendantes entre elles, Ressources entre elles — une Ressource ne
  fait pas partie de `items[]`, donc son « ordre » n'affecte que l'affichage/l'export, pas le
  manifest). Franchir la frontière d'un chapitre la fait **entrer** dans ce chapitre comme
  nouvelle scène (fichier déplacé racine → dossier chapitre, ajoutée à `Texts[]`) — l'inverse
  du cas précédent. Franchir la frontière depuis une scène indépendante vers zone Ressource (ou
  l'inverse) est un changement purement de représentation :
  - Scène indépendante → Ressource : retirée de `items[]` (son fichier reste sur disque à la
    racine, désormais non listé — devient une Ressource par la définition existante).
  - Ressource → scène indépendante : ajoutée à `items[]` comme nouvelle entrée `Scene: true`
    (voir [2026-08-23-standalone-scene-design.md](2026-08-23-standalone-scene-design.md)).

**Collision de nom** : si le nom de fichier cible existe déjà dans le dossier de destination
(chapitre cible ou racine), le déplacement est refusé — aucune écriture, message de statut
(même famille que les collisions déjà gérées par `createScene`/`createStandaloneScene`). La
ligne reste à sa position d'origine.

**Sauvegarde** : chaque déplacement s'applique **immédiatement** (fichier déplacé + manifest
réécrit avant que la touche suivante soit traitée) — pas de mode « staged + commit » comme le
mode structure existant ; choix explicite pour garder un seul modèle mental simple avec la
case à cocher (immédiate elle aussi), au prix d'une écriture disque par déplacement.

## Correction : export multi-scènes

`manuscriptDocFromChapters` (`export_ast.go:241`) ne lit aujourd'hui que `ch.texts[0]` — les
scènes suivantes d'un chapitre multi-scènes sont silencieusement ignorées à l'export. Corrigée
dans le cadre de cette spec : itère sur **tous** les `ch.texts`, chaque scène devenant sa propre
`Section` dans le `ManuscriptDoc` (titre = titre de la scène, pas du chapitre — changement de
granularité assumé : la table des matières exportée reflètera désormais une entrée par scène,
pas une entrée par chapitre, pour tout chapitre multi-scènes).

```go
func manuscriptDocFromChapters(dir string, parts []partRef, excluded map[string]bool) ManuscriptDoc {
    var doc ManuscriptDoc
    for _, p := range parts {
        for _, ch := range p.chapters {
            for _, t := range ch.texts {
                file := filepath.Join(ch.folder, t.file) // ch.folder == "" pour une scène indépendante
                if excluded[file] {
                    continue
                }
                data, err := os.ReadFile(filepath.Join(dir, file))
                if err != nil {
                    continue
                }
                doc = append(doc, Section{Title: t.title, Blocks: parseSection(data)})
            }
        }
    }
    return doc
}
```

## Branchement du filtre dans `runExport`

`runExport()` (`export.go:110`), dans la branche `m.exportWholeManuscript()` :
1. Charge `excluded := loadExportSelection(dir)` (aliasé aux clés du sidecar).
2. Appelle `manuscriptDocFromChapters(dir, v.parts, excluded)` (signature étendue ci-dessus).
3. Ajoute un nouveau parcours pour les Ressources : après les chapitres, pour chaque Ressource
   de `v.loose` **non exclue**, lit et ajoute une `Section` (titre = nom dé-slugifié), dans
   l'ordre alphabétique déjà utilisé par `v.loose`. **Nouveau comportement** : une Ressource
   peut désormais apparaître dans un export, ce qui n'était jamais possible auparavant.

Le cas « export du document courant seul » (`m.exportWholeManuscript() == false`, pas depuis le
corkboard) reste **inchangé** — la sélection d'export ne s'applique qu'à l'export du manuscrit
entier.

## Comptage de caractères

Nouvelle fonction jumelle de `wordCount` (`main.go:2854`) :

```go
// charCount returns the rune count of s, spaces included — used by the export-selection
// screen's character total. utf8.RuneCountInString, not len(s): len would count bytes, wrong
// for accented French text.
func charCount(s string) int {
    return utf8.RuneCountInString(s)
}
```

## Hors périmètre (YAGNI)

- Pas de propagation de l'exclusion aux compteurs de mots existants ailleurs (sidebar,
  corkboard, pace/burndown) — ils continuent de compter tous les mots du projet, inchangés.
  Décision explicite : un seul endroit (cet écran + l'export réel) reflète l'exclusion.
- Pas de déplacement d'un texte vers/depuis une Partie (`manifestItem.Part`) — cette spec
  couvre le déplacement entre chapitres bares (hors Partie) et racine (scène indépendante/
  Ressource) ; une Partie continue de se réordonner via le mode structure existant, inchangé.
- Pas d'annulation (undo) d'un déplacement — comme le reste d'okashi, la protection contre
  l'erreur passe par les Snapshots (`.okashi-bak/`, `b` depuis la sidebar), pas par un undo
  dédié à cet écran.

## Impact CLAUDE.md

- **§ Project model** : documenter le nouvel écran (`v` depuis la sidebar/corkboard),
  l'inclusion/exclusion d'export par texte (sidecar `.okashi-export.json`), et le fait qu'une
  Ressource peut désormais être incluse dans un export.
- **§ Shared Contracts §1** : aucun changement de *forme* du manifest par cette spec (le
  déplacement de textes réécrit `manifest.json` avec la forme v3 existante — retrait/ajout dans
  `Texts[]`/`items[]` — sans introduire de nouveau champ).

## Tests

- `exportselection_test.go` : round-trip du sidecar (exclusion persistée, purge des clés
  orphelines après suppression/renommage d'un fichier référencé) ; `loadExportSelection`
  tolérant sur fichier absent/corrompu/schéma non supporté.
- `notes_test.go`-like (nouveau fichier, ex. `exportselect_test.go`) : `enterExportSelect`
  construit la liste dans l'ordre attendu (chapitres+scènes indentées, scènes indépendantes,
  Ressources triées) avec le bon aperçu (111 caractères, normalisé) et les bons
  `words`/`chars` ; bascule `espace` persiste immédiatement et recalcule les totaux ; case
  chapitre dérivée (coché si ≥1 scène enfant incluse).
- Déplacement : scène réordonnée dans son propre chapitre (pas de changement disque hors
  manifest) ; scène franchissant la frontière d'un autre chapitre (fichier déplacé, manifest
  mis à jour, clé du sidecar migrée) ; collision de nom au déplacement (refusé, aucune
  écriture) ; scène indépendante ↔ Ressource (ajout/retrait dans `items[]` sans déplacement de
  fichier, puisque toutes deux vivent à la racine) ; chapitre entier déplacé (bloc + ses
  scènes, ordre `items[]` seul affecté).
- `export_ast_test.go` : `manuscriptDocFromChapters` produit une `Section` par scène (pas
  seulement la première) ; respecte la carte d'exclusion ; une scène indépendante (`folder ==
  ""`) se lit correctement à la racine.
- `export_wiring_test.go` (ou équivalent) : `runExport` sur le manuscrit entier inclut les
  Ressources cochées, exclut les textes décochés, export du document courant seul reste
  inchangé par la présence d'un sidecar.
