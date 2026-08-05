# Typographie française à la frappe

**Date** : 2026-08-05
**Auteur** : Baudouin (bvh@etik.com)
**Statut** : Approuvé

## Contexte

`smartQuote()` (`main.go`) transforme aujourd'hui les guillemets droits tapés au clavier en guillemets
anglais courbes : `'` → U+2018/U+2019 (simple), `"` → U+201C/U+201D (double), selon que le caractère
précédent indique une position ouvrante ou fermante. Aucune autre règle typographique française
(chevrons, espaces insécables, ponctuation répétée) n'est câblée dans l'éditeur.

Ce plan étend ce mécanisme à trois règles de typographie française, documentées dans le syllabus
*Règles et usages de typographie française* (G. Purnelle, ULiège, 2024) et dans le projet `typographeur`
(MIT, github.com/brunobord/typographeur — code non réutilisé tel quel, Python, orienté HTML ; seules ses
règles inspirent ce plan) :

1. Guillemets français (chevrons) avec espace insécable interne, à la place des guillemets courbes
   anglais.
2. Espace insécable devant `!`, `?`, `;` (fine, U+202F) et `:` (normale, U+00A0).
3. Plafond des points d'exclamation/interrogation répétés : jamais 2 consécutifs, jamais plus de 3.

Toutes les règles ci-dessous sont gouvernées par le réglage existant `smartQuotes`
(`OKASHI_SMARTQUOTES` / Properties) — **aucun nouveau réglage**. Cohérent avec la décision déjà prise
sur l'analyse POS (forkashi est french-first, pas de mode anglais à préserver) : `smartQuotes=true`
produit désormais un comportement français ; `false` laisse tout tel quel (guillemets droits, espaces
normales, pas de plafond).

## 1. Chevrons français

`smartQuote()` change de signature : `func smartQuote(prev rune, hasPrev bool, q rune) string`
(retournait un `rune`, retourne désormais une `string`, puisque le résultat pour `"` n'est plus un seul
caractère).

- `'` (apostrophe) : comportement inchangé — U+2018 (ouvrant) ou U+2019 (fermant), la même heuristique
  `opening` qu'aujourd'hui (`!hasPrev || prev` est espace/tab/newline/`(`/`[`/`{`).
- `"` (guillemet double) : produit `"« "` (chevron ouvrant + espace insécable normale) en position
  ouvrante, `" »"` (espace insécable normale + chevron fermant) en position fermante.

Le site d'appel (`main.go`, le bloc qui intercepte `km.Runes[0] == '\'' || km.Runes[0] == '"'`) insère
directement la chaîne retournée (plus besoin de `string(...)` autour de l'appel).

**Risque accepté** : aucune vérification de l'environnement au-delà du caractère précédent le curseur.
Un scénario d'édition non linéaire (l'utilisateur navigue en arrière et retape un guillemet près d'un
chevron existant) peut produire un double espace insécable visible — risque accepté, comportement
symétrique à `smartQuote()` aujourd'hui qui ne vérifie pas non plus l'environnement au-delà du caractère
précédent.

## 2. Espaces insécables devant `!` `?` `;` `:`

Nouvelle fonction, même point d'interception dans `Update()` que `smartQuote()` (une touche unique
tapée, `smartQuotes` actif) :

```go
var fineInsecableSigns = map[rune]bool{'!': true, '?': true, ';': true}

// punctuationSpacing reports whether the space immediately before the cursor should become
// non-breaking before inserting sign, and which non-breaking space to use. ok=false means no
// change (the preceding character is not an ordinary space — nothing to convert).
func punctuationSpacing(prev rune, hasPrev bool, sign rune) (insecable rune, ok bool) {
	if !hasPrev || prev != ' ' {
		return 0, false
	}
	if fineInsecableSigns[sign] {
		return ' ', true // espace fine insécable
	}
	if sign == ':' {
		return ' ', true // espace insécable normale
	}
	return 0, false
}
```

Sur `ok == true`, le site d'appel remplace le caractère précédent (`ReplaceCharBeforeCursor`, §4) par
`insecable` puis insère `sign`. Sur `ok == false`, insertion normale de `sign` — **aucune espace n'est
ajoutée si elle n'existait pas** (l'utilisateur pourrait taper une abréviation, un smiley, etc. ; on ne
transforme que ce qui est déjà une espace ordinaire).

## 3. Plafond des n-uples `!` / `?`

Règle indépendante des espaces insécables, mais partageant le même point d'interception pour `!` et `?`
(pas `;` ni `:`, qui n'ont pas cette notion de répétition emphatique en français). But : une séquence de
signes identiques ne doit jamais compter exactement 2, et jamais dépasser 3.

```go
// nupleInsert reports how many copies of sign to actually insert at the cursor, given count —
// the number of sign already present immediately before the cursor.
//   - count <= 0 (first of a new run): insert 1.
//   - count == 1: insert 2 (jump straight to 3 total — 2 is never a valid resting state in a
//     normal typing flow, since this same function already skipped it going from 0 to 1 to 3).
//   - count == 2: reachable only if the buffer already holds a non-conforming run (e.g. pasted
//     text, not produced by this function itself) — top up to 3 by inserting 1.
//   - count >= 3: insert 0 (the keystroke is swallowed, the sequence is already at its ceiling).
func nupleInsert(count int) int {
	switch {
	case count <= 0:
		return 1
	case count == 1:
		return 2
	case count == 2:
		return 1
	default: // count >= 3
		return 0
	}
}
```

`count` vient de `RunCountBeforeCursor(sign)` (§4) — le nombre d'occurrences consécutives de `sign`
immédiatement avant le curseur, avant que la frappe courante ne soit traitée.

**Interaction avec la règle d'espace insécable (§2)** : les deux règles s'exécutent toujours, dans cet
ordre, à la frappe de `!` ou `?` — d'abord `punctuationSpacing` (no-op si le caractère précédent n'est
pas une espace, ce qui est le cas naturel dès le 2ᵉ signe d'une séquence), puis `nupleInsert`. Pas
d'aiguillage explicite entre « premier signe de la séquence » et « signe suivant » : chaque étape
s'applique ou ne s'applique pas selon ce qu'elle trouve, indépendamment de l'autre.

## 4. Extension de `internal/textarea`

Deux nouvelles méthodes publiques sur `Model`, à côté de `CharBeforeCursor` :

```go
// ReplaceCharBeforeCursor deletes the rune immediately left of the cursor and inserts r in its
// place. A no-op at the start of a line (mirrors CharBeforeCursor's own boundary behavior).
func (m *Model) ReplaceCharBeforeCursor(r rune)

// RunCountBeforeCursor returns how many consecutive copies of r sit immediately before the
// cursor. 0 if the cursor is at the start of a line or the preceding rune isn't r.
func (m Model) RunCountBeforeCursor(r rune) int
```

Ces deux méthodes touchent le cœur de l'éditeur vendorisé (invariant CLAUDE.md : *« Editor-core changes
go in `internal/textarea` »*) — pas d'accès direct aux champs privés (`m.value`, `m.row`, `m.col`)
depuis `main.go`.

## Hors périmètre

- La réduction des n-uples sur du texte **déjà existant** (contenu chargé depuis un fichier, ou collé)
  n'est pas traitée — seule la frappe interactive caractère par caractère est couverte par ce plan. Un
  fichier `.md` externe contenant déjà `!!` ou `!!!!!` n'est jamais corrigé automatiquement à l'ouverture
  ni à la sauvegarde.
- Le point-virgule `;` n'a pas de règle de plafond (pas d'usage français de `;;;` répété).
- Aucun changement à l'export (RTF/PDF) — ce plan couvre uniquement la frappe live dans l'éditeur.
  Le texte déjà présent dans un fichier ouvert avant l'activation de `smartQuotes` (ou tapé avant ce
  plan) garde ses guillemets droits/espaces normales jusqu'à retype manuel.
- Aucun nouveau réglage : tout est gouverné par `smartQuotes` existant.
