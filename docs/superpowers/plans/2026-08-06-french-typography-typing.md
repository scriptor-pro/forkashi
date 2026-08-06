# Typographie française à la frappe — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Étendre le mécanisme de frappe intelligente (`smartQuotes`) pour produire, en français, des
chevrons « » avec espace insécable interne, des espaces insécables devant `!` `?` `;` `:`, et un
plafond sur les n-uples `!`/`?` répétés — le tout gouverné par le réglage `smartQuotes` existant.

**Architecture:** Deux nouvelles méthodes de mutation/lecture ciblées sur `internal/textarea.Model`
(`ReplaceCharBeforeCursor`, `RunCountBeforeCursor`), suivant le style déjà en place (`CharBeforeCursor`,
`deleteBeforeCursor`, `transposeLeft` — receiver `*Model` pour les mutations, `Model` par valeur pour
les lectures, indexation directe de `m.value[m.row]`, pas d'allocation). Côté `main.go`, `smartQuote()`
change de signature pour retourner une `string`, et un nouveau bloc d'interception clavier — copié sur
le pattern exact du bloc smart-quotes existant (`km.Type == tea.KeyRunes`, une seule rune, court-circuit
avec `return m, nil`) — capte `!` `?` `;` `:` pour appliquer l'espace insécable puis, pour `!`/`?`
uniquement, le plafond de répétition.

**Tech Stack:** Go 1.25, Bubble Tea (`tea.KeyMsg`/`tea.KeyRunes`), `internal/textarea` (fork vendored de
`bubbles/textarea`), tests `go test` standard (table-driven + intégration via `Update()`).

## Global Constraints

- Tout est gouverné par le réglage existant `smartQuotes` (`OKASHI_SMARTQUOTES` / Properties) — **aucun
  nouveau réglage**.
- Espace insécable fine = U+202F (devant `!` `?` `;`) ; espace insécable normale = U+00A0 (devant `:`,
  et interne aux chevrons `« »`).
- L'espace insécable devant `!?;:` n'est produite **que si une espace ordinaire existe déjà** avant le
  signe — jamais ajoutée d'office (pas de transformation d'une abréviation, d'un smiley, etc.).
- Plafond des n-uples `!`/`?` : jamais exactement 2 signes consécutifs, jamais plus de 3.
- Editor-core changes vont dans `internal/textarea`, jamais d'accès direct depuis `main.go` aux champs
  privés (`value`, `row`, `col`) de `textarea.Model`.
- Hors périmètre (ne pas implémenter) : correction rétroactive du texte déjà existant (fichier chargé ou
  collé), plafond pour `;`, tout changement à l'export RTF/PDF, tout nouveau réglage.
- Fichiers de référence pour le style de code et de tests :
  - `internal/textarea/textarea.go:1783-1790` (`CharBeforeCursor`, modèle de lecture)
  - `internal/textarea/textarea.go:723-726` et `740-751` (`deleteBeforeCursor`, `transposeLeft`, modèles
    de mutation ciblée)
  - `internal/textarea/editing_test.go:32-57` (`TestLineHelpers`, style de test interne au package)
  - `main.go:241-260` (`smartQuote`, à modifier)
  - `main.go:1731-1740` (site d'appel smart-quotes, modèle exact du nouveau bloc)
  - `smoke_test.go:466-514` (3 niveaux de test : unitaire pur, résolution de réglage, intégration
    `Update()`)

---

## Task 1: `ReplaceCharBeforeCursor` et `RunCountBeforeCursor` sur `internal/textarea.Model`

**Files:**
- Modify: `internal/textarea/textarea.go` (ajouter après `CharBeforeCursor`, ligne 1790)
- Test: `internal/textarea/editing_test.go` (ajouter après `TestLineHelpers`, ligne 57)

**Interfaces:**
- Produces:
  - `func (m *Model) ReplaceCharBeforeCursor(r rune)` — remplace en place la rune immédiatement à
    gauche du curseur par `r`. No-op en début de ligne (`m.col == 0`).
  - `func (m Model) RunCountBeforeCursor(r rune) int` — nombre d'occurrences consécutives de `r`
    immédiatement avant le curseur (0 si le curseur est en début de ligne ou si la rune précédente
    n'est pas `r`).

- [ ] **Step 1: Write the failing tests**

Ajouter dans `internal/textarea/editing_test.go`, à la suite de `TestLineHelpers` :

```go
func TestReplaceCharBeforeCursor(t *testing.T) {
	m := New()
	m.SetValue("a!")
	m.SetCursor(2)
	m.ReplaceCharBeforeCursor(' ')
	if got := m.Value(); got != "a " {
		t.Fatalf("after ReplaceCharBeforeCursor: %q, want %q", got, "a ")
	}
	if m.col != 2 {
		t.Fatalf("cursor col = %d, want 2 (unchanged)", m.col)
	}

	m.SetCursor(0)
	m.ReplaceCharBeforeCursor('x') // no-op at start of line
	if got := m.Value(); got != "a " {
		t.Fatalf("ReplaceCharBeforeCursor at col 0 should be no-op, got %q", got)
	}
}

func TestRunCountBeforeCursor(t *testing.T) {
	m := New()
	m.SetValue("a!!!b")
	m.SetCursor(4) // just before 'b', after "a!!!"
	if n := m.RunCountBeforeCursor('!'); n != 3 {
		t.Fatalf("RunCountBeforeCursor('!') = %d, want 3", n)
	}
	if n := m.RunCountBeforeCursor('?'); n != 0 {
		t.Fatalf("RunCountBeforeCursor('?') = %d, want 0 (wrong rune)", n)
	}

	m.SetCursor(0)
	if n := m.RunCountBeforeCursor('!'); n != 0 {
		t.Fatalf("RunCountBeforeCursor at col 0 = %d, want 0", n)
	}

	m.SetCursor(1) // just after "a"
	if n := m.RunCountBeforeCursor('!'); n != 0 {
		t.Fatalf("RunCountBeforeCursor('!') at col 1 = %d, want 0", n)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/textarea/... -run 'TestReplaceCharBeforeCursor|TestRunCountBeforeCursor' -v`
Expected: FAIL — `m.ReplaceCharBeforeCursor` et `m.RunCountBeforeCursor` undefined (méthodes non
implémentées).

- [ ] **Step 3: Write minimal implementation**

Ajouter dans `internal/textarea/textarea.go`, immédiatement après `CharBeforeCursor` (ligne 1790) :

```go
// ReplaceCharBeforeCursor replaces the rune immediately left of the cursor with r.
// A no-op at the start of a line (mirrors CharBeforeCursor's own boundary behavior).
func (m *Model) ReplaceCharBeforeCursor(r rune) {
	if m.col == 0 {
		return
	}
	m.value[m.row][m.col-1] = r
}

// RunCountBeforeCursor returns how many consecutive copies of r sit immediately before the
// cursor. 0 if the cursor is at the start of a line or the preceding rune isn't r.
func (m Model) RunCountBeforeCursor(r rune) int {
	n := 0
	for i := m.col - 1; i >= 0 && m.value[m.row][i] == r; i-- {
		n++
	}
	return n
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/textarea/... -run 'TestReplaceCharBeforeCursor|TestRunCountBeforeCursor' -v`
Expected: PASS

- [ ] **Step 5: Run the full textarea package test suite (regression check)**

Run: `go test ./internal/textarea/...`
Expected: PASS (aucune régression sur les tests existants)

- [ ] **Step 6: Commit**

```bash
git add internal/textarea/textarea.go internal/textarea/editing_test.go
git commit -m "textarea: ajoute ReplaceCharBeforeCursor et RunCountBeforeCursor"
```

---

## Task 2: Chevrons français dans `smartQuote()`

**Files:**
- Modify: `main.go:241-260` (`smartQuote`), `main.go:1731-1740` (site d'appel)
- Test: `smoke_test.go` (modifier `TestSmartQuoteHelper`, ajouter un cas dans `TestEditorSmartQuoteInsert`
  ou un nouveau test dédié)

**Interfaces:**
- Consumes: rien de nouveau (utilise `m.editor.CharBeforeCursor()` et `m.editor.InsertString(string)`,
  déjà en place).
- Produces: `func smartQuote(prev rune, hasPrev bool, q rune) string` (signature changée : retournait
  `rune`, retourne désormais `string`). Les tâches suivantes n'en dépendent pas directement (elles
  ajoutent un bloc séparé), mais Task 2 doit laisser `main.go` compilable et tous les tests existants
  au vert avant de passer à la suite.

- [ ] **Step 1: Update the failing unit test for the new chevron behavior**

Dans `smoke_test.go`, remplacer `TestSmartQuoteHelper` (lignes 466-484) par :

```go
func TestSmartQuoteHelper(t *testing.T) {
	nbsp := string(rune(0x00A0))
	cases := []struct {
		prev    rune
		hasPrev bool
		q       rune
		want    string
	}{
		{0, false, '\'', string(rune(0x2018))},  // start of line → opening '
		{'n', true, '\'', string(rune(0x2019))}, // contraction don't → closing '
		{'(', true, '\'', string(rune(0x2018))}, // after ( → opening
		{0, false, '"', "«" + nbsp},              // start of line → opening chevron + nbsp
		{' ', true, '"', "«" + nbsp},              // after space → opening chevron + nbsp
		{'d', true, '"', nbsp + "»"},              // after letter → closing chevron
	}
	for _, c := range cases {
		if got := smartQuote(c.prev, c.hasPrev, c.q); got != c.want {
			t.Fatalf("smartQuote(%q,%v,%q) = %q, want %q", c.prev, c.hasPrev, c.q, got, c.want)
		}
	}
}
```

Modifier aussi `TestEditorSmartQuoteInsert` (smoke_test.go:497-514) pour refléter le nouveau résultat
attendu :

```go
func TestEditorSmartQuoteInsert(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true
	m.editor.SetValue("")
	m.editor.SetCursor(0)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'"'}})
	m = nm.(model)
	expected := "«" + string(rune(0x00A0)) // opening chevron + non-breaking space
	if m.editor.Value() != expected {
		t.Fatalf("typing \" at start should insert an opening chevron + nbsp, got %q", m.editor.Value())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestSmartQuoteHelper|TestEditorSmartQuoteInsert' -v`
Expected: FAIL — `smartQuote` retourne encore `rune` (erreur de compilation sur la comparaison
`string`/`rune`), ou échec d'assertion une fois le type ajusté.

- [ ] **Step 3: Update `smartQuote` to return chevrons**

Remplacer `main.go:241-260` par :

```go
// smartQuote returns the curly form of a straight quote, as a string (the French
// double-quote form is multi-character: chevron + non-breaking space). It's an opening
// quote at the start of a line or after whitespace / an opening bracket; otherwise
// closing (which also yields the right apostrophe in contractions).
func smartQuote(prev rune, hasPrev bool, q rune) string {
	opening := !hasPrev || prev == ' ' || prev == '\t' || prev == '\n' ||
		prev == '(' || prev == '[' || prev == '{'
	nbsp := string(rune(0x00A0)) // espace insécable normale
	switch q {
	case '\'':
		if opening {
			return string(rune(0x2018)) // U+2018 left single quote
		}
		return string(rune(0x2019)) // U+2019 right single quote
	case '"':
		if opening {
			return "«" + nbsp
		}
		return nbsp + "»"
	}
	return string(q)
}
```

- [ ] **Step 4: Update the call site to drop the now-redundant `string(...)` wrap**

Dans `main.go:1735`, remplacer :

```go
			m.editor.InsertString(string(smartQuote(prev, hasPrev, km.Runes[0])))
```

par :

```go
			m.editor.InsertString(smartQuote(prev, hasPrev, km.Runes[0]))
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestSmartQuoteHelper|TestEditorSmartQuoteInsert' -v`
Expected: PASS

- [ ] **Step 6: Run the full test suite (regression check)**

Run: `go build ./... && go test ./...`
Expected: PASS — en particulier vérifier qu'aucun autre appelant de `smartQuote` ne dépendait du type
`rune` (recherche faite pendant l'exploration : le seul appelant est `main.go:1735`).

- [ ] **Step 7: Commit**

```bash
git add main.go smoke_test.go
git commit -m "typographie: chevrons français « » avec espace insécable pour les guillemets doubles"
```

---

## Task 3: Espaces insécables devant `!` `?` `;` `:`

**Files:**
- Modify: `main.go` (nouvelle fonction près de `smartQuote`, ~ligne 260 ; nouveau bloc d'interception
  dans `Update()`, entre `main.go:1740` et `main.go:1741`)
- Test: `smoke_test.go` (nouveaux tests)

**Interfaces:**
- Consumes: `m.editor.CharBeforeCursor() (rune, bool)`, `m.editor.ReplaceCharBeforeCursor(rune)`
  (Task 1), `m.editor.InsertString(string)`/`InsertRune(rune)` (existant, `internal/textarea/textarea.go:361-370`).
- Produces:
  - `var fineInsecableSigns = map[rune]bool{'!': true, '?': true, ';': true}`
  - `func punctuationSpacing(prev rune, hasPrev bool, sign rune) (insecable rune, ok bool)` — utilisé
    par Task 4 pour la logique combinée des n-uples.

- [ ] **Step 1: Write the failing unit test for `punctuationSpacing`**

Ajouter dans `smoke_test.go`, à la suite des tests smart-quote :

```go
func TestPunctuationSpacing(t *testing.T) {
	cases := []struct {
		prev        rune
		hasPrev     bool
		sign        rune
		wantInsec   rune
		wantOK      bool
	}{
		{' ', true, '!', rune(0x202F), true},  // ordinary space before ! → fine nbsp
		{' ', true, '?', rune(0x202F), true},  // ordinary space before ? → fine nbsp
		{' ', true, ';', rune(0x202F), true},  // ordinary space before ; → fine nbsp
		{' ', true, ':', rune(0x00A0), true},  // ordinary space before : → normal nbsp
		{'a', true, '!', 0, false},            // no preceding space → no-op
		{0, false, '!', 0, false},              // start of line → no-op
		{rune(0x202F), true, '!', 0, false},    // already a fine nbsp, not an ordinary space → no-op
	}
	for _, c := range cases {
		gotInsec, gotOK := punctuationSpacing(c.prev, c.hasPrev, c.sign)
		if gotOK != c.wantOK || (gotOK && gotInsec != c.wantInsec) {
			t.Fatalf("punctuationSpacing(%q,%v,%q) = %q,%v want %q,%v",
				c.prev, c.hasPrev, c.sign, gotInsec, gotOK, c.wantInsec, c.wantOK)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestPunctuationSpacing -v`
Expected: FAIL — `punctuationSpacing` undefined.

- [ ] **Step 3: Implement `punctuationSpacing` next to `smartQuote`**

Ajouter dans `main.go`, après la fonction `smartQuote` (donc après ce qui est désormais autour de la
ligne 265) :

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
		return rune(0x202F), true // espace fine insécable
	}
	if sign == ':' {
		return rune(0x00A0), true // espace insécable normale
	}
	return 0, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestPunctuationSpacing -v`
Expected: PASS

- [ ] **Step 5: Write the failing integration test via `Update()`**

Ajouter dans `smoke_test.go` :

```go
func TestEditorPunctuationSpacingInsert(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true
	m.editor.SetValue("a ")
	m.editor.SetCursor(2)

	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	m = nm.(model)
	expected := "a" + string(rune(0x202F)) + "!"
	if m.editor.Value() != expected {
		t.Fatalf("typing ! after 'a ' should convert the space to a fine nbsp, got %q", m.editor.Value())
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test . -run TestEditorPunctuationSpacingInsert -v`
Expected: FAIL — le comportement n'est pas encore câblé dans `Update()` ; `!` s'insère normalement sans
conversion de l'espace.

- [ ] **Step 7: Wire the new block into `Update()`**

Dans `main.go`, insérer un nouveau bloc juste après la fermeture du bloc smart-quotes (après
`main.go:1740`, avant `main.go:1741` — repérer `before := m.editor.Value()`) :

```go
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '!' || km.Runes[0] == '?' || km.Runes[0] == ';' || km.Runes[0] == ':') {
			prev, hasPrev := m.editor.CharBeforeCursor()
			if insecable, ok := punctuationSpacing(prev, hasPrev, km.Runes[0]); ok {
				m.editor.ReplaceCharBeforeCursor(insecable)
			}
			m.editor.InsertRune(km.Runes[0])
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings()
			return m, nil
		}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test . -run TestEditorPunctuationSpacingInsert -v`
Expected: PASS

- [ ] **Step 9: Run the full test suite (regression check)**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add main.go smoke_test.go
git commit -m "typographie: espace insécable devant ! ? ; (fine) et : (normale)"
```

---

## Task 4: Plafond des n-uples `!` / `?`

**Files:**
- Modify: `main.go` (nouvelle fonction `nupleInsert`, bloc `Update()` de Task 3 étendu pour `!`/`?`)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `punctuationSpacing` (Task 3), `m.editor.RunCountBeforeCursor(rune) int` (Task 1),
  `m.editor.ReplaceCharBeforeCursor(rune)` (Task 1), `m.editor.InsertRune(rune)` (existant).
- Produces: `func nupleInsert(count int) int`.

**Note d'implémentation (règle du plan, §3)** : les deux règles (espace insécable et plafond) s'exécutent
toujours dans cet ordre, sans aiguillage explicite entre elles — chacune s'applique ou non selon ce
qu'elle trouve. Pour `!`/`?`, le bloc de Task 3 s'étend donc pour appeler `nupleInsert` **après** avoir
géré l'espace insécable, et insérer `nupleInsert(count)` copies du signe au lieu d'insérer
inconditionnellement 1 copie.

- [ ] **Step 1: Write the failing unit test for `nupleInsert`**

Ajouter dans `smoke_test.go` :

```go
func TestNupleInsert(t *testing.T) {
	cases := []struct {
		count int
		want  int
	}{
		{0, 1},  // first of a new run
		{-1, 1}, // defensive: treated like 0
		{1, 2},  // jump straight to 3 total (2 is never a valid resting state)
		{2, 1},  // top up a non-conforming run (e.g. pasted) to 3
		{3, 0},  // already at ceiling — swallow the keystroke
		{4, 0},  // above ceiling (non-conforming) — still swallow
	}
	for _, c := range cases {
		if got := nupleInsert(c.count); got != c.want {
			t.Fatalf("nupleInsert(%d) = %d, want %d", c.count, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test . -run TestNupleInsert -v`
Expected: FAIL — `nupleInsert` undefined.

- [ ] **Step 3: Implement `nupleInsert` next to `punctuationSpacing`**

Ajouter dans `main.go`, après `punctuationSpacing` :

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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test . -run TestNupleInsert -v`
Expected: PASS

- [ ] **Step 5: Write the failing integration tests via `Update()`**

Ajouter dans `smoke_test.go` :

```go
func TestEditorNupleCeilingInsert(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true

	// Typing "!" once → 1 mark.
	m.editor.SetValue("a")
	m.editor.SetCursor(1)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	m = nm.(model)
	if m.editor.Value() != "a!" {
		t.Fatalf("1st ! : got %q, want %q", m.editor.Value(), "a!")
	}

	// Typing "!" again right after → jumps straight to 3 (never rests at 2).
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	m = nm.(model)
	if m.editor.Value() != "a!!!" {
		t.Fatalf("2nd ! : got %q, want %q (should jump to 3, never rest at 2)", m.editor.Value(), "a!!!")
	}

	// A 3rd keystroke at the ceiling is swallowed — no change.
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	m = nm.(model)
	if m.editor.Value() != "a!!!" {
		t.Fatalf("3rd ! : got %q, want %q (ceiling reached, keystroke swallowed)", m.editor.Value(), "a!!!")
	}
}

func TestEditorNupleTopUpNonConforming(t *testing.T) {
	m := initialModel()
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)
	m.screen = screenWriting
	m.focus = focusEditor
	m.editor.Focus()
	m.smartQuotes = true

	// Simulate a non-conforming pre-existing run (e.g. pasted text): "a??" then type "?".
	m.editor.SetValue("a??")
	m.editor.SetCursor(3)
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = nm.(model)
	if m.editor.Value() != "a???" {
		t.Fatalf("top-up from non-conforming run: got %q, want %q", m.editor.Value(), "a???")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test . -run 'TestEditorNupleCeilingInsert|TestEditorNupleTopUpNonConforming' -v`
Expected: FAIL — le plafond n'est pas encore câblé ; `!`/`?` s'insèrent un par un sans plafond.

- [ ] **Step 7: Extend the `Update()` block for `!`/`?` to apply the ceiling**

Dans `main.go`, remplacer le bloc ajouté en Task 3 Step 7 par la version étendue suivante (elle
distingue `!`/`?` — soumis au plafond — de `;`/`:` — insertion simple inchangée) :

```go
		if km, ok := msg.(tea.KeyMsg); ok && m.smartQuotes &&
			km.Type == tea.KeyRunes && len(km.Runes) == 1 &&
			(km.Runes[0] == '!' || km.Runes[0] == '?' || km.Runes[0] == ';' || km.Runes[0] == ':') {
			sign := km.Runes[0]
			prev, hasPrev := m.editor.CharBeforeCursor()
			if insecable, ok := punctuationSpacing(prev, hasPrev, sign); ok {
				m.editor.ReplaceCharBeforeCursor(insecable)
			}
			if sign == '!' || sign == '?' {
				count := m.editor.RunCountBeforeCursor(sign)
				for i := 0; i < nupleInsert(count); i++ {
					m.editor.InsertRune(sign)
				}
			} else {
				m.editor.InsertRune(sign)
			}
			m.dirty = true
			m.lastEditAt = time.Now()
			m.invalidateAppleFindings()
			return m, nil
		}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test . -run 'TestEditorNupleCeilingInsert|TestEditorNupleTopUpNonConforming' -v`
Expected: PASS

- [ ] **Step 9: Run the full test suite (regression check)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS — vérifier en particulier que `TestEditorPunctuationSpacingInsert` (Task 3) passe
toujours (le bloc a été remplacé, pas juste étendu : relire le diff pour confirmer que le comportement
`;`/`:` de Task 3 est préservé à l'identique).

- [ ] **Step 10: Commit**

```bash
git add main.go smoke_test.go
git commit -m "typographie: plafonne les n-uples ! et ? (jamais 2, jamais plus de 3)"
```

---

## Task 5: Vérification manuelle dans l'éditeur réel

**Files:** aucun changement de code — vérification manuelle uniquement.

- [ ] **Step 1: Lancer okashi sur un projet de test**

Run: `go run .` (depuis la racine du repo), ouvrir ou créer un petit projet de test, entrer l'éditeur
d'un chapitre.

- [ ] **Step 2: Vérifier les chevrons**

Taper `"` en début de ligne puis en fin de mot : confirmer visuellement `« ` (chevron + espace) puis
` »` (espace + chevron), et que le curseur se comporte normalement (pas de saut visible, pas de
caractère fantôme).

- [ ] **Step 3: Vérifier les espaces insécables**

Taper `Bonjour !`, `Ça va ?`, `Un ; deux`, `Titre :` — confirmer qu'un espace tapé juste avant `!?;:`
devient insécable (visuellement identique à une espace normale dans la plupart des terminaux, mais
confirmer qu'aucun retour à la ligne ne coupe entre l'espace et le signe si on réduit la largeur du
terminal). Taper aussi `:)`  (aucun espace avant) pour confirmer qu'aucune espace n'est insérée
artificiellement.

- [ ] **Step 4: Vérifier le plafond des n-uples**

Taper `!` trois fois rapidement de suite : confirmer la séquence `!` → `!!!` → (3e frappe avalée, reste
`!!!`) — jamais d'état à 2. Répéter pour `?`.

- [ ] **Step 5: Vérifier que `smartQuotes=false` désactive tout**

Couper le réglage (`OKASHI_SMARTQUOTES=off go run .`, ou via Properties), répéter les frappes ci-dessus :
confirmer guillemets droits, espaces ordinaires, aucun plafond — comportement strictement identique à
avant ce plan.

- [ ] **Step 6: Confirmer avec l'utilisateur**

Rapporter le résultat de la vérification manuelle à l'utilisateur avant de considérer la fonctionnalité
terminée.

---

## Hors périmètre (rappel, ne pas implémenter dans ce plan)

- Correction rétroactive des n-uples sur du texte déjà existant (chargé ou collé).
- Plafond de répétition pour `;`.
- Tout changement à l'export RTF/PDF.
- Tout nouveau réglage — tout reste gouverné par `smartQuotes` existant.
