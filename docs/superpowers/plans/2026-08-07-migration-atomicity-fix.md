# Corriger l'atomicité et la visibilité d'erreur de la migration v1→v2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Empêcher qu'une migration v1→v2 échouée à mi-chemin laisse le disque dans un état hybride
silencieux (certains fichiers déplacés, manifest jamais réécrit), et informer l'utilisateur quand
une migration échoue au lieu d'avaler l'erreur.

**Architecture:** Deux changements indépendants dans le chemin de migration existant. (1)
`migrateV1ToV2` (`migration.go`) valide que tous les fichiers source existent AVANT de déplacer quoi
que ce soit — une passe `os.Stat` sur `plan.moves` avant la boucle de `os.Rename` existante. (2)
`confirmMigration` (`main.go`) capture l'erreur de retour de `migrateV1ToV2` (actuellement avalée
avec `_ =`) et l'écrit dans `m.status`, la barre de statut déjà utilisée pour les erreurs
transitoires ailleurs dans ce fichier.

**Tech Stack:** Go 1.25, tests `go test` standard (table-driven, `t.TempDir()`, style déjà en place
dans `migration_test.go` et `main_test.go`).

## Global Constraints

- Pas de migration partielle : soit tous les items migrent, soit aucun fichier ne bouge et le
  manifest reste en v1, inchangé (comportement "tout ou rien" confirmé par l'utilisateur).
- La validation en amont ne doit RIEN écrire sur disque si elle échoue — ni déplacement de fichier,
  ni écriture de manifest.
- L'erreur affichée à l'utilisateur passe exclusivement par `m.status` — jamais de popup, jamais de
  changement à `migrationConfirmView` ou à un layout à géométrie fixe.
- Pas de nouveau mécanisme de retry automatique en session ; l'utilisateur corrige manuellement
  (renommer le fichier ou éditer le manifest) puis relance okashi.
- Le comportement de navigation de `confirmMigration` reste identique à aujourd'hui dans tous les
  cas : `m.migrationPending` repasse à `nil`, l'écran retourne à `screenWriting`.
- Design de référence : `docs/superpowers/specs/2026-08-07-migration-atomicity-fix-design.md`.

---

## Task 1: Validation en amont dans `migrateV1ToV2`

**Files:**
- Modify: `migration.go:150-169` (`migrateV1ToV2`)
- Modify: `migration_test.go:167-188` (`TestMigrateV1ToV2FailsAtomicallyIfAMoveFails` — corriger
  cette couverture existante, qui ne vérifiait que le manifest et pas les fichiers)
- Test: `migration_test.go` (nouveau test)

**Interfaces:**
- Consumes: `migrationStep` (`migration.go:80-83`, champs `moves []migrationMove` et `out
  manifest`), `migrationMove` (`migration.go:72-76`, champs `fromFile`, `toFolder`, `toFile`) —
  types déjà définis, inchangés par cette tâche.
- Produces: `migrateV1ToV2(dir string, v1 manifestV1) error` — signature inchangée ; seul le
  comportement interne change (validation ajoutée avant la boucle de déplacement existante).

**Note d'implémentation** : le test existant `TestMigrateV1ToV2FailsAtomicallyIfAMoveFails`
(`migration_test.go:167-188`) ne vérifie aujourd'hui QUE que le manifest reste en v1 après un échec
— il ne vérifie PAS que le fichier `01-un.md` (dont le déplacement aurait réussi, puisqu'il vient
avant l'item manquant dans `items[]`) reste bien à sa place d'origine. C'est exactement le trou qui
a laissé passer le bug réel (3 fichiers déjà déplacés, manifest jamais réécrit, migration rejouée en
boucle). Ce test doit être étendu pour vérifier aussi les fichiers, pas seulement le manifest.

- [ ] **Step 1: Étendre le test existant pour vérifier aussi les fichiers (pas seulement le manifest)**

Dans `migration_test.go`, remplacer `TestMigrateV1ToV2FailsAtomicallyIfAMoveFails` (lignes 167-188)
par :

```go
func TestMigrateV1ToV2FailsAtomicallyIfAMoveFails(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	// "missing.md" is declared but never created on disk — its rename will fail.
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[
			{"file":"01-un.md","title":"Un"},{"file":"missing.md","title":"Deux"}]}`), 0o644)

	v1, _, _ := readManifestV1(dir)
	err := migrateV1ToV2(dir, v1)
	if err == nil {
		t.Fatal("migration must fail when a listed file is missing")
	}

	// The manifest must remain the ORIGINAL v1 file — never partially rewritten.
	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("manifest must still be the original v1 content after a failed migration, got: %s", data)
	}

	// "01-un.md" must NOT have been moved — its rename would have succeeded (it
	// exists on disk and comes before the missing item in items[]), but validating
	// ALL sources up front before moving anything means nothing touches disk when
	// any one item is missing. This reproduces the real bug: a v1 manifest whose
	// later item references a typo'd/renamed filename left earlier items already
	// moved while the manifest stayed v1 — migration then silently retried forever.
	if _, err := os.Stat(filepath.Join(dir, "01-un.md")); err != nil {
		t.Fatalf("01-un.md must still exist at the manuscript root (no partial move), got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "un")); !os.IsNotExist(err) {
		t.Fatal("un/ folder must not have been created — nothing should move when validation fails")
	}
}
```

- [ ] **Step 2: Écrire le nouveau test de validation en amont**

Ajouter à la suite dans `migration_test.go` :

```go
// TestMigrateV1ToV2ValidatesAllSourcesBeforeMovingAny reproduces the exact
// real-world failure mode: a v1 manifest with 4 items where the 4th references
// a typo'd filename ("Bureau-ONU.mk" vs the real "Bureau-ONU.md" on disk). Before
// the fix, items 1-3 would already be moved to their new folders by the time item
// 4's os.Stat failed, leaving a hybrid disk state and a manifest stuck on v1
// forever (needsMigration keeps re-triggering, migration keeps re-failing at the
// same spot, silently). After the fix, validation happens before any move, so
// nothing on disk changes when item 4 is missing.
func TestMigrateV1ToV2ValidatesAllSourcesBeforeMovingAny(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Annonce.md"), []byte("annonce"), 0o644)
	os.WriteFile(filepath.Join(dir, "Attribution.md"), []byte("attribution"), 0o644)
	os.WriteFile(filepath.Join(dir, "Bureau-ONU.md"), []byte("bureau"), 0o644)
	// Manifest references "Bureau-ONU.mk" (typo) — the real file is "Bureau-ONU.md".
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[
			{"file":"Annonce.md","title":"Annonce"},
			{"file":"Attribution.md","title":"Attribution"},
			{"file":"Bureau-ONU.mk","title":"Bureau ONU"}]}`), 0o644)

	v1, _, _ := readManifestV1(dir)
	err := migrateV1ToV2(dir, v1)
	if err == nil {
		t.Fatal("migration must fail when any listed file is missing")
	}

	for _, want := range []string{"Annonce.md", "Attribution.md", "Bureau-ONU.md"} {
		if _, statErr := os.Stat(filepath.Join(dir, want)); statErr != nil {
			t.Fatalf("%s must still exist at the manuscript root (no partial move), got: %v", want, statErr)
		}
	}
	for _, unwanted := range []string{"annonce", "attribution", "bureau-onu"} {
		if _, statErr := os.Stat(filepath.Join(dir, unwanted)); !os.IsNotExist(statErr) {
			t.Fatalf("%s/ folder must not have been created — validation must run before any move", unwanted)
		}
	}

	data, readErr := os.ReadFile(filepath.Join(dir, manifestName))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("manifest must still be the original v1 content after a failed migration, got: %s", data)
	}
}
```

- [ ] **Step 3: Lancer les tests pour vérifier qu'ils échouent**

Run: `go test . -run 'TestMigrateV1ToV2FailsAtomicallyIfAMoveFails|TestMigrateV1ToV2ValidatesAllSourcesBeforeMovingAny' -v`

Expected: `TestMigrateV1ToV2FailsAtomicallyIfAMoveFails` FAIL (le fichier `01-un.md` a bien été déplacé
par l'implémentation actuelle avant l'échec sur `missing.md` — sauf que dans ce cas précis `01-un.md`
vient AVANT l'item manquant dans `items[]`, donc son déplacement a déjà réussi avant l'échec ; le
`os.Stat(filepath.Join(dir, "01-un.md"))` du nouveau test échouera puisque le fichier n'est plus à
la racine).
`TestMigrateV1ToV2ValidatesAllSourcesBeforeMovingAny` FAIL de la même manière (`annonce/` et
`attribution/` existeront déjà, créés par l'implémentation actuelle avant l'échec sur l'item 3).

- [ ] **Step 4: Écrire l'implémentation minimale — validation en amont**

Dans `migration.go`, remplacer `migrateV1ToV2` (lignes 150-169) :

```go
// migrateV1ToV2 executes the migration for dir: moves each v1 chapter file into
// its own new folder, then writes the v2 manifest ONCE at the end — only if
// every move succeeded. If any move fails partway through, migration stops and
// returns the error without writing the manifest, so the original v1 manifest
// (and whatever files were already moved) is the only state change on disk; the
// caller is expected to surface the error rather than retry automatically.
func migrateV1ToV2(dir string, v1 manifestV1) error {
	plan, err := migratePlan(dir, v1)
	if err != nil {
		return err
	}
	for _, mv := range plan.moves {
		fromPath := filepath.Join(dir, mv.fromFile)
		if _, err := os.Stat(fromPath); err != nil {
			return fmt.Errorf("migration: %s: %w", mv.fromFile, err)
		}
		toDir := filepath.Join(dir, mv.toFolder)
		if err := os.MkdirAll(toDir, 0o755); err != nil {
			return fmt.Errorf("migration: mkdir %s: %w", mv.toFolder, err)
		}
		if err := os.Rename(fromPath, filepath.Join(toDir, mv.toFile)); err != nil {
			return fmt.Errorf("migration: move %s: %w", mv.fromFile, err)
		}
	}
	return writeManifest(dir, plan.out)
}
```

par :

```go
// migrateV1ToV2 executes the migration for dir: moves each v1 chapter file into
// its own new folder, then writes the v2 manifest ONCE at the end — only if
// every move succeeded. Before touching disk at all, every source file is
// validated to exist (validateSourcesExist) — this makes the migration "all or
// nothing" at the file-move level too, not just at the manifest-write level: a
// manifest referencing a missing/typo'd filename fails immediately, with NO
// folder created and NO file moved, so the manuscript is left exactly as it was
// (still v1, still readable next time via needsMigration). Without this
// up-front check, an item late in items[] failing after earlier items already
// moved would leave a hybrid disk state (some files moved, manifest still v1) —
// the exact bug that made migration silently retry forever on every launch.
func migrateV1ToV2(dir string, v1 manifestV1) error {
	plan, err := migratePlan(dir, v1)
	if err != nil {
		return err
	}
	if err := validateSourcesExist(dir, plan.moves); err != nil {
		return err
	}
	for _, mv := range plan.moves {
		toDir := filepath.Join(dir, mv.toFolder)
		if err := os.MkdirAll(toDir, 0o755); err != nil {
			return fmt.Errorf("migration: mkdir %s: %w", mv.toFolder, err)
		}
		if err := os.Rename(filepath.Join(dir, mv.fromFile), filepath.Join(toDir, mv.toFile)); err != nil {
			return fmt.Errorf("migration: move %s: %w", mv.fromFile, err)
		}
	}
	return writeManifest(dir, plan.out)
}

// validateSourcesExist checks that every move's source file exists in dir,
// before migrateV1ToV2 moves anything. Returns the first missing file's error,
// wrapped with its filename — the caller surfaces this to the user (see
// confirmMigration, main.go) so a typo'd or renamed v1 manifest entry is
// reported instead of silently corrupting the manuscript's on-disk state.
func validateSourcesExist(dir string, moves []migrationMove) error {
	for _, mv := range moves {
		if _, err := os.Stat(filepath.Join(dir, mv.fromFile)); err != nil {
			return fmt.Errorf("migration: %s: %w", mv.fromFile, err)
		}
	}
	return nil
}
```

- [ ] **Step 5: Lancer les tests pour vérifier qu'ils passent**

Run: `go test . -run 'TestMigrateV1ToV2FailsAtomicallyIfAMoveFails|TestMigrateV1ToV2ValidatesAllSourcesBeforeMovingAny' -v`

Expected: PASS

- [ ] **Step 6: Lancer la suite complète de `migration_test.go` (regression check)**

Run: `go test . -run 'TestMigratePlan|TestMigrateV1ToV2|TestNeedsMigration' -v`

Expected: PASS — en particulier `TestMigrateV1ToV2MovesFilesAndWritesV2Manifest` et
`TestMigrateV1ToV2DedupesCollidingSlugs` (cas nominaux, tous les fichiers présents) doivent
continuer à réussir sans modification, la validation en amont ne changeant rien à leur chemin.

- [ ] **Step 7: Lancer la suite complète du projet (regression check)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add migration.go migration_test.go
git commit -m "migration: valide l'existence de tous les fichiers source avant tout déplacement"
```

---

## Task 2: Erreur visible dans `confirmMigration`

**Files:**
- Modify: `main.go:140-156` (`confirmMigration`)
- Test: `main_test.go` (nouveau test)

**Interfaces:**
- Consumes: `migrateV1ToV2(dir string, v1 manifestV1) error` (Task 1, signature inchangée),
  `m.status string` (champ déjà existant sur `model`, utilisé ailleurs dans `main.go` pour des
  messages transitoires, ex. ligne ~1049 `"échec de la vérification grammaticale"`).
- Produces: aucune nouvelle interface publique — seul le comportement interne de `confirmMigration`
  change (le retour d'erreur de `migrateV1ToV2`, aujourd'hui avalé, alimente désormais `m.status`).

**Note d'implémentation** : `confirmMigration` (`main.go:142-156`) avale aujourd'hui l'erreur de
`migrateV1ToV2` avec `_ = migrateV1ToV2(m.migrationDir, v1)` (ligne 148). Cette tâche capture cette
erreur et, si non-nil, l'écrit dans `m.status`. Le reste du flux (`m.migrationPending = nil`,
`m.files.SetDir(...)`, `m.applyProjectSettings()`, `m.screen = screenWriting`) reste identique dans
tous les cas — succès ou échec — conformément à la contrainte globale du plan (pas de nouvel écran,
pas de nouveau flux de retry).

- [ ] **Step 1: Écrire le test qui échoue**

Ajouter dans `main_test.go`, à la suite de `TestConfirmMigrationExecutesAndEntersWriting` :

```go
// TestConfirmMigrationShowsErrorStatusOnFailure reproduces the real bug: before
// this fix, confirmMigration discarded migrateV1ToV2's error with `_ =`, so a v1
// manifest referencing a missing/typo'd filename failed silently — the user saw
// no indication anything went wrong, and the migration prompt would return on
// every subsequent launch with no clue why. After the fix, the error surfaces in
// m.status, naming the offending file.
func TestConfirmMigrationShowsErrorStatusOnFailure(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-un.md"), []byte("x"), 0o644)
	// "missing.md" is declared but never created on disk — migration will fail.
	os.WriteFile(filepath.Join(dir, manifestName),
		[]byte(`{"schemaVersion":1,"title":"N","items":[
			{"file":"01-un.md","title":"Un"},{"file":"missing.md","title":"Deux"}]}`), 0o644)

	t.Setenv("OKASHI_DIR", dir)
	m := initialModel()
	m.enterWriting()
	if m.migrationPending == nil {
		t.Fatal("setup: expected migrationPending")
	}

	m.confirmMigration()

	if m.migrationPending != nil {
		t.Fatal("confirmMigration must clear migrationPending even on failure")
	}
	if m.screen != screenWriting {
		t.Fatal("confirmMigration must enter the writing screen even on failure")
	}
	if !strings.Contains(m.status, "missing.md") {
		t.Fatalf("m.status must name the missing file after a failed migration, got: %q", m.status)
	}

	// The manifest must remain v1 — migration must not have partially succeeded.
	data, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), `"schemaVersion":1`) {
		t.Fatalf("manifest must still be v1 after a failed migration, got: %s", data)
	}
}
```

- [ ] **Step 2: Lancer le test pour vérifier qu'il échoue**

Run: `go test . -run TestConfirmMigrationShowsErrorStatusOnFailure -v`

Expected: FAIL — `m.status` est vide (l'erreur est aujourd'hui avalée par `_ =`).

- [ ] **Step 3: Écrire l'implémentation minimale**

Dans `main.go`, remplacer `confirmMigration` (lignes 140-156) :

```go
// confirmMigration executes the pending v1→v2 migration and enters the writing
// screen. Called when the author accepts the migration confirm screen.
func (m *model) confirmMigration() {
	if m.migrationPending == nil {
		return
	}
	v1, _, err := readManifestV1(m.migrationDir)
	if err == nil {
		_ = migrateV1ToV2(m.migrationDir, v1) // best-effort; errors surface via the
		// manuscript's warning path on next resolveManuscript, consistent with
		// every other write path in this codebase (no modal error dialog).
	}
	m.migrationPending = nil
	m.files.SetDir(m.files.dir) // re-reads entries against the migrated v2 manifest
	m.applyProjectSettings()
	m.screen = screenWriting
}
```

par :

```go
// confirmMigration executes the pending v1→v2 migration and enters the writing
// screen. Called when the author accepts the migration confirm screen. On
// failure (e.g. the v1 manifest references a missing or typo'd filename), the
// error surfaces in m.status instead of being silently discarded — otherwise
// the manuscript stays on v1 and needsMigration re-triggers the same failing
// migration on every subsequent launch with no indication why (the bug this
// fixes). The manifest itself is untouched on failure (migrateV1ToV2 validates
// every source file before moving anything), so the author can fix the
// offending file or manifest entry and simply relaunch okashi to retry.
func (m *model) confirmMigration() {
	if m.migrationPending == nil {
		return
	}
	v1, _, err := readManifestV1(m.migrationDir)
	if err == nil {
		if migErr := migrateV1ToV2(m.migrationDir, v1); migErr != nil {
			m.status = "migration échouée : " + migErr.Error()
		}
	}
	m.migrationPending = nil
	m.files.SetDir(m.files.dir) // re-reads entries against the migrated v2 manifest
	m.applyProjectSettings()
	m.screen = screenWriting
}
```

- [ ] **Step 4: Lancer le test pour vérifier qu'il passe**

Run: `go test . -run TestConfirmMigrationShowsErrorStatusOnFailure -v`

Expected: PASS

- [ ] **Step 5: Lancer la suite complète des tests de migration (regression check)**

Run: `go test . -run 'TestEnterWriting|TestConfirmMigration|TestCancelMigration|TestMigrate|TestNeedsMigration' -v`

Expected: PASS — en particulier `TestConfirmMigrationExecutesAndEntersWriting` (cas nominal, succès)
doit continuer à passer sans `m.status` renseigné (aucune régression sur le chemin heureux).

- [ ] **Step 6: Lancer la suite complète du projet (regression check)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go
git commit -m "migration: affiche l'erreur dans la barre de statut au lieu de l'avaler silencieusement"
```

---

## Hors périmètre (rappel, ne pas implémenter dans ce plan)

- Migration partielle / option "continuer malgré les items en échec".
- Changement à `migrationConfirmView` (l'écran de plan affiché avant confirmation reste identique).
- Détection ou correction automatique des extensions de fichier erronées dans un manifest v1.
- Nouveau mécanisme de retry automatique en session.
