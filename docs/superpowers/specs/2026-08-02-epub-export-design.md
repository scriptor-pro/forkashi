# Export EPUB + refonte du choix de format à l'export

**Date** : 2026-08-02
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

okashi exporte aujourd'hui systématiquement 4 formats à chaque `ctrl+e` — `.rtf`, `.pdf`,
`.docx`, `.odt` — écrits l'un après l'autre par `runExport()` (`export.go`), sans que
l'utilisateur choisisse individuellement lesquels produire. Le seul choix actuel est le
**style** (`m` Manuscript / `t` Tufte), qui affecte la typographie mais pas la liste des
formats écrits.

Ce sous-projet ajoute un **cinquième format, EPUB**, seul format de livre électronique
pertinent aujourd'hui pour la diffusion francophone (MOBI est abandonné, AZW3/KFX sont
propriétaires et Amazon convertit lui-même depuis l'EPUB à l'import KDP ; voir recherche
documentée en conversation). L'ajout de l'EPUB motive une refonte plus large : puisque tous
les formats ne se valent pas selon l'usage visé (soumission à un éditeur vs. diffusion
numérique), l'utilisateur doit pouvoir choisir explicitement quels formats produire à chaque
export, plutôt que de toujours recevoir les 4 (bientôt 5) fichiers.

## 1. Architecture du writer EPUB

Nouveau fichier `export_epub.go`, suivant le patron déjà établi par `export_docx.go` /
`export_rtf.go` / `export_odt.go` / `export_pdf.go` :

```go
func writeEPUB(doc ManuscriptDoc, st ExportStyle, meta Meta) ([]byte, error)
```

Consomme le même AST style-agnostique produit une fois par `export_ast.go`
(`Section`, `Paragraph`, `Heading`, `Blockquote`, `List`, `SceneBreak`, `Endnote`,
`Endnotes`) — **aucun changement au parsing Markdown existant**.

Le fichier `.epub` produit est une archive ZIP contenant :

- `mimetype` — non compressé (stocké, pas déflaté), doit être le premier fichier de
  l'archive ; contient littéralement `application/epub+zip`.
- `META-INF/container.xml` — pointeur standard vers le manifeste OPF.
- `OEBPS/content.opf` — manifeste : métadonnées (`Meta.Author`, `Meta.Title`), liste des
  fichiers du package, référence de couverture si présente.
- `OEBPS/toc.ncx` + une page de navigation XHTML (EPUB3 nav) — table des matières, une
  entrée par `Section` du `ManuscriptDoc`, dans l'ordre du document.
- `OEBPS/chapter-NN.xhtml` — un fichier XHTML par chapitre, produit par un walk de l'AST
  (miroir de ce que fait déjà `export_docx.go` pour l'OOXML, mais générant du XHTML).
- `OEBPS/style.css` — feuille de style minimale (une seule, partagée par tous les chapitres ;
  pas de personnalisation par l'utilisateur dans cette première version).
- `OEBPS/cover.xhtml` — page de couverture (texte généré ou image, voir §3).

Les notes de bas de page (`Endnotes`) sont rendues **en fin de chapitre**, dans une section
"Notes" en XHTML — identique au traitement déjà en place pour RTF/PDF/DOCX/ODT. L'EPUB3
supporte nativement les vraies notes cliquables en popup, mais ce n'est **pas** utilisé ici :
décision actée de rester cohérent avec le comportement des 4 autres formats plutôt que
d'introduire une divergence de rendu pour la même donnée source.

## 2. Écran de sélection à l'export (refonte du prompt `ctrl+e`)

L'écran actuel (`m.exportPrompt`, gérée par `case "m":`/`case "t":` dans `main.go` et
`corkboard.go`) est remplacé par une liste combinée navigable :

```
Export ▸
  [ ] RTF
  [ ] PDF
  [ ] DOCX
  [ ] ODT
  [ ] EPUB

  Style : (•) Manuscript   ( ) Tufte

  ↑↓ naviguer · espace cocher · ←→ style · ↵ exporter · esc annuler
```

- **Rien n'est coché à l'ouverture** — l'utilisateur doit choisir explicitement au moins un
  format à chaque export. Aucune mémorisation du dernier choix entre deux exports.
- **↑/↓** déplacent le curseur entre les 5 cases de format et la ligne de style.
- **espace** bascule la case de format sous le curseur (coché/décoché) ; sur la ligne de
  style, bascule entre Manuscript et Tufte.
- **←/→** basculent directement le style quand le curseur est sur la ligne Style (raccourci
  en plus de espace, pas un remplacement).
- **↵** valide et lance l'export. Si aucune case de format n'est cochée :
  `m.status = "choisissez au moins un format"`, l'écran reste ouvert, rien n'est exporté.
- **esc** annule sans exporter, comme le comportement actuel.

**Changement de comportement de `runExport()`** : la fonction ne produit plus systématiquement
4 fichiers — elle boucle sur la liste des formats effectivement cochés. Le message de statut
final liste dynamiquement ce qui a été exporté, par ex.
`"exporté <slug>.pdf + .epub vers export/"` (au lieu de la liste fixe
`" + .rtf + .pdf + .docx + .odt"` actuelle).

**Ordre et arrêt sur erreur** : comportement actuel conservé — un échec sur un format
interrompt l'export sans tenter les formats suivants dans la liste cochée (pas de
best-effort partiel).

## 3. Couverture EPUB

**Properties** (`i` depuis le hub) gagne un nouveau champ **Couverture** : un chemin de
fichier (relatif au dossier du projet, ou absolu), stocké dans `<project>/.okashi.json` aux
côtés de `Width` et `Smartquotes` — c'est un réglage **par projet**, pas personnel comme
Author/Contact (qui vivent dans le `config.json` global de l'utilisateur).

Saisie : champ texte libre, édité au clavier comme Titre/Auteur/Contact/Largeur aujourd'hui
— l'utilisateur tape ou colle un chemin, aucun sélecteur de fichier graphique.

**Comportement à l'export EPUB** :

- **Champ vide (par défaut)** : page de couverture XHTML générée automatiquement — titre et
  auteur centrés, sur fond blanc, mise en forme minimale via `style.css`. Pas d'image.
- **Champ renseigné** : okashi tente de lire le fichier au chemin indiqué au moment de
  l'export.
  - **Formats acceptés** : `.jpg`/`.jpeg` et `.png` uniquement (couverture universelle sur
    les liseuses). Tout autre format est traité comme invalide.
  - **Fichier trouvé et lisible, format valide** → copié dans `OEBPS/cover.jpg` (ou `.png`)
    et référencé comme couverture dans le manifeste OPF.
  - **Fichier introuvable, illisible, ou format non supporté** → repli silencieux sur la
    couverture texte générée, **avec un avertissement explicite dans le message de statut**
    (ex. `"exporté … — couverture introuvable, page de titre utilisée"`), pour ne jamais
    faire échouer l'intégralité de l'export à cause d'un chemin d'image cassé.

## 4. Gestion d'erreurs et écriture atomique

`writeEPUB()` retourne `([]byte, error)`, comme les autres writers. `runExport()` écrit le
résultat via `atomicWrite()` (temp-file + rename), cohérent avec la règle architecturale du
projet (CLAUDE.md, "Files & sync") — pas de fichier `.epub` partiellement écrit en cas
d'échec en cours de génération.

Cas d'erreur :

- **Échec de génération** (marshalling XML/XHTML cassé, contenu non gérable) →
  `m.status = "export failed (epub): " + err.Error()`, même pattern que
  `"export failed (pdf/docx/odt): "` déjà en place.
- **Manuscrit vide** → déjà intercepté en amont par le garde `len(doc) == 0` existant dans
  `runExport()`, avant d'atteindre l'écriture d'un format quelconque — aucun nouveau
  garde-fou à ajouter pour l'EPUB spécifiquement.
- **Couverture invalide** → repli silencieux + avertissement de statut, §3 ci-dessus (pas un
  échec bloquant de l'export).

## 5. Tests

Suivant le pattern déjà établi (`export_pdf_test.go`, `export_titlepage_test.go`,
`export_wiring_test.go`) :

- **`export_epub_test.go`** :
  - `TestWriteEPUBManuscriptValid` / `TestWriteEPUBTufteValid` — génère un EPUB depuis un
    `ManuscriptDoc` de test pour chaque style, vérifie via `archive/zip.NewReader` que
    l'archive est valide, que `mimetype` est présent en premier et **non compressé**
    (`zip.Store`, pas `zip.Deflate`), et que `content.opf` contient Author/Title attendus.
  - `TestWriteEPUBTableOfContents` — plusieurs chapitres → `toc.ncx`/nav XHTML contient
    autant d'entrées que de `Section`, dans le même ordre que `ManuscriptDoc`.
  - `TestWriteEPUBCoverImageValid` — chemin d'image valide (fixture JPEG/PNG dans
    `t.TempDir()`) → `OEBPS/cover.jpg`/`.png` présent dans l'archive, référencé dans le
    manifeste.
  - `TestWriteEPUBCoverImageInvalidFallsBackToText` — chemin invalide ou format non
    supporté → pas de crash, couverture texte générée, statut d'avertissement retourné/vérifié.
- **Test d'intégration `runExport`** (miroir de l'intégration `.odt` déjà en place) : format
  EPUB coché → fichier `.epub` écrit sur disque dans `<project>/export/` ; non coché → absent.
- **Test de l'écran de sélection** : navigation ↑/↓, bascule espace, validation ↵ avec/sans
  case cochée (message d'erreur attendu si aucune), esc annule.

Pas de dépendance externe de validation stricte EPUB3 (pas d'`epubcheck` embarqué dans cette
version) — la validation reste structurelle (ZIP bien formé, XML/XHTML bien formé), pas une
conformité exhaustive à la spécification EPUB3. Un problème de compatibilité avec une liseuse
particulière serait traité comme un correctif ultérieur si signalé.

## Hors périmètre de ce sous-projet

- Notes de bas de page cliquables en popup (EPUB3 natif) — endnotes en fin de chapitre
  seulement, cohérent avec les 4 autres formats.
- Personnalisation de la feuille de style EPUB par l'utilisateur (police, taille, couleurs) —
  une seule CSS fixe.
- Sélecteur de fichier graphique pour la couverture — saisie texte du chemin uniquement.
- Validation stricte EPUB3 via un outil externe (`epubcheck`).
- MOBI, AZW3/KFX, FictionBook, DAISY — écartés comme obsolètes, propriétaires fermés, ou hors
  du marché francophone pertinent (voir recherche en conversation).
