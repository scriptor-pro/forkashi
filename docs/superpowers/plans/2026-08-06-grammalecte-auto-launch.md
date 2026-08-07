# Auto-lancement du serveur Grammalecte — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Permettre à okashi de lancer et gérer automatiquement son propre serveur Grammalecte en
sous-processus, configuré via `OKASHI_GRAMMALECTE_CMD`, sans bloquer le démarrage et sans jamais
toucher un serveur qu'okashi n'a pas lui-même lancé.

**Architecture:** Un second point d'injection package-level (`launchGrammalecte`, miroir de
`newGrammarChecker`) découple "lancer le process" de "vérifier la disponibilité", pour garder les
tests rapides et sans dépendance à un vrai interpréteur Python. `initialModel()` reste synchrone et
non-bloquant : elle lance le sous-processus si besoin puis retourne immédiatement avec
`m.grammarChecker == nil` si pas encore prêt. Un nouveau `tea.Cmd` de polling (`pollGrammalecteCmd`,
même style que `checkGrammarCmd`) se reprogramme via `tea.Tick` jusqu'à 60 fois (30s) et pousse un
message `grammalecteReadyMsg` que `Update()` consomme pour activer `m.grammarChecker`. Le nettoyage à
la fermeture se fait dans `func main()`, après `p.Run()`, sur le `*exec.Cmd` conservé dans le modèle
final (nil si le serveur était déjà là au démarrage — jamais la propriété d'okashi dans ce cas).

**Tech Stack:** Go 1.25, `os/exec` (nouveau dans le projet), Bubble Tea (`tea.Cmd`/`tea.Tick`/`tea.Msg`),
tests `go test` standard (table-driven + `httptest.NewServer` pour simuler le serveur Grammalecte,
suivant le style déjà en place dans `grammar_grammalecte_test.go`).

## Global Constraints

- Nouvelle variable d'environnement `OKASHI_GRAMMALECTE_CMD` : commande complète à lancer (programme +
  arguments), parsée simplement (`strings.Fields` ou équivalent) — **pas** un shell complet, pas de
  support de `&&`, `|`, redirections, ou substitution de variables internes à la chaîne.
- Non définie → comportement actuel préservé à l'identique (aucun lancement automatique) + message
  d'aide discret via `m.status` (pas une popup, pas le layout à géométrie fixe de l'inspector —
  `inspector.go:579-589` a des positions de ligne critiques pour les clics, ne pas y toucher).
- Un serveur déjà disponible au démarrage (`gc.Available()` vrai avant tout lancement) n'est **jamais**
  lancé ni tué par okashi, peu importe qui l'a démarré.
- `initialModel()` reste non-bloquante : retourne immédiatement, jamais d'attente synchrone des ~5-6s
  de démarrage du serveur Python.
- Polling : `tea.Tick` toutes les 500ms, plafonné à 60 tentatives (30s), silencieux à l'échec comme à
  l'expiration — cohérent avec la philosophie best-effort déjà documentée sur `Available()`
  (`grammar_grammalecte.go:49-56`, *"okashi must run fine without French grammar checking rather than
  crash"*).
- Le sous-processus lancé par okashi est tué (`SIGTERM`, `Kill()` en repli) uniquement à la fermeture
  du programme, dans `func main()` après `p.Run()` — jamais depuis `Update()`.
- Fichiers de référence pour le style de code et de tests :
  - `grammar_backend.go:36-48` (`newGrammarChecker`, le point d'injection existant à imiter)
  - `main.go:519-587` (`initialModel()`, à étendre — probe existante `main.go:553-556` à préserver)
  - `main.go:650-656` (`autosaveTickMsg`/`autosaveTick`, modèle de `tea.Tick`)
  - `main.go:682-689` (`checkGrammarCmd`, modèle de `tea.Cmd` simple retournant un message)
  - `main.go:675-680`, `1037-1052` (`grammarResultMsg`, modèle de traitement de message dans `Update()`)
  - `main.go:3018-3042` (`func main()`, à étendre pour le cleanup)
  - `grammar_grammalecte_test.go:1-60` (style de test avec `httptest.NewServer` + `t.Setenv`)
  - `smoke_test.go:1809-1826`, `1860-1863` (pattern d'injection de `newGrammarChecker` factice dans les
    tests — `orig := newGrammarChecker; newGrammarChecker = func() grammarChecker {...}; defer func()
    { newGrammarChecker = orig }()`)

---

## Task 1: Lecture de la config et point d'injection du lancement

**Files:**
- Modify: `grammar_grammalecte.go` (ajouter après `grammalectePort()`, ligne 34)
- Test: `grammar_grammalecte_test.go` (ajouter à la suite des tests existants)

**Interfaces:**
- Produces:
  - `func grammalecteAutoLaunchCmd() (string, bool)` — lit `OKASHI_GRAMMALECTE_CMD`, retourne
    `("", false)` si absente/vide, `(cmdline, true)` sinon.
  - `func launchGrammalecteServer(cmdline string) (*exec.Cmd, error)` — découpe `cmdline` en
    programme + arguments (`strings.Fields`), lance via `exec.Command(...).Start()` (non-bloquant,
    stdin/stdout/stderr **non capturés**, laissés à zéro-valeur — le TUI ne doit jamais voir la sortie
    du process Python), retourne le `*exec.Cmd` démarré ou une erreur si `Start()` échoue ou si
    `cmdline` est vide après découpage.
  - `var launchGrammalecte = launchGrammalecteServer` — variable de package (miroir de
    `newGrammarChecker`), pour permettre l'injection d'un lanceur factice dans les tests sans jamais
    invoquer un vrai interpréteur Python.

- [ ] **Step 1: Write the failing tests**

Ajouter dans `grammar_grammalecte_test.go` :

```go
func TestGrammalecteAutoLaunchCmd(t *testing.T) {
	t.Setenv("OKASHI_GRAMMALECTE_CMD", "")
	if _, ok := grammalecteAutoLaunchCmd(); ok {
		t.Fatal("empty OKASHI_GRAMMALECTE_CMD should report not configured")
	}

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "python3 /path/to/grammalecte-server.py")
	cmdline, ok := grammalecteAutoLaunchCmd()
	if !ok {
		t.Fatal("non-empty OKASHI_GRAMMALECTE_CMD should report configured")
	}
	if cmdline != "python3 /path/to/grammalecte-server.py" {
		t.Fatalf("unexpected cmdline: %q", cmdline)
	}
}

func TestLaunchGrammalecteServer(t *testing.T) {
	// "true" is a real, near-instant, dependency-free binary on any Unix system —
	// enough to prove Start() succeeds and returns a live *exec.Cmd without
	// depending on Python or Grammalecte being installed in the test environment.
	cmd, err := launchGrammalecteServer("true")
	if err != nil {
		t.Fatalf("launchGrammalecteServer(\"true\") returned error: %v", err)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil *exec.Cmd")
	}
	cmd.Wait() // reap the child so the test doesn't leak a zombie process

	if _, err := launchGrammalecteServer(""); err == nil {
		t.Fatal("launchGrammalecteServer(\"\") should return an error, not silently no-op")
	}

	if _, err := launchGrammalecteServer("this-binary-does-not-exist-anywhere"); err == nil {
		t.Fatal("launchGrammalecteServer with a nonexistent program should return an error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestGrammalecteAutoLaunchCmd|TestLaunchGrammalecteServer' -v`
Expected: FAIL — `grammalecteAutoLaunchCmd` and `launchGrammalecteServer` undefined.

- [ ] **Step 3: Write minimal implementation**

Ajouter dans `grammar_grammalecte.go`, après `grammalectePort()` (ligne 34), et ajouter `os/exec` à
l'import existant :

```go
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)
```

```go
// grammalecteAutoLaunchCmd reports the command line to auto-launch a Grammalecte server, read
// from OKASHI_GRAMMALECTE_CMD. ok=false means the variable is unset or empty — auto-launch is
// off, preserving today's behavior (manual server management only).
func grammalecteAutoLaunchCmd() (string, bool) {
	cmdline := os.Getenv("OKASHI_GRAMMALECTE_CMD")
	if cmdline == "" {
		return "", false
	}
	return cmdline, true
}

// launchGrammalecteServer starts cmdline as a detached subprocess. cmdline is split on
// whitespace into a program and its arguments — this is NOT a shell: no &&, |, redirections, or
// variable substitution are supported, only a plain command with positional arguments. Stdin,
// stdout, and stderr are left at their zero value (not inherited, not captured) so the launched
// process never writes to the TUI's terminal. The returned *exec.Cmd is the caller's handle for
// later cleanup (see main()); the process itself keeps running after this function returns.
func launchGrammalecteServer(cmdline string) (*exec.Cmd, error) {
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return nil, fmt.Errorf("grammalecte: empty command line")
	}
	cmd := exec.Command(fields[0], fields[1:]...)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("grammalecte: failed to start %q: %w", cmdline, err)
	}
	return cmd, nil
}

// launchGrammalecte is the package var the model calls to auto-launch a server. Tests can
// replace it with a fake to avoid spawning real processes (mirrors newGrammarChecker,
// grammar_backend.go:48).
var launchGrammalecte = launchGrammalecteServer
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestGrammalecteAutoLaunchCmd|TestLaunchGrammalecteServer' -v`
Expected: PASS

- [ ] **Step 5: Run the full test suite (regression check)**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add grammar_grammalecte.go grammar_grammalecte_test.go
git commit -m "grammalecte: ajoute la lecture de OKASHI_GRAMMALECTE_CMD et le lancement du sous-processus"
```

---

## Task 2: Câblage dans `initialModel()` — champ modèle + lancement conditionnel

**Files:**
- Modify: `main.go` (struct `model` autour de la ligne 483 ; `initialModel()` autour des lignes 553-586)
- Test: `smoke_test.go` (nouveaux tests)

**Interfaces:**
- Consumes: `grammalecteAutoLaunchCmd()`, `launchGrammalecte` (Task 1).
- Produces: nouveau champ `model.grammalecteProc *exec.Cmd` — nil sauf si okashi a lui-même lancé le
  sous-processus (c'est le marqueur de propriété utilisé par Task 4 pour le cleanup).

**Note d'implémentation** : la probe existante (`main.go:553-556`, `gc := newGrammarChecker(); if gc
!= nil && !gc.Available() { gc = nil }`) reste le premier geste, inchangée. Le lancement ne se
déclenche que si cette probe a échoué (`gc == nil` après la probe) ET que `grammalecteAutoLaunchCmd()`
retourne `ok == true`. Si le lancement réussit, `m.grammarChecker` reste `nil` à ce stade (le serveur
vient d'être lancé, pas encore confirmé disponible — Task 3 s'en charge via le polling) mais
`m.grammalecteProc` est renseigné. Si `launchGrammalecte` échoue (erreur retournée), le comportement
reste celui d'aujourd'hui : `m.grammarChecker` et `m.grammalecteProc` restent tous deux nil,
silencieusement (philosophie best-effort).

- [ ] **Step 1: Write the failing tests**

Ajouter dans `smoke_test.go` :

```go
func TestInitialModelLaunchesGrammalecteWhenConfiguredAndUnavailable(t *testing.T) {
	origChecker := newGrammarChecker
	origLaunch := launchGrammalecte
	defer func() {
		newGrammarChecker = origChecker
		launchGrammalecte = origLaunch
	}()

	newGrammarChecker = func() grammarChecker { return fakeChecker{} } // Available() defaults false on the zero-value grammarChecker below
	launched := false
	launchGrammalecte = func(cmdline string) (*exec.Cmd, error) {
		launched = true
		if cmdline != "fake-server --flag" {
			t.Fatalf("unexpected cmdline passed to launchGrammalecte: %q", cmdline)
		}
		return exec.Command("true"), nil
	}

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "fake-server --flag")
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)

	m := initialModel()

	if !launched {
		t.Fatal("initialModel should have called launchGrammalecte when no server was available and OKASHI_GRAMMALECTE_CMD was set")
	}
	if m.grammalecteProc == nil {
		t.Fatal("model.grammalecteProc should be set after a successful auto-launch")
	}
	if m.grammarChecker != nil {
		t.Fatal("grammarChecker should still be nil right after launch — availability isn't confirmed yet")
	}
	m.grammalecteProc.Wait() // reap the child
}

func TestInitialModelDoesNotLaunchWhenNotConfigured(t *testing.T) {
	origChecker := newGrammarChecker
	origLaunch := launchGrammalecte
	defer func() {
		newGrammarChecker = origChecker
		launchGrammalecte = origLaunch
	}()

	newGrammarChecker = func() grammarChecker { return fakeChecker{} }
	launched := false
	launchGrammalecte = func(cmdline string) (*exec.Cmd, error) {
		launched = true
		return nil, nil
	}

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "")
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)

	m := initialModel()

	if launched {
		t.Fatal("initialModel should not launch anything when OKASHI_GRAMMALECTE_CMD is unset")
	}
	if m.grammalecteProc != nil {
		t.Fatal("model.grammalecteProc should be nil when nothing was launched")
	}
}

func TestInitialModelDoesNotLaunchWhenServerAlreadyAvailable(t *testing.T) {
	origChecker := newGrammarChecker
	origLaunch := launchGrammalecte
	defer func() {
		newGrammarChecker = origChecker
		launchGrammalecte = origLaunch
	}()

	newGrammarChecker = func() grammarChecker { return availableFakeChecker{} }
	launched := false
	launchGrammalecte = func(cmdline string) (*exec.Cmd, error) {
		launched = true
		return nil, nil
	}

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "fake-server")
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)

	m := initialModel()

	if launched {
		t.Fatal("initialModel should not launch a server that is already available")
	}
	if m.grammalecteProc != nil {
		t.Fatal("model.grammalecteProc should be nil — okashi never owns a server it didn't launch")
	}
	if m.grammarChecker == nil {
		t.Fatal("grammarChecker should be set immediately when the server was already available")
	}
}

// availableFakeChecker is a grammarChecker whose Available() always reports true, for testing
// the "server already running" path distinctly from fakeChecker's default false.
type availableFakeChecker struct{}

func (availableFakeChecker) Name() string                           { return "Fake" }
func (availableFakeChecker) Available() bool                        { return true }
func (availableFakeChecker) Check(string) ([]grammarFinding, error) { return nil, nil }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestInitialModelLaunchesGrammalecteWhenConfiguredAndUnavailable|TestInitialModelDoesNotLaunchWhenNotConfigured|TestInitialModelDoesNotLaunchWhenServerAlreadyAvailable' -v`
Expected: FAIL — `m.grammalecteProc` undefined (compile error).

- [ ] **Step 3: Add the model field**

Dans `main.go`, ajouter le champ après `lastGrammarCheck time.Time` (ligne 483) :

```go
	grammarChecker   grammarChecker
	appleFindings    map[string][]grammarFinding
	checkingGrammar  bool
	autoRecheck      bool      // re-run the Apple pass after edits settle (opt-in)
	lastGrammarCheck time.Time // when the last Apple pass was dispatched
	// grammalecteProc is non-nil only when THIS okashi process launched the Grammalecte
	// server itself (OKASHI_GRAMMALECTE_CMD configured, no server was already reachable).
	// It is the ownership marker main() uses to decide whether to kill the subprocess on
	// exit — a server that was already running before okashi started is never touched.
	grammalecteProc *exec.Cmd
```

Ajouter `"os/exec"` aux imports de `main.go` s'il n'y est pas déjà.

- [ ] **Step 4: Wire the launch into `initialModel()`**

Dans `main.go`, remplacer le bloc existant (lignes 549-556) :

```go
	// Probe the grammar backend's availability once, here, at startup — not on every
	// render. If unavailable (no Grammalecte server running), fall back to nil so
	// m.grammarChecker != nil keeps meaning "backend is actually usable" everywhere it's
	// checked (the analysis action row, the click handlers, the inspector label).
	gc := newGrammarChecker()
	if gc != nil && !gc.Available() {
		gc = nil
	}
```

par :

```go
	// Probe the grammar backend's availability once, here, at startup — not on every
	// render. If unavailable (no Grammalecte server running), fall back to nil so
	// m.grammarChecker != nil keeps meaning "backend is actually usable" everywhere it's
	// checked (the analysis action row, the click handlers, the inspector label).
	gcForProbe := newGrammarChecker()
	available := gcForProbe != nil && gcForProbe.Available()
	gc := gcForProbe
	if !available {
		gc = nil
	}

	// Auto-launch: only when nothing answered above AND the user opted in via
	// OKASHI_GRAMMALECTE_CMD. A server that was already reachable is never a candidate
	// for launching — it isn't okashi's to own or later kill (see main()'s cleanup).
	var grammalecteProc *exec.Cmd
	if !available {
		if cmdline, ok := grammalecteAutoLaunchCmd(); ok {
			if proc, err := launchGrammalecte(cmdline); err == nil {
				grammalecteProc = proc
			}
			// err != nil: silent, best-effort — grammarChecker and grammalecteProc both
			// stay nil, identical to today's "no server available" behavior.
		}
	}
```

Dans le littéral `model{...}` (lignes 558-584), ajouter le nouveau champ après `grammarChecker: gc,` :

```go
		grammarChecker:  gc,
		grammalecteProc: grammalecteProc,
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestInitialModelLaunchesGrammalecteWhenConfiguredAndUnavailable|TestInitialModelDoesNotLaunchWhenNotConfigured|TestInitialModelDoesNotLaunchWhenServerAlreadyAvailable' -v`
Expected: PASS

- [ ] **Step 6: Run the full test suite (regression check)**

Run: `go build ./... && go test ./...`
Expected: PASS — en particulier vérifier que les tests existants qui injectent `newGrammarChecker`
(`TestCheckGrammarAction`, `TestActionRowHiddenWithoutBackend`, etc., `smoke_test.go`) passent toujours
sans modification (ils n'ont pas `OKASHI_GRAMMALECTE_CMD` configurée dans leur environnement de test,
donc le nouveau chemin de lancement ne se déclenche jamais pour eux).

- [ ] **Step 7: Commit**

```bash
git add main.go smoke_test.go
git commit -m "grammalecte: lance le sous-processus au démarrage si configuré et aucun serveur disponible"
```

---

## Task 3: Polling non-bloquant de disponibilité

**Files:**
- Modify: `main.go` (nouveau type de message + `tea.Cmd` près de `checkGrammarCmd` ; `Init()` ; `Update()`)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `m.grammarChecker` construit mais potentiellement encore indisponible (Task 2),
  `m.grammalecteProc != nil` comme condition de déclenchement du polling.
- Produces:
  - `type grammalecteReadyMsg struct{ checker grammarChecker }`
  - `func pollGrammalecteCmd(gc grammarChecker, attempt int) tea.Cmd`
  - `const grammalectePollMaxAttempts = 60`
  - `const grammalectePollInterval = 500 * time.Millisecond`

**Note d'implémentation** : le polling ne démarre que si `m.grammalecteProc != nil` immédiatement après
`initialModel()` — c'est-à-dire seulement quand okashi vient de lancer le sous-processus lui-même
(Task 2). Le point de départ le plus simple est `Init()` (`main.go:975-977`), qui retourne déjà
`autosaveTick()` — il peut retourner un `tea.Batch` des deux commandes. `Init()` est un receiver par
valeur (`func (m model) Init() tea.Cmd`), donc il a accès en lecture à `m.grammarChecker` et
`m.grammalecteProc` tels que construits par `initialModel()`.

- [ ] **Step 1: Write the failing unit test for `pollGrammalecteCmd`**

Ajouter dans `smoke_test.go` :

```go
func TestPollGrammalecteCmdSucceedsWhenAvailable(t *testing.T) {
	cmd := pollGrammalecteCmd(availableFakeChecker{}, 0)
	msg := cmd()
	if _, ok := msg.(grammalecteReadyMsg); !ok {
		t.Fatalf("expected grammalecteReadyMsg when checker is available, got %#v", msg)
	}
}

func TestPollGrammalecteCmdGivesUpAtCeiling(t *testing.T) {
	cmd := pollGrammalecteCmd(fakeChecker{}, grammalectePollMaxAttempts)
	msg := cmd()
	if msg != nil {
		t.Fatalf("expected nil msg once the attempt ceiling is reached, got %#v", msg)
	}
}

func TestPollGrammalecteCmdReschedulesBelowCeiling(t *testing.T) {
	// fakeChecker.Available() is false by default (zero value), so this call will sleep
	// 500ms and recurse — verify it eventually gives up rather than looping forever, using
	// a checker that never becomes available and a small attempt budget to keep the test fast.
	start := time.Now()
	cmd := pollGrammalecteCmd(fakeChecker{}, grammalectePollMaxAttempts-1) // one retry left
	msg := cmd()
	elapsed := time.Since(start)
	if elapsed < grammalectePollInterval {
		t.Fatalf("expected at least one %v sleep before giving up, elapsed only %v", grammalectePollInterval, elapsed)
	}
	if msg != nil {
		t.Fatalf("expected nil msg after the final retry still fails, got %#v", msg)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestPollGrammalecteCmd' -v`
Expected: FAIL — `pollGrammalecteCmd`, `grammalecteReadyMsg`, `grammalectePollMaxAttempts` undefined.

- [ ] **Step 3: Write minimal implementation**

Ajouter dans `main.go`, juste après `checkGrammarCmd` (après la ligne 689) :

```go
// grammalecteReadyMsg signals that an auto-launched Grammalecte server has become reachable.
// It carries the checker so Update() can activate it without needing outside context.
type grammalecteReadyMsg struct{ checker grammarChecker }

// grammalectePollInterval is the delay between successive availability checks while waiting
// for an auto-launched Grammalecte server to finish starting up (typically ~5-6s in practice).
const grammalectePollInterval = 500 * time.Millisecond

// grammalectePollMaxAttempts caps how long pollGrammalecteCmd keeps retrying (60 × 500ms = 30s)
// before giving up silently — the launched process may have crashed or never bind its port;
// best-effort means okashi keeps running without French grammar checking rather than polling
// forever.
const grammalectePollMaxAttempts = 60

// pollGrammalecteCmd checks whether an auto-launched Grammalecte server has become reachable
// yet. On success it returns grammalecteReadyMsg carrying gc. On failure it sleeps
// grammalectePollInterval and retries, up to grammalectePollMaxAttempts total attempts, after
// which it gives up silently (returns nil — Update() treats a nil tea.Msg as a no-op, so this
// simply stops the polling loop without any user-visible error).
func pollGrammalecteCmd(gc grammarChecker, attempt int) tea.Cmd {
	return func() tea.Msg {
		for {
			if gc.Available() {
				return grammalecteReadyMsg{checker: gc}
			}
			if attempt >= grammalectePollMaxAttempts {
				return nil
			}
			time.Sleep(grammalectePollInterval)
			attempt++
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestPollGrammalecteCmd' -v`
Expected: PASS

- [ ] **Step 5: Write the failing tests for `Init()` and `Update()` wiring**

Ajouter dans `smoke_test.go` :

```go
func TestInitStartsGrammalectePollWhenAutoLaunched(t *testing.T) {
	m := initialModel()
	m.grammalecteProc = &exec.Cmd{} // simulate "okashi launched a process" without really spawning one
	m.grammarChecker = nil
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() should return a non-nil command when grammalecteProc is set")
	}
}

func TestEditorGetsGrammarCheckerAfterPollSucceeds(t *testing.T) {
	m := initialModel()
	m.grammarChecker = nil
	fake := availableFakeChecker{}
	nm, _ := m.Update(grammalecteReadyMsg{checker: fake})
	m = nm.(model)
	if m.grammarChecker == nil {
		t.Fatal("grammarChecker should be set after receiving grammalecteReadyMsg")
	}
	if m.grammarChecker.Name() != fake.Name() {
		t.Fatalf("grammarChecker.Name() = %q, want %q", m.grammarChecker.Name(), fake.Name())
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test . -run 'TestInitStartsGrammalectePollWhenAutoLaunched|TestEditorGetsGrammarCheckerAfterPollSucceeds' -v`
Expected: FAIL — `Init()` doesn't yet check `grammalecteProc`; `Update()` doesn't yet handle
`grammalecteReadyMsg`.

- [ ] **Step 7: Wire polling into `Init()` and message handling into `Update()`**

Dans `main.go`, remplacer `Init()` (lignes 975-977) :

```go
func (m model) Init() tea.Cmd {
	return autosaveTick()
}
```

par :

```go
func (m model) Init() tea.Cmd {
	if m.grammalecteProc != nil && m.grammarChecker == nil {
		return tea.Batch(autosaveTick(), pollGrammalecteCmd(newGrammarChecker(), 0))
	}
	return autosaveTick()
}
```

**Attention** : `newGrammarChecker()` reconstruit un checker frais ici plutôt que de réutiliser un
champ stocké — c'est intentionnel et sûr, puisque `grammalecteChecker()` (`grammar_grammalecte.go:37-42`)
ne fait que construire un client HTTP sans état (pas de connexion ouverte à l'appel). Ce nouveau
checker est celui dont `Available()` sera sondé, et c'est lui qui remontera dans
`grammalecteReadyMsg.checker` une fois prêt.

Dans `main.go`, ajouter le traitement du message juste après le bloc `grammarResultMsg` (après la
ligne 1052) :

```go
	if msg, ok := msg.(grammalecteReadyMsg); ok {
		m.grammarChecker = msg.checker
		return m, nil
	}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test . -run 'TestInitStartsGrammalectePollWhenAutoLaunched|TestEditorGetsGrammarCheckerAfterPollSucceeds' -v`
Expected: PASS

- [ ] **Step 9: Run the full test suite (regression check)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add main.go smoke_test.go
git commit -m "grammalecte: sonde la disponibilité du serveur auto-lancé sans bloquer le démarrage"
```

---

## Task 4: Message d'aide et nettoyage à la fermeture

**Files:**
- Modify: `main.go` (`View()` autour de la ligne 1946 pour le message d'aide ; `func main()` pour le
  cleanup)
- Test: `smoke_test.go`

**Interfaces:**
- Consumes: `m.grammarChecker`, `m.grammalecteProc` (Task 2), `grammalecteAutoLaunchCmd()` (Task 1).
- Produces: `func killGrammalecteProc(proc *exec.Cmd)` — utilisé uniquement par `main()`, testable
  isolément.

**Note d'implémentation sur le message d'aide** : `inspector.go:579-589` a un layout à géométrie fixe
documenté ("Stable layout so the click rows never shift") avec des positions de ligne utilisées par
des gestionnaires de clic (`analysisActionRowY`). Ne pas y ajouter de ligne conditionnelle. Le message
d'aide utilise plutôt `m.status` (la barre de statut, déjà utilisée pour des messages transitoires
comme `"échec de la vérification grammaticale"`, `main.go:1049`) — affiché une fois, au démarrage,
seulement quand pertinent (pas de commande configurée ET pas de backend disponible).

- [ ] **Step 1: Write the failing test for the help message**

Ajouter dans `smoke_test.go` :

```go
func TestInitialModelShowsHelpStatusWhenNotConfiguredAndUnavailable(t *testing.T) {
	origChecker := newGrammarChecker
	defer func() { newGrammarChecker = origChecker }()
	newGrammarChecker = func() grammarChecker { return fakeChecker{} } // Available() false

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "")
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)

	m := initialModel()

	if !strings.Contains(m.status, "OKASHI_GRAMMALECTE_CMD") {
		t.Fatalf("status should hint at OKASHI_GRAMMALECTE_CMD when no backend is available and auto-launch isn't configured, got %q", m.status)
	}
}

func TestInitialModelNoHelpStatusWhenBackendAvailable(t *testing.T) {
	origChecker := newGrammarChecker
	defer func() { newGrammarChecker = origChecker }()
	newGrammarChecker = func() grammarChecker { return availableFakeChecker{} }

	t.Setenv("OKASHI_GRAMMALECTE_CMD", "")
	dir := t.TempDir()
	t.Setenv("OKASHI_DIR", dir)

	m := initialModel()

	if strings.Contains(m.status, "OKASHI_GRAMMALECTE_CMD") {
		t.Fatalf("status should not mention auto-launch when a backend is already available, got %q", m.status)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestInitialModelShowsHelpStatusWhenNotConfiguredAndUnavailable|TestInitialModelNoHelpStatusWhenBackendAvailable' -v`
Expected: FAIL — `m.status` doesn't yet mention `OKASHI_GRAMMALECTE_CMD`.

- [ ] **Step 3: Add the help status in `initialModel()`**

Dans `main.go`, étendre le bloc de lancement conditionnel ajouté en Task 2 Step 4 :

```go
	// Auto-launch: only when nothing answered above AND the user opted in via
	// OKASHI_GRAMMALECTE_CMD. A server that was already reachable is never a candidate
	// for launching — it isn't okashi's to own or later kill (see main()'s cleanup).
	var grammalecteProc *exec.Cmd
	var startupStatus string
	if !available {
		if cmdline, ok := grammalecteAutoLaunchCmd(); ok {
			if proc, err := launchGrammalecte(cmdline); err == nil {
				grammalecteProc = proc
			}
			// err != nil: silent, best-effort — grammarChecker and grammalecteProc both
			// stay nil, identical to today's "no server available" behavior.
		} else {
			startupStatus = "Grammalecte indisponible — configurez OKASHI_GRAMMALECTE_CMD pour un lancement automatique"
		}
	}
```

Dans le littéral `model{...}`, ajouter après `grammalecteProc: grammalecteProc,` :

```go
		grammalecteProc: grammalecteProc,
		status:          startupStatus,
```

**Attention** : `status: "",` apparaît déjà dans le littéral existant (ligne 575, `status: "",`) — le
remplacer par `status: startupStatus,` plutôt que d'ajouter une seconde clé `status` (Go refuserait la
compilation avec deux clés dupliquées dans un littéral de struct).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -run 'TestInitialModelShowsHelpStatusWhenNotConfiguredAndUnavailable|TestInitialModelNoHelpStatusWhenBackendAvailable' -v`
Expected: PASS

- [ ] **Step 5: Write the failing test for cleanup**

Ajouter dans `smoke_test.go` :

```go
func TestKillGrammalecteProcNilIsNoop(t *testing.T) {
	killGrammalecteProc(nil) // must not panic
}

func TestKillGrammalecteProcTerminatesProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start test process: %v", err)
	}
	killGrammalecteProc(cmd)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// process exited — success, regardless of the exact exit code/signal reported
	case <-time.After(3 * time.Second):
		t.Fatal("killGrammalecteProc did not terminate the process within 3s")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test . -run 'TestKillGrammalecteProc' -v`
Expected: FAIL — `killGrammalecteProc` undefined.

- [ ] **Step 7: Implement `killGrammalecteProc` and wire it into `main()`**

Ajouter dans `main.go`, juste avant `func main()` (avant la ligne 3018) :

```go
// killGrammalecteProc terminates a Grammalecte server subprocess that okashi itself launched.
// A nil proc (no auto-launch happened, or a pre-existing server was reused instead) is a no-op —
// okashi never touches a server it didn't start. SIGTERM is tried first; if the process hasn't
// exited within a second, Kill() (SIGKILL) forces it, so okashi never hangs on exit waiting for
// a misbehaving child.
func killGrammalecteProc(proc *exec.Cmd) {
	if proc == nil || proc.Process == nil {
		return
	}
	proc.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		proc.Process.Kill()
	}
}
```

Ajouter `"syscall"` aux imports de `main.go` s'il n'y est pas déjà.

Dans `main.go`, remplacer `func main()` (lignes 3018-3042) :

```go
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("okashi " + version)
			return
		case "--help", "-h", "help":
			fmt.Println(usage)
			return
		}
		// A positional path opens okashi in that folder, overriding $OKASHI_DIR.
		if dir, isDirArg, err := resolveDirArg(os.Args); isDirArg {
			if err != nil {
				fmt.Fprintln(os.Stderr, "okashi: "+err.Error())
				os.Exit(1)
			}
			os.Setenv("OKASHI_DIR", dir)
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
```

par :

```go
func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("okashi " + version)
			return
		case "--help", "-h", "help":
			fmt.Println(usage)
			return
		}
		// A positional path opens okashi in that folder, overriding $OKASHI_DIR.
		if dir, isDirArg, err := resolveDirArg(os.Args); isDirArg {
			if err != nil {
				fmt.Fprintln(os.Stderr, "okashi: "+err.Error())
				os.Exit(1)
			}
			os.Setenv("OKASHI_DIR", dir)
		}
	}

	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if fm, ok := finalModel.(model); ok {
		killGrammalecteProc(fm.grammalecteProc)
	}
	if err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test . -run 'TestKillGrammalecteProc' -v`
Expected: PASS

- [ ] **Step 9: Run the full test suite (regression check)**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add main.go smoke_test.go
git commit -m "grammalecte: message d'aide au démarrage et arrêt du sous-processus auto-lancé à la fermeture"
```

---

## Task 5: Vérification manuelle avec le vrai serveur Grammalecte

**Files:** aucun changement de code — vérification manuelle uniquement.

**Contexte** : la machine de développement a Grammalecte installé à `/home/Baudouin/Apps/` (script
`grammalecte-server.py`, confirmé fonctionnel par un test manuel pendant le brainstorming — port 8080,
endpoint `/gc_text/fr`, ~5-6s de démarrage). Cette tâche vérifie l'intégration bout-en-bout avec le
vrai process, pas seulement les fakes des tâches précédentes.

- [ ] **Step 1: Vérifier qu'aucun serveur Grammalecte ne tourne déjà**

Run: `pgrep -af grammalecte-server`
Expected: aucune sortie. Si un process traîne d'une session précédente, le tuer avant de continuer
(`kill <pid>`) pour tester le vrai chemin d'auto-lancement plutôt que le chemin "déjà disponible".

- [ ] **Step 2: Lancer okashi avec l'auto-lancement configuré**

Run (depuis la racine du repo) :
```bash
OKASHI_GRAMMALECTE_CMD="python3 /home/Baudouin/Apps/grammalecte-server.py" OKASHI_DIR=/tmp/okashi-verify go run .
```

Vérifier que l'écran d'accueil s'affiche **immédiatement**, sans délai perceptible d'attente des ~5-6s
de démarrage du serveur Python (confirme le non-bloquant, spec §2).

- [ ] **Step 3: Vérifier l'activation différée de la correction grammaticale**

Ouvrir un chapitre, aller dans l'inspector (onglet Analyse). Immédiatement après le démarrage, le nom
du backend peut ne pas encore apparaître (le serveur Python démarre encore). Attendre ~6-10 secondes,
rouvrir/rafraîchir l'inspector : le nom "Grammalecte" doit apparaître comme backend actif, sans avoir
eu besoin de relancer okashi.

- [ ] **Step 4: Vérifier une correction réelle**

Taper une phrase avec une faute grammaticale connue (ex. `Il es venu hier.`), déclencher la
vérification grammaticale (`ctrl+g` ou action de l'inspector selon le raccourci en place). Confirmer
qu'une remarque grammaticale apparaît (soulignement vert), cohérente avec le test manuel déjà effectué
pendant le brainstorming (`Il es venu` → suggestion `est`).

- [ ] **Step 5: Vérifier le nettoyage à la fermeture**

Quitter okashi normalement (`ctrl+c` ou la séquence de sortie habituelle). Puis :
```bash
pgrep -af grammalecte-server
```
Expected: aucune sortie — le process lancé par okashi a bien été terminé.

- [ ] **Step 6: Vérifier la réutilisation d'un serveur déjà actif**

Lancer manuellement le serveur d'abord :
```bash
cd /home/Baudouin/Apps && python3 grammalecte-server.py &
sleep 6
```
Puis lancer okashi avec la même variable `OKASHI_GRAMMALECTE_CMD` configurée. Vérifier que le backend
est actif dès l'ouverture de l'inspector (pas de délai d'attente supplémentaire — le serveur était
déjà là). Quitter okashi, puis vérifier que le serveur lancé manuellement **tourne toujours** :
```bash
pgrep -af grammalecte-server
```
Expected: le process est toujours présent (okashi ne doit jamais tuer un serveur qu'il n'a pas lancé
lui-même, spec §2.2/§4). Le tuer manuellement pour finir le nettoyage :
```bash
pkill -f grammalecte-server
```

- [ ] **Step 7: Vérifier le cas non configuré**

Lancer okashi sans `OKASHI_GRAMMALECTE_CMD` ni serveur actif :
```bash
OKASHI_DIR=/tmp/okashi-verify go run .
```
Vérifier que la barre de statut affiche le message d'aide mentionnant `OKASHI_GRAMMALECTE_CMD` au
démarrage, et qu'aucun process Python n'est lancé (`pgrep -af grammalecte-server` reste vide pendant
toute la session).

- [ ] **Step 8: Confirmer avec l'utilisateur**

Rapporter le résultat de la vérification manuelle à l'utilisateur avant de considérer la fonctionnalité
terminée.

---

## Hors périmètre (rappel, ne pas implémenter dans ce plan)

- Configuration par projet (Properties) — uniquement variable d'environnement globale.
- Redémarrage automatique si le serveur meurt en cours de session après un lancement réussi.
- Parsing shell complet pour `OKASHI_GRAMMALECTE_CMD` (`&&`, `|`, redirections, substitution de
  variables).
- Message d'erreur visible en cas d'échec du lancement au-delà du message d'aide générique du démarrage
  (pas de distinction "commande introuvable" vs "erreur Python" vs "timeout" affichée à l'utilisateur).
