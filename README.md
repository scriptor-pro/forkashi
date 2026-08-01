# forkashi

**forkashi** is a personal French-language fork of
[okashi](https://github.com/snackztime/okashi), a distraction-free
terminal writing app for long-form manuscripts. This fork adapts it for
French: a French spellchecking dictionary, French grammar checking via
[Grammalecte](https://www.grammalecte.net/), a project-wide aggregated
writing goal, and `.odt` export — see
[`docs/superpowers/specs/2026-08-01-forkashi-v1-design.md`](docs/superpowers/specs/2026-08-01-forkashi-v1-design.md)
for the full rationale. Everything else below describes the underlying
okashi app, whose interface stays in English in this fork for now.

**Write a whole book in your terminal, in plain Markdown.** okashi is a
distraction-free writing app for long-form manuscripts — split your work into
chapters it keeps in order, brainstorm in an outline, read the whole thing
through, and export an agent-ready `.docx`, `.odt`, or an elegant PDF. No
database, no proprietary bundle, no lock-in: it's just `.md` files in
ordinary folders.

For writers who live in the terminal (vim, Markdown, plain text) and want a real
manuscript workflow — ordered chapters, a corkboard, word-count goals, snapshots —
without leaving the keyboard or handing their prose to someone else's cloud.

Plain `.md` files · manuscript-aware sidebar · live word counts · outline &
corkboard · RTF / PDF / DOCX / ODT export · full-screen focus — all from the
command line.

<!-- Generate with `vhs demo.tape` (see demo.tape). -->
![okashi — opening a chapter and toggling the Tufte preview](docs/demo.gif)

---

## Install

forkashi is a personal fork with no prebuilt releases — build it from
source. It needs **Go 1.25+** (`go version` to check).

### Linux

Most distro package managers lag behind (Debian/Ubuntu stable, for
instance, ship Go 1.19–1.21). If `go version` reports something older than
1.25, install a current toolchain manually rather than waiting on your
distro:

```sh
# check what you have first
go version

# if it's too old, install a fresh Go 1.25.x under your home directory
# (no root needed) — see https://go.dev/dl/ for the current patch release
curl -LO https://go.dev/dl/go1.25.12.linux-amd64.tar.gz
tar -C "$HOME/.local" -xzf go1.25.12.linux-amd64.tar.gz
echo 'export PATH="$HOME/.local/go/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
go version   # should now report go1.25.12
```

Then build:

```sh
git clone https://github.com/scriptor-pro/forkashi
cd forkashi
make build        # or: go build -o forkashi .
./forkashi --version
```

`make build` (and plain `go build`) produce a `forkashi` binary in the
repo root — move it onto your `PATH` if you want it available everywhere:

```sh
sudo mv forkashi /usr/local/bin/
```

### macOS

```sh
git clone https://github.com/scriptor-pro/forkashi
cd forkashi
go build -o forkashi .
./forkashi --version
```

The `make apple` target (NSSpellChecker + Foundation Models grammar
backend, Xcode required) still builds, but in this fork the grammar
backend is wired to [Grammalecte](https://www.grammalecte.net/) regardless
of build tags — see `grammar_backend.go` — so `make apple` is not
currently the way to get grammar checking here.

### From source, either OS

```sh
go run .          # run without installing a binary
```

---

## Quick start

Below, `okashi` refers to whatever's on your `PATH` — that's the
`forkashi` binary you built above, whichever name you gave it:

```sh
okashi              # open the writing app
okashi --version    # print the version
okashi --help       # show help
```

okashi opens in your writing folder (see [Configuration](#configuration) for
where that is). The sidebar shows your documents and projects on the left; the
editor is centered on the right. Collapse the sidebar with `ctrl+b` for a
full-screen writing surface.

**Saving:** okashi autosaves as you write and shows a save indicator in the
status bar. Press `ctrl+s` to save explicitly at any time.

**Snapshots:** every file keeps a ring of timestamped backups in a `.okashi-bak/`
folder beside it. Select a file in the sidebar and press `b` to browse them —
preview any snapshot, take one on demand with `n`, or restore one with `⏎`
(your current version is backed up first, so a restore is never destructive).
Press `d` to **diff** the selected snapshot against the current file, or `D`
twice to diff two snapshots — a scrollable, color-coded view (removed red, added
green, changed words highlighted) so you can see exactly what a draft changed.

---

## Keyboard shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `ctrl+b` | Toggle sidebar |
| `ctrl+y` | Inspector tabs |
| `ctrl+l` | Outline (full-screen brainstorming — beats & notes) |
| `ctrl+k` | Corkboard (full-screen manuscript navigator); `c` from the sidebar |
| `ctrl+o` | Home (launch screen) |
| `esc` | Switch focus / back |
| `ctrl+c` | Quit |

### Files

| Key | Action |
|-----|--------|
| `ctrl+n` | New file (`+` new, right-click / F2 rename) |
| `r` | Rename file |
| `M` | Move file or folder |
| `del` | Delete file |
| `d` | Duplicate file |

### Writing

| Key | Action |
|-----|--------|
| `ctrl+s` | Save |
| `ctrl+t` | Typewriter scrolling (caret stays centered) |
| `ctrl+d` | Focus dim (dim everything outside the current sentence) |
| `ctrl+g` | Set goals |
| `ctrl+r` | Spelling suggestions |

### Export & preview

| Key | Action |
|-----|--------|
| `ctrl+e` | Export (RTF · PDF · DOCX · ODT) |
| `ctrl+p` | Markdown preview |
| `t` | Toggle Tufte view (inside preview) |

### Search

| Key | Action |
|-----|--------|
| `ctrl+f` | Search (Tab to scope · ctrl+a all sources) |

---

## Project model

The atom is one `.md` file. Larger structures are plain folders:

- **Manuscript** — a folder containing a `manifest.json`. The manifest is the
  sole source of order and display titles. Files listed in `items` are chapters;
  unlisted `.md` files are Resources (visible but not part of the ordered view or
  export). `ctrl+e` exports the whole manuscript.
- **Category** — a plain folder of unnumbered documents (no manifest). Good for
  loose notes, research, or reference material.
- **Resources** — `.md` files inside a manuscript folder that are not listed in
  `items`, or unnumbered files at the root or in a category.
- **Legacy manuscripts** — a folder with no manifest but at least one
  numerically-prefixed file (e.g. `01-opening.md`) is recognized for display
  only: order by numeric prefix, titles de-slugged from filenames. This is a
  read-only transitional view; no structural writes are offered.

### The corkboard (your manuscript navigator)

The left pane is a clean chapter list. In a manuscript, press `ctrl+k` (or `c`
from the sidebar) to open the **corkboard** — a full-screen spread of your
chapters, each a card with its word count and a **synopsis** (or its first line,
dimmed, until you write one). A header line shows the chapter count, total word
count, and — if you've set a project goal — your progress toward it; the card
you're currently editing is marked. On the corkboard:

- `⏎` open a chapter · `e` edit its synopsis · `J`/`K` reorder (staged; `esc`
  then `y` to apply, or `esc` to discard)
- `a` add / promote · `x` remove (demote to a resource) · `r` retitle
- `ctrl+e` export the whole manuscript · `esc` back

The read-through pager is `m` from the sidebar. The **outline** (`ctrl+l`) is a
separate free-form planning doc — jot beats and structure there before they
become chapters.

`ctrl+n` in a manuscript asks **chapter or resource** — a chapter is appended to
the manifest; a resource is an unlisted supporting doc, filed loosely or into a
folder (`Characters/Aldous`). Synopses live in a small `.okashi-synopsis.json`
beside your files; order lives in `manifest.json`.

### The outline (brainstorming before structure)

Press `ctrl+l` to open the full-screen **outline** — a place to think *before* you
have chapters. It's a plain `outline.md` of two levels: each top-level bullet is a
**beat**, its indented bullets are **notes**. You type freely (list-continuation
included); `alt+↑`/`alt+↓` move a whole beat (with its notes) up or down.

When a beat is ready to become a chapter, `alt+↵` **promotes** it: okashi creates
the chapter in the current manuscript, seeds its synopsis from the beat's notes,
and checks the beat off (`- [x] …`). It's a **one-way** bridge — the outline is
your brainstorm, not a live index, so reordering chapters later in the corkboard
never rewrites the outline. The outline is also visible read-only in the inspector
while you write.

**No lock-in.** Your work is just Markdown files in ordinary folders, with a
small human-readable `manifest.json` for order and titles — no database, no
proprietary bundle. Everything is `grep`-able, diff-able, and git-friendly, and
reads perfectly well in any other editor. okashi writes files atomically
(temp-file + rename) so a crash or a synced-folder conflict can't corrupt them.
Stop using okashi tomorrow and your manuscript is exactly where you left it.

---

## Export

Press `ctrl+e` to export. Choose a style:

| Key | Style | Description |
|-----|-------|-------------|
| `m` | Manuscript | Double-spaced manuscript format for agents/editors (submit the `.docx`) |
| `t` | Tufte | Elegant serif, for a readable or printable copy |

Both styles produce a `.rtf`, a `.pdf`, a `.docx`, and a `.odt`, written to
`<project>/export/`. (`.docx` is what most agents and editors ask for;
`.odt` opens directly in LibreOffice/OpenOffice Writer.)
When invoked from the outline, the full manuscript is exported (all chapters
concatenated). When invoked from the editor, only the current document is
exported.

A whole-manuscript Manuscript-style export opens with a standard title page:
your name and contact block (`OKASHI_AUTHOR` / `OKASHI_CONTACT`) top-left, an
approximate word count top-right, and the title centered below. Set
`OKASHI_AUTHOR` to also stamp your name into the running header. Single-chapter
exports skip the title page.

---

## Revision notes

Press `n` (with a file selected in the sidebar) to open **revision notes** for that
chapter — jot "cut the flashback", "check this date", "too slow here". Add with `a`, edit
with `e`, delete with `d`. Notes live in a small `.okashi-notes/` sidecar beside your files
and never touch the manuscript text, so they stay out of your prose and out of exports.

---

## Goals & pace

Set writing goals with `ctrl+g`: a daily word target, a project word target, daily
writing minutes, and an optional **deadline**. With a project target + deadline, the Goals
inspector tab shows a live **burndown** — "≈850/day to hit 80,000 by Mar 1" — that adjusts
as you write.

okashi keeps a per-project **word-count history**. The Goals tab shows a recent sparkline
and your current **streak**; press `g` (in the sidebar) to open the full **writing-history
heatmap** — a contributions-style grid of how much you wrote each day.

---

## French spelling & grammar (forkashi)

Spellcheck uses a French Hunspell dictionary
([Dicollecte](https://www.grammalecte.net/), "toutes variantes" — classic
and 1990-reform spellings both accepted) built into the binary; it works
out of the box.

Grammar checking calls a locally running
[Grammalecte](https://www.grammalecte.net/) server over HTTP
(`localhost:8080` by default; override with `OKASHI_GRAMMALECTE_HOST` /
`OKASHI_GRAMMALECTE_PORT`). Start it yourself before launching forkashi —
see [Grammalecte's own docs](https://www.grammalecte.net/) for the server
mode setup. If the server isn't running, forkashi just runs without
grammar checking rather than failing — check the "Check grammar" row in
the sidebar's inspector to see whether it connected.

---

## Preview

`ctrl+p` opens a rendered Markdown preview of the current document (powered by
[glamour](https://github.com/charmbracelet/glamour)). The preview is read-only;
`↑`/`↓` scroll, `ctrl+p` returns to editing.

Inside the preview, press `t` to toggle **Tufte view** — a book-style layout
that floats footnotes into **margin sidenotes** when the terminal is wide enough
to hold the text plus a right margin. The body stays at your writing measure; the
preview pane widens to accommodate the notes. On a narrower terminal, footnotes
fall back to numbered endnotes.

The preview theme follows your terminal background (dark or light). Override
with `OKASHI_THEME=dark` or `OKASHI_THEME=light`.

---

## Configuration

### Properties (in-app)

Press `i` on a project in the launch hub to open **Properties** — an editable screen for the
things you'd otherwise set via env vars:

- **Title** — the manuscript display title (written to `manifest.json`; manuscripts only).
- **Author** and **Contact** — your name and a multi-line contact block for the export title page,
  saved to a personal `config.json` in your OS config dir (macOS `~/Library/Application
  Support/okashi/`, Linux `~/.config/okashi/`) — set once, applies to every project.
- **Width** and **Smart quotes** — per-project editor preferences, saved to `<project>/.okashi.json`.

`⇥` moves between fields, `⏎` edits, `space` toggles, `ctrl+s` saves, `esc` backs out. Both stores
are plain JSON you can read or edit by hand.

### Environment variables

Each variable below is a **default** that the matching Properties field overrides when set. An
env-only setup keeps working unchanged.

| Variable | Default | Description |
|----------|---------|-------------|
| `OKASHI_DIR` | _(see below)_ | Override the writing folder — set this to point okashi anywhere |
| `OKASHI_WIDTH` | `72` | Editor column width, 20–200 (per-project override in Properties) |
| `OKASHI_SMARTQUOTES` | `on` | Smart curly quotes as you type; set `off`, `false`, or `0` to disable |
| `OKASHI_THEME` | _(auto)_ | Force `dark` or `light` for the Markdown preview |
| `OKASHI_ICONS` | _(auto)_ | Glyph set: `nerd` (Nerd Font glyphs), `plain` (Unicode only), or unset for auto-detect |
| `OKASHI_AUTHOR` | _(none)_ | Author name for the Manuscript running header + title page (editable in Properties) |
| `OKASHI_CONTACT` | _(none)_ | Free-text contact block for the Manuscript title page (editable in Properties) |

### Writing folder

okashi opens in a writing folder resolved in this order:

1. `$OKASHI_DIR` — set this to point okashi anywhere you like.
2. iCloud Drive — `~/Library/Mobile Documents/com~apple~CloudDocs/okashi`, when iCloud Drive is enabled.
3. `~/Documents/okashi` — cross-platform fallback (iCloud off, or Linux).

The folder is created on first run and seeded with a sample manuscript under `Demo/` (a
short piece, *The Lighthouse*) so you have something to explore — delete it whenever you
like. Until you create your own work, the launch screen shows a quick primer on okashi's
model (manuscript · category · notes).

---

## Text selection

okashi enables mouse reporting so the scroll wheel and click-to-focus work.
This suppresses the terminal's native drag-to-select. Two ways to select text:

- **Quick modifier drag** — hold **⌥ Option** (iTerm2 / Ghostty / Terminal.app)
  or **Shift** (most other terminals) and drag.
- **Selection mode** — press **`ctrl+x`** to release okashi's mouse capture so a
  **plain drag** selects; a `-- SELECT --` marker shows in the status bar. Press
  `ctrl+x` again to restore clicking.

Then **⌘C** (or your terminal's copy key) to copy.

---

## Terminals

okashi runs anywhere. The **outline and corkboard** — reordering, promoting,
navigating — all have keys that work on any terminal, including macOS
**Terminal.app**, where the **Option** key types accented characters instead of
acting as Meta:

- **Moving things** — reorder with `shift+↑/↓` (a chapter in the corkboard, a beat
  in the outline; `J/K` also works in the corkboard). **Promote** a beat with
  `ctrl+p`. The `alt+↑/↓` / `alt+↵` aliases are a convenience on Meta-capable
  terminals (iTerm2, Ghostty, kitty, WezTerm, Alacritty). To use them in
  Terminal.app: *Settings → Profiles → Keys → Left Option key → Esc+*.
- **Nerd Font glyphs** degrade to plain Unicode automatically on Terminal.app and
  the Linux console (override with `OKASHI_ICONS`).
- **Truecolor** renders best in a 24-bit terminal; through tmux, enable `RGB`/`Tc`.

---

## License

Code: MIT — see [LICENSE](LICENSE). Copyright (c) 2026 Michael Pentz
(original okashi); forkashi is a fork of that codebase under the same
license.

The bundled French dictionary (`assets/fr.aff`, `assets/fr.dic`) is
[Dicollecte](https://www.grammalecte.net/), licensed separately under
MPL 2.0 — see [`assets/fr.LICENSE`](assets/fr.LICENSE).
