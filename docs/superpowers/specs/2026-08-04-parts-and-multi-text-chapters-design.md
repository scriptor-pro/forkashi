# Parties et chapitres multi-textes

**Date** : 2026-08-04
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

⚠️ **HARD GATE — contrat partagé (CLAUDE.md §1)** : cette spec change la *forme* du schéma
`manifest.json` (v1 → v2). Avant implémentation, ce changement de forme doit être communiqué et
adopté dans le repo de l'app companion en parallèle — okashi ne peut pas déployer v2 seul sans
casser la lecture côté app tant qu'elle ne comprend que v1. Voir §"Compatibilité contrat partagé"
ci-dessous.

## Contexte

Aujourd'hui (manifest v1) :

- Un **manuscrit** = un dossier avec `manifest.json` ; `items[]` est une liste plate d'objets
  `{file, title}` — un item = un fichier `.md` = un chapitre. C'est l'unique granularité :
  1 fichier = 1 chapitre, pas de regroupement au-dessus, pas de subdivision en-dessous.
- `manuscriptView.chapters []chapterRef` (`manuscript.go`) est LE point de résolution unique
  consommé par la sidebar (`filelist.go`), le corkboard (`corkboard.go`), le pager (`pager.go`),
  l'export (`export.go` + 6 exporteurs), l'inspecteur (`inspector.go`) et le hub (`home.go`).

Cette spec redéfinit le chapitre et ajoute la Partie :

- **Chapitre** = un dossier contenant un ou plusieurs textes `.md`, réordonnables par l'auteur à
  tout moment. Le cas actuel (1 seul texte) reste le cas courant, pas une exception.
- **Partie** = un regroupement optionnel de chapitres consécutifs au sein d'un manuscrit. Modèle
  **mixte** (confirmé par la pratique de bibisco et Scrivener, qui autorisent tous deux des
  chapitres hors-partie coexistant avec des chapitres groupés en parties dans le même
  classeur/manuscrit) : un manuscrit peut avoir des chapitres directement sous sa racine
  (ex. un prologue) ET des parties contenant d'autres chapitres, simultanément.

## Modèle de données

### Manifest v2

```go
type manifestText struct {
    File  string `json:"file"`  // slug seul, ex. "scene-ouverture.md" — pas de préfixe numérique
    Title string `json:"title"`
}

type manifestChapter struct {
    Folder string         `json:"folder"` // slug seul, ex. "chapitre-un" — pas de préfixe numérique
    Title  string         `json:"title"`
    Texts  []manifestText `json:"texts"` // ordre = ordre d'affichage/export ; ≥1 en usage normal
}

// Un item est SOIT une Partie SOIT un chapitre nu (hors-partie) — mutuellement exclusif.
type manifestItem struct {
    Part     string            `json:"part,omitempty"`     // titre de la Partie ; "" si chapitre nu
    Chapters []manifestChapter `json:"chapters,omitempty"` // chapitres de la Partie
    Chapter  *manifestChapter  `json:"chapter,omitempty"`  // le chapitre, si hors-partie
}

type manifest struct {
    SchemaVersion int            `json:"schemaVersion"` // 2
    Title         string         `json:"title"`
    Items         []manifestItem `json:"items"`
}
```

`items[]` reste LA source d'ordre, à tous les niveaux : ordre des parties/chapitres-nus au niveau
racine, ordre des chapitres dans `Chapters[]`, ordre des textes dans `Texts[]`.

### Disque

- Un **chapitre** est un **dossier physique** : `<manuscrit>/chapitre-un/scene-ouverture.md`,
  `<manuscrit>/chapitre-un/scene-confrontation.md`.
- Une **Partie n'a pas de dossier** — elle n'existe que comme regroupement dans
  `manifest.json`. Créer une Partie ne touche jamais le disque, seulement le manifest.
- **Aucun préfixe numérique nulle part** (ni sur le nom du dossier-chapitre, ni sur les fichiers
  texte à l'intérieur) : le nom est un slug dérivé du titre au moment de la création, birth-stable
  ensuite (un retitre ne renomme jamais le dossier/fichier — cohérent avec `renameChapterTitle`
  existant). L'ordre réel vit exclusivement dans le manifest ; aucune opération de réorganisation
  ne nécessite donc de renommer quoi que ce soit sur le disque.

### `manuscriptView` (le point de résolution unique)

```go
type textRef struct {
    file  string // relatif au dossier du chapitre
    title string
    words int    // compteur de mots de CE texte (pour le sélecteur — voir §UX)
}

type chapterRef struct {
    folder string
    title  string
    texts  []textRef
}

type partRef struct {
    title    string      // "" pour la Partie synthétique qui porte les chapitres hors-partie
    chapters []chapterRef
}

type manuscriptView struct {
    source  manuscriptSource
    title   string
    parts   []partRef // TOUJOURS au moins un partRef ; un manuscrit sans Partie a un seul
                       // partRef{title: ""} contenant tous ses chapitres
    loose   []fileEntry
    warning string
}
```

Choix clé : **un manuscrit sans Partie est représenté comme "une Partie sans titre"** contenant
tous ses chapitres, plutôt que d'avoir deux formes différentes de `manuscriptView` (plate vs
groupée). Chaque site consommateur itère uniformément
`for _, p := range v.parts { for _, ch := range p.chapters { ... } }` et n'affiche l'en-tête de
Partie que si `p.title != ""` — pas de branchement conditionnel dupliqué à chaque site.

`resolveManuscript`/`manifestView` (`manuscript.go`) construisent cette forme à partir du manifest
v2 ; `chapterRef`/`textRef` restent la seule interface que les 7 sites consommateurs connaissent
(inchangée en substance, juste enrichie d'un niveau).

### Sites consommateurs impactés

| Site | Impact |
|---|---|
| `filelist.go` (sidebar) | Boucle `parts→chapters` ; en-tête non-sélectionnable (titre + somme des mots du groupe) quand `p.title != ""` ; chaque chapitre reste **une seule ligne repliée** (comme aujourd'hui), affichant la somme des mots de ses textes |
| `corkboard.go` | Même regroupement en en-têtes de section au-dessus des cartes chapitre |
| `pager.go` | Insère une ligne de titre de Partie dans le flux de lecture avant ses chapitres (cohérence avec le rendu export) |
| `export.go` + 6 exporteurs | Concatène `chapter.texts` dans l'ordre (saut de paragraphe entre deux textes d'un même chapitre) ; insère une rupture de section avec le titre de la Partie avant ses chapitres (même mécanique que la rupture déjà utilisée entre chapitres) |
| `inspector.go` | `proj.chapters` = compte total de chapitres, toutes parties/racine confondues (aplati) |
| `home.go` | Compteur de mots global = somme récursive sur tous les niveaux |
| `manuscript.go` (`isChapterOf`) | Cherche dans tous les chapitres de toutes les parties |

## Migration v1 → v2

**Politique : migration forcée, pas de coexistence.** Tous les manuscrits existants (v1) sont
migrés vers v2 ; okashi ne lit/écrit plus jamais la forme v1 après cette version.

**Déclenchement** : au premier `readManifest` d'un manuscrit dont `schemaVersion == 1`, okashi
affiche un écran de confirmation avant toute écriture disque :

```
Ce manuscrit utilise l'ancien format de chapitres.

  14 chapitres seront convertis en dossiers :
    01-un.md         → chapitre-un/chapitre-un.md
    02-deux.md        → chapitre-deux/chapitre-deux.md
    ...

  Chaque chapitre gagnera un titre par défaut si nécessaire
  (« Chapitre un », « Chapitre deux », …) — les titres déjà
  personnalisés dans le manifest sont conservés tels quels.

  [Entrée] convertir maintenant   [Échap] annuler et fermer
```

- **Slug du dossier** : dérivé du titre v1 actuel du chapitre (`items[].title`), pas du nom de
  fichier — cohérent avec le fait que le titre est déjà la donnée montrée à l'auteur.
- **Nom du fichier texte migré** : même slug que le dossier (le texte unique issu de la migration
  porte le nom du chapitre, pas un nom générique).
- **Titre par défaut "Chapitre un/deux/…"** : filet de sécurité si un titre v1 est vide (cas qui ne
  devrait normalement jamais se produire avec un manifest v1 valide, où `items[].title` est déjà
  rempli en pratique).
- **`Échap` (annuler)** : le manuscrit reste affiché tel quel pour cette session (même traitement
  que le cas "manifest illisible" déjà existant) ; okashi redemandera à la prochaine ouverture — pas
  de blocage permanent, pas de contournement silencieux.
- **Exécution atomique** : une passe par chapitre — crée le dossier, `os.Rename` le fichier dedans
  (même volume) — puis un **unique** `writeManifest` réécrit tout le manifest en v2 à la fin (pas
  une écriture par chapitre). Si un déplacement échoue en cours de route, la migration s'arrête et
  remonte l'erreur SANS avoir touché au manifest — jamais d'état où le manifest v2 référence des
  fichiers pas encore déplacés.

## UX de création et de réorganisation

### `ctrl+n` — picker à 4 choix

Chapitre / Texte / Ressource / Partie (au lieu des 2 actuels Chapitre/Ressource).

- **Texte** : option présente uniquement si un chapitre est actuellement ouvert dans l'éditeur
  (`m.currentFile` appartient à un chapitre) — absente du picker sinon, pas grisée. Le nouveau
  texte est ajouté à la fin de `texts[]` du chapitre ouvert, nommé par slug du titre saisi au
  prompt (même flux que la création de chapitre aujourd'hui).
- **Partie** : demande un titre, crée un `partRef` vide en fin de manifest (aucun chapitre dedans
  au départ) — l'auteur y déplace des chapitres ensuite via le structure mode.

### Structure mode (`s`) — étendu à 3 niveaux

Le staging existant (`m.structureItems`, mutable en mémoire, commit atomique en sortie derrière
confirmation) s'étend pour porter la hiérarchie Partie → Chapitre → Texte :

- **Créer une Partie** également possible depuis le structure mode (pas seulement `ctrl+n`).
- **Déplacer un chapitre** : mêmes touches déjà en place (`J`/`shift+down`/`alt+down` et
  `K`/`shift+up`/`alt+up`, plus `up/down`/`j/k` pour la navigation simple), traversant maintenant
  les frontières de Partie — déplacer un chapitre au-delà du dernier élément d'une Partie le fait
  sortir vers la Partie suivante (ou la racine).
- **Déplacer un texte vers un autre chapitre** : depuis la vue "textes de ce chapitre" (ouverte
  depuis un chapitre en structure mode), les mêmes touches de déplacement
  (`J`/`K`/`shift+↑↓`/`alt+↑↓`) font sortir un texte vers le chapitre voisin (précédent/suivant
  dans l'ordre du manuscrit) quand on dépasse le premier/dernier texte de la liste courante — même
  mécanique de "sortie vers le voisin" que le déplacement chapitre↔partie, appliquée un niveau plus
  bas. Pas de picker séparé pour choisir une destination arbitraire.

### Ouverture d'un chapitre depuis la sidebar

`⏎` sur un chapitre :
- **Un seul texte** : ouvre directement ce texte dans l'éditeur — comportement inchangé par
  rapport à aujourd'hui, aucune régression perçue pour le cas courant.
- **Plusieurs textes** : ouvre un petit sélecteur listant les textes du chapitre, **chacun avec son
  propre compteur de mots** affiché à côté de son titre ; `⏎` sur un texte du sélecteur l'ouvre dans
  l'éditeur.

## Chapitre et Partie vides

Traitement symétrique, aucune suppression automatique nulle part dans la hiérarchie — l'auteur
garde toujours le contrôle explicite :

- **Chapitre sans texte** (dernier texte supprimé/déplacé ailleurs) : le chapitre reste affiché
  (dossier vide toléré), jusqu'à suppression explicite par l'auteur.
- **Partie sans chapitre** (dernier chapitre déplacé ailleurs) : même traitement, la Partie reste
  affichée dans le manifest jusqu'à suppression explicite.
- **Avertissement à l'export** : sur l'écran de préparation d'export (le chooser en cases à
  cocher), si un chapitre ou une Partie inclus dans le périmètre est vide, un avertissement
  s'affiche avant le lancement effectif de l'export :
  `« Le chapitre XXX ne contient aucun texte — êtes-vous certain de l'inclure dans l'export ? »`
  (et l'équivalent pour une Partie sans chapitre). Pas d'interruption bloquante en plein export.

## Export

- **Concaténation multi-textes** : dans chacun des 6 exporteurs (`export_rtf.go`, `export_pdf.go`,
  `export_epub.go`, `export_docx.go`, `export_odt.go`, `export_titlepage.go`), le point qui fait
  aujourd'hui un `ReadFile` unique par chapitre devient une boucle sur `chapter.texts`, lecture et
  concaténation dans l'ordre. Séparateur entre deux textes d'un même chapitre : un simple saut de
  paragraphe — pas de rupture de page (les textes d'un même chapitre sont une continuité narrative,
  contrairement aux chapitres eux-mêmes qui commencent chacun sur une nouvelle
  page/section).
- **Titre de Partie** : éditable comme un titre de chapitre ; à l'export, une rupture de
  section/page avec ce titre précède les chapitres de la Partie — même mécanique que la rupture
  déjà utilisée entre chapitres, un niveau au-dessus. Une Partie sans titre (le partRef synthétique
  des chapitres hors-partie) n'insère aucune rupture supplémentaire.

## Promotion depuis l'outline

`ctrl+p`/`alt+↵` (promotion d'un beat en chapitre, `outline.go`) : le nouveau chapitre atterrit
dans la **dernière Partie du manuscrit si une Partie existe**, sinon à la racine (hors-partie) —
suppose que si l'auteur travaille déjà avec des Parties, un nouveau chapitre promu appartient
probablement à la Partie en cours d'écriture. Comportement inchangé (chapitre à la racine) pour un
manuscrit sans aucune Partie.

## Compatibilité contrat partagé (CLAUDE.md §1 — HARD GATE)

Le manifest v2 change la *forme* du schéma partagé avec l'app companion (nouveaux champs
`part`/`chapters`/`chapter`/`folder`/`texts`, disparition de la forme plate `items[].file`). Avant
implémentation :

- Confirmer et documenter ce changement de forme dans le repo de l'app companion en parallèle —
  l'app doit pouvoir lire (a minima) le manifest v2 avant qu'okashi ne commence à l'écrire sur des
  manuscrits partagés, sous peine de casser la lecture côté app.
- `readManifest` (`manifest.go`) doit refuser un `schemaVersion` non supporté exactement comme
  aujourd'hui (§4.1 du CLAUDE.md : jamais d'inférence de structure, affichage à plat en dernier
  recours) — donc un manifest v2 lu par une app companion pas encore mise à jour échouera
  proprement (pas de corruption), mais l'objectif reste de synchroniser les deux implémentations
  avant que v2 ne soit répandu dans des corpus partagés réels.

## Hors périmètre (v1 de cette fonctionnalité)

- Pas d'imbrication de Parties (une Partie ne contient que des chapitres, jamais d'autres Parties).
- Pas de repli/dépli des groupes de Partie dans la sidebar (toujours dépliée — cohérent avec "v1 =
  simple", pas de nouvel état UI à persister).
- Pas de picker de destination arbitraire pour déplacer un texte ou un chapitre — uniquement le
  glissement vers le voisin immédiat (§UX structure mode).
- Pas de fusion automatique de deux chapitres en un seul, ni de scission automatique d'un chapitre
  en plusieurs — ces opérations restent manuelles (créer un texte, déplacer, supprimer).

## Tests

- `manifest_test.go` : round-trip JSON du manifest v2 (Partie + chapitres + textes, chapitres
  hors-partie), `manifestInsert`/`Remove`/`Reorder` étendus aux 3 niveaux.
- `manuscript_test.go` : `resolveManuscript` sur un manifest v2 (parties + chapitres nus mêlés,
  Partie synthétique pour un manuscrit sans Partie), chapitre/partie vide toléré et affiché.
- Migration : test dédié qui charge un manifest v1 fixture, exécute la migration, vérifie le
  manifest v2 résultant ET l'arborescence disque (dossiers créés, fichiers déplacés), y compris le
  cas d'échec à mi-chemin (aucune écriture manifest si un `os.Rename` échoue).
- Sites consommateurs (`filelist_test.go`, `corkboard_test.go`, `pager_test.go`,
  `export_wiring_test.go`, `inspector_test.go`) : chacun avec un manuscrit v2 à parties mêlées,
  vérifie le regroupement visuel et les totaux de mots.
- Export : test par exporteur confirmant la concaténation multi-textes et la rupture de section
  Partie ; test de l'avertissement chapitre/partie vide sur l'écran de préparation.
