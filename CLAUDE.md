# CLAUDE.md — okashi

Terminal writing app for long-form manuscripts and prose. Go + Bubble Tea (Elm-style
Model/Update/View) + lipgloss. Two real roles:

1. **A shipped product** — the CLI/agent face of a plain-`.md` writing corpus.
2. **The scope oracle** for a companion macOS app. The companion app matches okashi's
   feature set; okashi defines what "lean" means. The shared design + contracts reference
   lives in the companion app's repo.

> **Reconciled 2026-06-26** against the shipped okashi; **updated 2026-06-26** (manifest
> reconciliation — Tasks 1–6). An earlier draft described an aspirational architecture
> (gap-buffer editor, a manifest ordering file) that the codebase did not use; this version
> describes what okashi actually is, with all shared contracts resolved and adopted.

---

## Architecture invariants — DO NOT violate or "improve away"

### TUI stack
- Bubble Tea architecture: `Model` / `Update` / `View`. Side effects via `tea.Cmd`, never
  inline in `Update`. lipgloss for styling; `bubbles` for stock components.
- **The editor is a VENDORED `internal/textarea`** — a fork of `bubbles/textarea` that
  okashi owns (typewriter scrolling, focus dimming, configurable measure width, smart curly
  quotes, `MoveToLine`, …). Editor-core changes go in `internal/textarea`, not by swapping
  in stock `bubbles/textarea`.

### Performance model — how okashi stays instant on a 400-page work
The strategy is **split-into-files + windowed rendering**, NOT one giant buffer:
- A manuscript is many per-chapter `NN-title.md` files. **The editor loads ONE file at a
  time** (`loadFile` → `editor.SetValue`), so it never holds a 400-page buffer — each
  editing surface is chapter-sized.
- **`View()` MUST stay O(visible).** Render only the visible window. The read-through
  **manuscript pager** (`pager.go`) builds its wrapped line→source map **once** on open and
  renders only `lines[offset:offset+height]`; the file pane and outline do likewise. The
  **editor** (`internal/textarea`) `View()` is also windowed — it renders only the visible
  rows (tracks an explicit scroll `offset`; `locateRow`/`displayHeight` map display rows to
  source via the buffer-sized wrap cache), so even a large *single* file edits smoothly (a
  400-page file went 51 ms → ~2 ms/frame). Never stringify a whole document every frame —
  Bubble Tea diffs the entire `View()` output, so a giant string is the performance killer.
- A true gap-buffer editor core is **not** built and not needed: the bottleneck was *render*
  cost (styling every line), fixed by windowing `View()`; a gap buffer addresses *edit* cost,
  which never dominated. A prefix-sum wrap-height index (to make the editor `View()` perfectly
  flat vs size, vs the current cheap residual cached-wrap walks) is a possible future
  optimization, deferred as YAGNI.
- Keep lipgloss **out of the hot path** — pre-style static chrome; don't restyle per cell
  per frame.

### Files & sync — okashi's signature obligation
- **RULE (ADOPTED): write atomically (temp file + rename**, atomic on the same volume);
  never write in place. *Status:* `save()`, export, and the recents store all write atomically
  via `atomicWrite` (`atomicwrite.go`). okashi runs outside the macOS sandbox and **cannot use
  `NSFileCoordinator`** (the mechanism the app uses to coordinate with the iCloud daemon), so
  atomic writes + iCloud `NSFileVersion` are what keep the shared corpus from corrupting.
- Dotfiles (names starting with `.`) are excluded from the pane and from manuscript
  detection. Atomic-write temp files are dot-prefixed so they never appear as documents while
  in flight.

### Theming
- lipgloss styling; the theme is detected once at startup (override with `OKASHI_THEME`),
  rendered as truecolor where the terminal supports it (degrades to 256). ⚠️ Truecolor
  through tmux needs `RGB`/`Tc` enabled.
- A **shared semantic-role theme JSON** with the app (see Shared Contracts §3) is
  aspirational — okashi does not read a shared theme file today.

---

## Project model (the shipped reality)

- **The atom is one `.md` file.**
- **Manuscript** = a folder containing a `manifest.json`. The manifest is the sole source of
  order (`items` array), chapter membership (listed in `items` = chapter; unlisted `.md` =
  Resource), and display titles (`items[].title`, `manifest.title`). okashi reads it **and
  writes it** — create + chapter-title retitle, and **structure mode** (reorder / insert / remove
  chapters, confirm-on-commit); cross-container **move** via the general **file mover** (`M`) is planned (see Shared
  Contracts §1).
  - **Legacy fallback:** a folder with **no** manifest but ≥1 numerically-prefixed file is
    treated as a manuscript for display only — order = numeric prefix, titles = de-slugged
    filename. A read-only transitional courtesy for un-migrated corpora.
- **Category** = a plain folder of unnumbered docs (no manifest, no numbered files).
- **Loose / "Resources"** = unnumbered files at the root, in a category, or inside a
  manuscript folder but not listed in `items`; shown, excluded from the ordered view and
  from export.
- Shipped features: the launch hub; a manuscript-aware sidebar (titles + per-chapter word
  counts); the **outline** (`ctrl+l`: a full-screen **editor-first** planning surface — two-level
  **beats + notes** in `outline.md`; `shift/alt+↑↓` move a beat, `ctrl+p`/`alt+↵` **promote** a beat
  → chapter [seeds the synopsis from its notes, marks it `[x]`], one-way; the `outline.md` format is
  a shared contract with the companion app — see Shared Contracts §4); the **pager** (`m`: read-through with jump-to-edit); **export**
  (`ctrl+e`: RTF + PDF, Manuscript or Tufte style); **all-texts export-selection screen** (`v` from the sidebar:
  full-screen list of every chapter, scene, standalone scene, and Resource with an export-inclusion checkbox, word/character
  totals, and a 111-character preview per entry; `space` toggles inclusion, `shift+↑↓` moves an entry within its container
  or across container boundaries, converting independent resources ↔ standalone scenes as needed; the selection state
  persists in a per-project `.okashi-export.json` sidecar; **Resources can now appear in an export when checked on this screen**
  — previously impossible); **rename** (`r`: manifest chapters
  **retitle** the `items[].title` — filename stays birth-stable; legacy numbered chapters,
  loose files, Resources, and folders rename on disk); markdown **preview** (`ctrl+p`,
  glamour); **Properties** (`i` from the hub: edit title / author / contact / width /
  smartquotes — see below); **Snapshots** (`b` from the sidebar: browse / preview / restore /
  **diff** a file's `.okashi-bak/` timestamped backups; `n` on demand, `d`/`D` diff, restore backs
  up the current version first); **pace** (`ctrl+g` deadline → Goals-tab burndown; per-project word
  history → `g` heatmap); **corkboard** (`c` from the binder: per-chapter synopses in a card view +
  reorder — the corkboard is the **manuscript navigator**: `ctrl+k` toggles the left pane
  between the chapter list and corkboard cards (title + word count + synopsis, first-line
  fallback); sidebar `⏎` open · `e` synopsis · `J/K` staged reorder (confirm on exit) · `c`
  full-screen spread (`a` add/promote · `x` demote · `r` retitle · `⏎` open · `ctrl+e` whole-
  manuscript export) · `m` pager; `ctrl+n` in a manuscript → chapter|resource|scene picker (resource loose or into a
  subfolder); **scene is always offered**: when the sidebar selection is a chapter, it appends a scene to that
  chapter's ordered texts; when no chapter is selected, it creates a **standalone scene** (a top-level ordered item
  at the manuscript root, no containing folder); the text-picker screen (opened on a multi-scene or empty chapter)
  also creates a scene via `ctrl+n`. Synopses in an okashi-owned `.okashi-synopsis.json` sidecar, NOT
  the manifest; the pop-down binder + standalone structure modal are retired); **revision notes** (`n` from the sidebar: per-file chapter notes —
  add/edit/delete; okashi-owned `.okashi-notes/<base>.json` sidecar, follows rename/delete;
  **v1 = chapter-scoped**, line/sentence anchoring + editor-gutter floating is planned v2).
- **Properties** (`i` on a hub project): okashi's one *editable* metadata surface. Personal
  identity (author, contact) → global `config.json` in the OS user-config dir (`os.UserConfigDir()`,
  alongside `recent.json`; macOS `~/Library/Application Support/okashi`); per-project width +
  smartquotes → `<project>/.okashi.json`; title → `manifest.json`. Effective value resolves
  **file → env → default** per field (`resolveSettings` in `settings.go`), so `OKASHI_*` stays a
  working default. **No manifest schema change** — the shared-contract HARD GATE is untriggered.
  Properties also edits **PDF Manuscript-style typography** (2026-08-26): font (Courier/Times —
  Times is an fpdf core font, no embedded TTF), line height, and either characters-per-line
  (Courier, fixed-pitch — derives the left/right margins) or 4 direct margins (Times,
  variable-pitch, no meaningful CPL), all per-project in `.okashi.json` (**file → default** only;
  no `OKASHI_*` env knob was added for these). Applies **only** to the PDF export's Manuscript
  style — the Tufte style (ET Book, fixed 108/90/108 margins) is unaffected. RTF/DOCX/ODT/EPUB
  are unaffected.
- Env knobs (all now **defaults** overridable in Properties where a field exists): `OKASHI_DIR`,
  `OKASHI_WIDTH`, `OKASHI_SMARTQUOTES`, `OKASHI_THEME`,
  `OKASHI_ICONS` (`nerd`/`plain`/`auto`; unset = auto — Nerd Font glyphs except on
  Terminal.app / Linux VT console, which get plain glyphs since the font isn't patched),
  `OKASHI_AUTHOR` (export header + title page), `OKASHI_CONTACT` (title-page contact block).

### Scope discipline
- **okashi defines the lean feature set — keep it lean.** New surface area here propagates to
  the app. When a change would expand scope, surface it and ask.
- No tag system / smart folders / PKM. No sub-document "sheets" (granularity = how files are
  split).

---

## ⚠️ SHARED CONTRACTS — keep aligned with the companion app

okashi and the companion macOS app operate the **same on-disk corpus**. Keep this block aligned
with the companion app's copy.

### 1. Manuscript ordering & membership — RESOLVED (2026-06-26)
- **RESOLVED (2026-06-26); okashi became a writer (2026-06-30):** order, membership, and display
  titles live in the shared per-manuscript `manifest.json` (see the companion app's storage-spine
  design doc, §2.1 and §6). okashi
  **reads and writes** it (create + chapter-title retitle, and **structure mode** reorder / insert /
  remove with a commit confirm; cross-container move via the export-selection screen is shipped, 2026-08-23):
  - **Manifest manuscript** (folder with `manifest.json`): `items` order is canonical;
    `items[].title` is the chapter display title; `manifest.title` is the manuscript title;
    a file is a chapter **iff** it is listed in `items`; any unlisted `.md` is a Resource.
    okashi reads this **and writes it** — it creates manuscripts and retitles `items[].title`
    (no-confirm; filename birth-stable), and **structure mode** reorders / inserts / removes
    chapters (`s` from the binder; staged, committed on exit behind one confirm). Cross-container
    **move** (into/out of a manuscript) via the general **file mover** (`M`) is still planned. If the manifest is
    unreadable or its `schemaVersion` is unsupported,
    okashi refuses to infer structure — it shows files flat as loose documents with a status
    note; it does **not** fall back to prefix ordering.
  - **Legacy manuscript** (no manifest, ≥1 numerically-prefixed file): filename-prefix
    convention is a **read-only display fallback** — order by numeric prefix, titles
    de-slugged from filenames. A transitional courtesy for un-migrated corpora; no
    structural writes offered here either.
  - **Category** (neither manifest nor numbered files): plain folder of documents.
- **Authority (revised 2026-07-01; export-selection cross-container moves shipped 2026-08-23):** **both apps write the shared manifest.** okashi creates
  manuscripts (New Project) and retitles chapter display titles (`r` on a manifest chapter →
  `items[].title`, no-confirm); **structure mode** (SHIPPED) reorders / inserts / removes chapters
  behind a commit **confirmation** (`s` from the binder), mirroring the companion app's own confirm sheet.
  Cross-container **move** via the **export-selection screen** (`v` → `space`/`shift+↑↓` to move texts across chapters or
  convert between Resources and standalone scenes) rewrites `manifest.json` using the existing v3 shape (retrofit `Texts[]`/`items[]`
  entries per the v3 schema, no new field, no schema-shape change). The general **file mover** (`M`, a separate cross-container mechanism) is still planned. The companion app owns
  the app-side structural writers (`ManuscriptStore.reorder`/`.move`/insert).
  Safety for the shared corpus = atomic writes + `NSFileVersion`; each writer read-modify-writes.
  `r` on a legacy (manifest-less) numbered chapter still does a prefix-preserving file rename (O1).
- **HARD GATE (standing):** any change to the manifest **shape** (schema, field set,
  serialization) must STOP, confirm with the user, and implement in **both** repos together.
  okashi **writes v1-shaped manifests** (allowed); the gate is about changing the shared schema
  *shape*, not about writing data.
- **forkashi note (2026-08-23):** this fork has no known companion macOS app consuming
  `manifest.json` (only `origin`/scriptor-pro/forkashi and `upstream`/snackztime/okashi
  remotes exist), so the HARD GATE requiring cross-repo manifest-shape coordination does not
  apply here. `manifestSchemaVersion` was advanced from 2 to 3 on 2026-08-23 with the addition
  of `manifestChapter.Scene` (a boolean flag marking a scene as **standalone**: a top-level
  ordered item with no containing chapter folder). This is a **deliberate divergence** from the
  upstream okashi schema.

### 2. Markdown flavor — HARD GATE (ADOPTED)
- Flavor = **CommonMark + GFM (tables, task lists, strikethrough, autolinks) + footnotes**,
  via `goldmark` with the matching extensions. **Footnotes must be enabled** to match the
  app's `swift-markdown`/cmark-gfm config.
- *Status:* the **export** parser (`export_ast.go`) uses `goldmark` + **GFM + Footnote**
  extensions (matching the app); footnotes export as per-chapter endnotes; the rarer GFM
  constructs degrade (tables → pipe rows, strikethrough → plain, task lists → `[ ]`/`[x]`).
  The `ctrl+p` **preview** uses glamour, which renders GFM but exposes no hook to add the
  footnote extension — so footnote syntax shows literally in the preview (known limitation).
- Before changing the supported flavor/extension set: STOP, confirm, implement in BOTH
  codebases together. No proprietary syntax. Pin `goldmark` + extension versions.

### 3. Theme JSON — LIGHT (cosmetic, no gate)
- Themes are **semantic-role JSON** (`background, foreground, heading, emphasis, code, link,
  selection, gutter`), read by both apps. *Aspirational for okashi* until it reads a shared
  theme file; today it detects a theme itself. If adopted, keep the role set in sync (adding a
  role updates both apps and all theme files together).

### 4. Outline (`outline.md`) — HARD GATE on the grammar (ADOPTED 2026-07-07)
- **Shared format, different editors.** A per-manuscript **`<project>/outline.md`** (plain Markdown) is a
  two-level planning surface both apps read/write: a **beat** = a top-level list item (`-`/`*`/`+` + space,
  indent 0); a `[x]`/`[X]` box marks the beat **promoted**; **notes** = the beat block's non-beat lines
  (de-bulleted + de-indented, blanks dropped); text before the first beat (preamble) is ignored (v1).
- **Promote is one-way:** create a chapter (title = beat text), seed its synopsis from the beat's notes, mark
  the beat `[x]`. No back-sync — reordering chapters never rewrites the outline.
- **Parity + an accepted asymmetry.** okashi's `outline_parse.go` and the app's `OutlineParser` are
  line-for-line equivalent on the grammar. okashi edits the raw Markdown (**preserves** everything — headings,
  blank lines, `*`/`+` markers, custom note indentation); the app parses → beats → **serializes** (regenerates
  the file to `- title` / `- [x] title` + 2-space `  - note`), so okashi-only freeform extras are **normalized
  away on an app save**. Beats + notes + promoted state round-trip both ways; that normalization asymmetry is
  **accepted** (it's not data loss of the outline itself).
- **HARD GATE:** any change to the beat/note **grammar** (marker set, the promoted box, note nesting / `>2`
  levels, preamble handling, or the promote side-effects) is a shared-format change — STOP, confirm, implement
  in **both** (`outline_parse.go` ⇄ `OutlineParser`). Routine outline edits are normal runtime ops.

*(End of mirrored block.)*

---

## Build & test

- Module: `okashi` (flat `package main` at the repo root + `internal/textarea`). Go `1.25`
  (raised by the pure-Go PDF dep). **On this dev machine `go` is not on PATH — invoke it as
  `/opt/homebrew/bin/go`** (and `gofmt` likewise).
- Build: `go build ./...`  ·  Run: `go run .`  ·  Test: `go test ./...`  ·  Vet: `go vet ./...`
- **The default build is PURE-GO** (no cgo). An OPTIONAL on-device grammar backend (Tier 2:
  NSSpellChecker + Apple Intelligence/Foundation Models) lives behind `//go:build darwin && cgo
  && applegrammar` — `grammar_apple.{m,h}`, `grammar_apple_fm.swift`, `grammar_apple_darwin.go`;
  a pure-Go stub (`grammar_apple_stub.go`) keeps the default build cgo-free. Build it with
  `make apple` (compiles the Swift static lib + `go build -tags applegrammar`); macOS + Xcode
  only. NEVER let cgo leak into the default build — everything routes through the `grammarChecker`
  interface + the `newGrammarChecker` var. Distribution: pure-Go bottle everywhere + an
  Apple-silicon bottle (Foundation Models gates at runtime via `okashi_fm_available`).
  **forkashi note:** this fork sets `newGrammarChecker = grammalecteChecker` (untagged) in
  `grammar_backend.go`, so the Grammalecte HTTP backend is the default grammar checker even
  on an `applegrammar` build — the Tier 2 Apple backend described above is bypassed in favor
  of French-language support via Grammalecte (see
  `docs/superpowers/specs/2026-08-01-forkashi-v1-design.md` §3). The Apple backend still
  exists and is still tested, it's just not wired into `initialModel()` in this fork.
- Key deps (all pure Go): `bubbletea`, `bubbles`, `lipgloss`, `glamour`, `x/ansi`,
  `yuin/goldmark` (parsing — shared-contract governed, pin it), `codeberg.org/go-pdf/fpdf`
  (PDF export — the maintained fork, NOT the archived `github.com/go-pdf/fpdf`), `x/text`
  (cp1252 transcode for the manuscript PDF). ET Book TTFs are embedded under `assets/etbook/`
  for the Tufte PDF.
- Design/plan history lives under `docs/superpowers/{specs,plans}/`.

---

## Working agreement

- okashi is **lean by design** and is the oracle the app matches — bias toward restraint; ask
  before expanding scope.
- When a task would touch a **shared contract** (§1–§3) or the **atomic-write rule**, surface
  it and confirm rather than proceeding.
- The shared design reference is the app repo's `SPEC.md`; this file is okashi's operational
  rule set.
- Adopted & shipped: **atomic writes** (pending earlier, now in `save()` + export), **GFM +
  footnotes** in the export parser (shipped with Tasks 1–3 of the 2026-06-22 export refactor),
  and **okashi as a manifest writer** (create + chapter-title retitle; 2026-06-30, shared-contract
  change mirrored in the companion app).
