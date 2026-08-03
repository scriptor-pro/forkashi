# Notes de révision dans l'inspecteur (onglet Mots)

**Date** : 2026-08-03
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

okashi/forkashi a deux fonctionnalités aujourd'hui indépendantes l'une de l'autre :

- Les **notes de révision** (`n` depuis la sidebar) : annotations libres par fichier, stockées dans un
  sidecar okashi-owned `.okashi-notes/<base>.json` (`notes.go`). v1 = notes scopées au chapitre entier
  (pas d'ancrage ligne/phrase).
- L'**inspecteur** (`ctrl+y`, `inspector.go`) : panneau latéral en 4 onglets (Mots / Plan / Objectifs /
  Analyse) qui affiche déjà, dans l'onglet **Mots**, les statistiques du document ouvert (mots,
  caractères, paragraphes), les stats du projet, la lisibilité (Kandel-Moles) et les mots surutilisés.

Aujourd'hui, si un fichier a des notes de révision attachées, rien ne le signale pendant l'écriture — il
faut penser à rouvrir l'écran Notes (`n`) pour s'en souvenir. Cette spec ajoute un rappel visuel dans
l'inspecteur, déjà visible en continu pendant l'édition.

## Objectif

Quand le fichier actuellement ouvert dans l'éditeur a une ou plusieurs notes de révision, les afficher
dans l'onglet Mots de l'inspecteur, sous les informations déjà présentes (Document / Projet / Lisibilité
/ Surutilisés).

## Emplacement et déclenchement

- Nouvelle section **NOTES**, ajoutée en dernier dans le rendu du cas `default: // tabWords` de
  `inspectorModel.View()` (`inspector.go`), donc visuellement tout en bas de l'onglet Mots — après
  "Surutilisés" quand cette section existe, sinon après "Lisibilité".
- La section n'apparaît **que si un fichier est ouvert** — même garde que les sections
  Lisibilité/Surutilisés existantes (`doc.words > 0`). Au hub, ou tant qu'aucun fichier n'est chargé
  dans l'éditeur, la section Notes n'est pas rendue du tout (pas de placeholder hors contexte fichier).

## Comportement d'affichage

**Fichier ouvert sans note** : une ligne placeholder discrète (style `subtle`, cohérent avec le
placeholder déjà utilisé dans `notesView()`) :

```
(aucune note — n pour en ajouter)
```

**Fichier ouvert avec une ou plusieurs notes** : chaque note listée sur sa propre ligne, dans l'ordre où
elle est stockée (ordre `notes.go` — ordre de création). Pour chaque note :

- seule la **première ligne** du texte est affichée (une note peut être multi-lignes, saisie via le
  textarea de l'écran Notes) ;
- si la note continue au-delà de la première ligne, un indicateur `…` est ajouté — même convention que
  `notesView()` (`first[:idx] + " …"`) ;
- la ligne résultante est tronquée à la largeur du panneau via `ansi.Truncate`, cohérent avec le reste de
  l'inspecteur (`renderOutline`, `kvRow`).

Aucune limite de nombre de notes affichées (contrairement à "Surutilisés" qui plafonne à 5) : le v1 des
notes de révision est scopé au chapitre, donc un fichier n'en accumule typiquement que quelques-unes ;
si ça devient un problème visuel en usage réel, un plafond pourra être ajouté plus tard (YAGNI pour
l'instant).

## Câblage technique

- `computeDocStats(text string) docStats` ne change pas — il n'a pas accès au chemin du fichier et ce
  n'est pas son rôle (il calcule des stats à partir du seul contenu texte).
- Le chargement des notes se fait séparément, là où `doc`/`proj` sont déjà calculés avant l'appel à
  `m.inspector.View(...)` (`main.go`, juste avant la ligne qui construit `insInner`) :
  ```go
  notes := loadNotes(m.currentFile)
  ```
  `loadNotes` existe déjà dans `notes.go` et est déjà tolérant (fichier absent/corrompu/schéma
  incompatible → `nil`), donc aucun nouveau cas d'erreur à gérer ici.
- `inspectorModel.View()` gagne un paramètre supplémentaire `notes []note`, ajouté en dernier dans la
  signature existante (après `analysis analysisState`) et utilisé uniquement dans la branche
  `tabWords` :
  ```go
  func (in inspectorModel) View(width int, doc docStats, proj projStats, outline string,
      goals goalStats, analysis analysisState, notes []note) string
  ```
  Le seul autre appelant de `View()` est `main.go:1744` — à mettre à jour avec `notes` au même endroit
  que `notes := loadNotes(m.currentFile)` ci-dessus.
- Nouvelle fonction dans `inspector.go`, suivant le pattern de `renderOutline` :
  ```go
  func renderNotesSection(notes []note, width int) string
  ```
  Rendu :
  - `sectionHeader("Notes", width)` pour le titre de section (cohérent avec "Document" / "Projet" /
    "Lisibilité" / "Surutilisés") ;
  - puis soit la ligne placeholder, soit une ligne par note (première ligne + `…` si tronquée par
    retour à la ligne, puis `ansi.Truncate` par largeur).
- Pas de changement au format du sidecar `.okashi-notes/<base>.json`, ni à `note`/`notesFile` — lecture
  seule côté inspecteur, aucune interaction (édition/suppression) n'est ajoutée dans ce panneau : pour
  éditer une note, l'utilisateur passe toujours par l'écran Notes (`n`).

## Hors périmètre

- Pas d'interaction (clic, édition, suppression) sur les notes depuis l'inspecteur — lecture seule,
  rappel visuel uniquement.
- Pas de troncature/plafond du nombre de notes affichées en v1 (voir ci-dessus).
- Pas de changement au scope des notes (toujours "chapitre" en v1, pas d'ancrage ligne/phrase — c'est le
  sujet d'un v2 déjà noté dans CLAUDE.md, non concerné par cette spec).
- Pas de nouveau raccourci clavier : la section est purement informative, affichée automatiquement
  quand l'inspecteur est ouvert sur l'onglet Mots.

## Tests

- Test unitaire pour `renderNotesSection` : liste vide → placeholder ; une note courte → une ligne ;
  une note multi-lignes → première ligne + `…` ; troncature par largeur.
- Test pour `inspectorModel.View()` (tab Mots) : avec des notes passées, la sortie contient le texte de
  la première note ; sans notes mais avec un fichier ouvert (`doc.words > 0`), la sortie contient le
  placeholder ; sans fichier ouvert (`doc.words == 0`), la section Notes est absente.
- Test d'intégration légère (style `smoke_test.go`) : ouvrir un fichier ayant une note sidecar,
  `ctrl+y` pour afficher l'inspecteur, confirmer que le texte de la note apparaît dans la `View()`
  rendue.
