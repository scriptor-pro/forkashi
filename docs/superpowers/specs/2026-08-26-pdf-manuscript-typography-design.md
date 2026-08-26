# Mise en page PDF configurable (style Manuscript) — Design

> Statut : validé en brainstorming le 2026-08-26, prêt pour plan d'implémentation.

## Contexte et motivation

L'export PDF a deux styles, chacun avec une police et une mise en page **entièrement
figées dans le code** (`export_pdf.go`, `pdfStyle{...}`) :

- **Manuscript** : police Courier (core font fpdf, transcodage cp1252), corps 12pt,
  interligne 24pt, marges 72pt (1 pouce) sur les 4 côtés, indentation de première ligne
  par 5 espaces.
- **Tufte** : police ET Book (embarquée, `assets/etbook/*.ttf`), corps 12pt, titre 16pt,
  interligne 17pt, marges 108/90/108pt.

L'utilisateur veut pouvoir choisir **Times New Roman** en plus de Courier, et régler
lui-même l'interligne, la largeur de ligne (caractères par ligne, espaces comprises) et
les 4 marges — plutôt que de dépendre de constantes figées.

## Décisions actées en brainstorming

1. **Portée format : PDF uniquement.** RTF/DOCX/ODT/EPUB continuent de dériver leur
   police du style (`ExportStyle`) sans changement. Pas de découplage police/style
   au-delà du PDF dans ce chantier.
2. **Portée style : Manuscript uniquement.** Le style Tufte garde ET Book et sa mise en
   page actuelle, non négociables ici — ces nouveaux réglages n'ont aucun effet quand
   Tufte est sélectionné.
3. **Emplacement UI : écran Properties (`i`).** Pas un nouveau champ dans l'écran export
   (`ctrl+e`) — un réglage de projet persistant, au même niveau que largeur éditeur /
   guillemets typographiques / couverture.
4. **CPL vs marges selon la police.** Courier est à chasse fixe → « caractères par
   ligne » (CPL) a un sens exact et pilote les marges gauche/droite calculées. Times est
   à chasse variable → pas de CPL, l'utilisateur règle les 4 marges directement.

## Modèle de données

### `projectSettings` (`.okashi.json`) — nouveaux champs

Tous pointeurs (nil = non défini, retombe sur le défaut), suivant le pattern existant
(`Width`, `Smartquotes`, `Cover`) :

```go
type projectSettings struct {
    Width       *int    `json:"width,omitempty"`
    Smartquotes *bool   `json:"smartquotes,omitempty"`
    Cover       *string `json:"cover,omitempty"`

    // Nouveaux champs — mise en page PDF, style Manuscript uniquement.
    PdfFont         *string  `json:"pdfFont,omitempty"`         // "courier" | "times"
    PdfLineHeight   *float64 `json:"pdfLineHeight,omitempty"`   // points
    PdfCharsPerLine *int     `json:"pdfCharsPerLine,omitempty"` // Courier seulement
    PdfMarginTop    *float64 `json:"pdfMarginTop,omitempty"`    // points
    PdfMarginBottom *float64 `json:"pdfMarginBottom,omitempty"` // points
    PdfMarginLeft   *float64 `json:"pdfMarginLeft,omitempty"`   // points ; ignoré si police=Courier (calculé depuis CPL)
    PdfMarginRight  *float64 `json:"pdfMarginRight,omitempty"`  // points ; idem
}
```

`PdfMarginLeft`/`PdfMarginRight` restent stockés même en mode Courier (dernière valeur
saisie), mais sont **ignorés au rendu** tant que la police est Courier — évite de perdre
la préférence de l'utilisateur s'il bascule sur Times puis revient.

### `effectiveSettings` — nouveaux champs résolus

```go
type effectiveSettings struct {
    Author, Contact string
    Width           int
    Smartquotes     bool
    Cover           string

    PdfFont                                            string  // "courier" | "times"
    PdfLineHeight                                       float64
    PdfCharsPerLine                                     int     // n'a de sens que si PdfFont == "courier"
    PdfMarginTop, PdfMarginBottom                       float64
    PdfMarginLeft, PdfMarginRight                       float64 // valeur directe si Times ; recalculée depuis PdfCharsPerLine si Courier
}
```

### Défauts (comportement actuel inchangé sans configuration)

| Champ             | Défaut   | Note |
|-------------------|----------|------|
| `PdfFont`         | `courier`| identique à aujourd'hui |
| `PdfLineHeight`   | `24`     | identique à aujourd'hui |
| `PdfCharsPerLine` | dérivé de `72` de marge à corps 12pt Courier ≈ **65** (calculé une fois, voir ci-dessous, puis figé comme constante par défaut) |
| `PdfMarginTop`    | `72`     | identique à aujourd'hui |
| `PdfMarginBottom` | `72`     | identique à aujourd'hui |
| `PdfMarginLeft`   | `72`     | identique à aujourd'hui (valeur de repli si l'utilisateur passe à Times sans avoir réglé de marge) |
| `PdfMarginRight`  | `72`     | idem |

Le défaut de `PdfCharsPerLine` est calculé une fois pendant le développement (via
`pdf.GetStringWidth` sur une page A4 595pt de large, marges 72pt de chaque côté, Courier
12pt) et gelé comme constante entière — pas un recalcul dynamique au démarrage. Ça garantit qu'un
projet sans `.okashi.json` produit un PDF **pixel-identique** à l'export actuel.

### Bornes de validation (sur le modèle de `clampWidth`)

| Champ             | Min | Max  |
|-------------------|-----|------|
| `PdfLineHeight`   | 10  | 40   |
| `PdfCharsPerLine` | 40  | 120  |
| Chaque marge      | 20  | 200  |

Valeur hors bornes à la saisie → rejetée, message de statut, la valeur affichée revient à
l'originale (même UX que le champ Largeur existant : `p.width` / `origWidth` dans
`updatePropertiesEditing`).

## Résolution CPL → marges (Courier)

Dans `writePDF` (ou une fonction extraite, `resolveManuscriptMargins`), quand
`eff.PdfFont == "courier"` :

```go
pdf := fpdf.New("P", "pt", "A4", "")
pdf.SetFont("Courier", "", 12)
charWidth := pdf.GetStringWidth("0") // chasse fixe : n'importe quel caractère a la même largeur
pageWidth, _ := pdf.GetPageSize()
textWidth := float64(eff.PdfCharsPerLine) * charWidth
margin := (pageWidth - textWidth) / 2
if margin < 20 { margin = 20 } // garde-fou, ne devrait pas arriver avec CPL borné à 120
marginLeft, marginRight = margin, margin
```

Quand `eff.PdfFont == "times"`, `PdfMarginLeft`/`PdfMarginRight` sont utilisées
directement, aucun calcul.

## Rendu (`export_pdf.go`)

- `pdfStyle{font, bodySize, titleSize, lineHeight, indent}` gagne les marges resolues
  comme paramètres locaux à `writePDF` (pas besoin de les mettre dans `pdfStyle`, qui est
  passé à `writeBlockPDF` — les marges ne servent qu'à `pdf.SetMargins` en tête de
  fonction).
- `writePDF` prend un paramètre supplémentaire `eff effectiveSettings` (ou juste les
  champs PDF pertinents — à trancher au plan). La branche `else` (Manuscript) construit
  `cfg` et les marges depuis `eff` au lieu des constantes actuelles ; la branche Tufte
  est **strictement inchangée**.
- `cfg.font` devient `"Courier"` ou `"Times"` selon `eff.PdfFont` — `Times` est une core
  font fpdf, même mécanisme d'enregistrement que `Courier` (aucun fichier à embarquer),
  et passe par le même chemin `pdfEnc`/cp1252 (`pdfEnc` teste déjà `st == StyleTufte`,
  pas le nom de la police — aucun changement nécessaire là).
- La ligne d'en-tête courant (`SetHeaderFunc`) et la page de titre
  (`writeTitlePagePDF`) utilisent aujourd'hui `"Courier"` en dur (lignes 100 et 133) —
  ces deux call sites doivent aussi utiliser `eff.PdfFont` pour rester cohérents avec le
  corps du texte.

## UI — écran Properties (`i`)

Nouveaux `propKind` après `propCover` :

```go
propPdfFont
propPdfLineHeight
propPdfCharsPerLine  // visible seulement si PdfFont == courier
propPdfMarginTop
propPdfMarginBottom
propPdfMarginLeft    // visible seulement si PdfFont == times
propPdfMarginRight   // visible seulement si PdfFont == times
```

`p.fields` est déjà construit conditionnellement (manuscrit vs non-manuscrit) dans
`newPropertiesModel` — on étend ce calcul pour omettre `propPdfCharsPerLine` quand la
police en cours d'édition est Times, et omettre les deux champs marge quand elle est
Courier. La liste `p.fields` se recalcule au moment où `propPdfFont` bascule (comme
`smartquotes`, bascule directe par `espace`/`entrée`, pas un textinput).

Section affichée sous « Couverture », avant le pied de page :

```
  Police PDF        Courier
  Interligne (pt)   24
  Caract./ligne     65
```

ou, si Times sélectionné :

```
  Police PDF        Times
  Interligne (pt)   24
  Marge haute (pt)  72
  Marge basse (pt)  72
  Marge gauche (pt) 72
  Marge droite (pt) 72
```

Bascule de police : `espace`/`entrée` sur la ligne « Police PDF » cycle Courier ↔ Times
(comme `smartquotes` aujourd'hui — pas un textinput, un radio inline). Chaque champ
numérique suit le pattern `propWidth` : `textinput`, validation + clamp au commit
(`esc`/`entrée`), retour à la valeur d'origine si invalide.

Note : la ligne « Police PDF » et « Interligne » figurent dans `p.fields` pour **tout**
projet exportable (manuscrit ou dossier simple), comme `propAuthor`/`propWidth`
aujourd'hui — pas réservées au cas `isManuscript`. Seule la police en cours d'édition
détermine si `propPdfCharsPerLine` ou les deux champs marge apparaissent (règle décrite
plus haut) ; l'appartenance manuscrit/dossier n'entre pas dans ce calcul.

## Persistance (`properties.go`, méthode `save`)

Étendre le bloc « Width/smartquotes/Cover → per-project `.okashi.json` » pour inclure les
7 nouveaux champs sur le même modèle dirty-tracking (comparaison à `origX`, écriture
uniquement si changé, une seule écriture atomique groupée avec les champs existants
puisqu'ils partagent déjà le même fichier).

## Tests

- `settings_test.go` : `mergeSettings` avec/sans ces champs, bornes, défauts exacts
  (garantir la non-régression du PDF actuel sans `.okashi.json`).
- `export_pdf_test.go` : rendu avec Times (aucune erreur, contenu texte présent après
  extraction), rendu avec CPL personnalisé (vérifier que les marges calculées changent en
  conséquence), rendu Tufte inchangé même avec des `PdfFont`/marges renseignées dans
  `.okashi.json` (confirme le cloisonnement Manuscript-only).
- `properties_test.go` : dirty-tracking sur les nouveaux champs, bascule de police change
  bien `p.fields`, validation/clamp sur interligne, CPL, marges.

## Hors périmètre (explicitement exclu)

- RTF, DOCX, ODT, EPUB ne changent pas.
- Le style Tufte ne change pas.
- Pas de police custom arbitraire (fichier `.ttf` fourni par l'utilisateur) — seulement
  Courier et Times, les deux core fonts fpdf déjà accessibles sans embarquement.
- Pas d'unité alternative (cm/pouces) pour les marges — points uniquement, cohérent avec
  le système de coordonnées fpdf déjà en `"pt"`.

## Impact CLAUDE.md

À la fin de l'implémentation, ajouter une note sous la section Properties existante du
Project model : la police PDF (Courier/Times) et la mise en page (interligne, CPL,
marges) du style Manuscript sont désormais des réglages de projet éditables via
Properties (`i`), persistés dans `.okashi.json`. Pas de changement de schéma partagé
(pas de gate à déclencher).
