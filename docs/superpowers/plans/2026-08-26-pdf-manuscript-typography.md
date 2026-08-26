# Mise en page PDF configurable (style Manuscript) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user pick Times as an alternative to Courier for the PDF Manuscript
style, and configure line height, characters-per-line (Courier) or 4 margins (Times) —
editable per-project in Properties (`i`), persisted in `.okashi.json`.

**Architecture:** Extend `projectSettings`/`effectiveSettings` (the existing
defaults←env←file resolution in `settings.go`) with 7 new pointer fields. Thread the
resolved values through `Meta` (already the per-export settings carrier passed to all 5
format writers) into `writePDF`, which uses them only on the Manuscript branch — the
Tufte branch is untouched. Extend the Properties screen (`properties.go`) with 3–7
conditional fields (font toggle, line height, then either chars-per-line or 4 margins
depending on the selected font), following the exact pattern already used for
`width`/`smartquotes`/`cover`.

**Tech Stack:** Go, `codeberg.org/go-pdf/fpdf` (PDF core fonts Courier/Times — no new TTF
embedding), `bubbles/textinput`, `encoding/json`.

**Spec:** `docs/superpowers/specs/2026-08-26-pdf-manuscript-typography-design.md`

## Global Constraints

- PDF format only. RTF/DOCX/ODT/EPUB writers are not touched.
- Manuscript style only. Tufte (`StyleTufte`) branch in `writePDF` must remain
  byte-for-byte behaviorally unchanged.
- A project with no `.okashi.json` (or none of these 7 fields set) must render an
  **identical** PDF to today's output — defaults reproduce current constants exactly
  (Courier, line height 24, margins 72/72/72/72).
- Default chars-per-line is **62** — computed from `fpdf.GetStringWidth("0")` on Courier
  12pt with the current 72pt side margins on A4 (595.28pt wide), floored so the derived
  margin never comes in narrower than today's 72pt. This is frozen as a Go constant, not
  recomputed at runtime.
- Validation bounds (reject + revert to prior value on invalid, mirroring the existing
  `propWidth` pattern): line height `[10, 40]`, chars-per-line `[40, 120]`, each margin
  `[20, 200]`.
- `PdfFont` is a plain `string` (`"courier"` | `"times"`), not a closed Go enum — keeps
  the door open for more fonts later without a schema change (per spec's noted future
  extensibility).
- All new `.okashi.json` fields are pointers (nil = unset → falls through to default),
  matching `Width *int` / `Smartquotes *bool` / `Cover *string`.
- Writes stay atomic via the existing `atomicWrite` — no new write path introduced.

---

## File Structure

- **Modify `settings.go`**: add 7 fields to `projectSettings`, 7 resolved fields to
  `effectiveSettings`, defaults + clamping in `mergeSettings`, new clamp helpers.
- **Modify `export_ast.go`**: add the 7 resolved PDF fields to `Meta` (the carrier struct
  already passed to every format writer).
- **Modify `export.go`**: `runExport` populates the new `Meta` fields from
  `resolveSettings(dir)` (`eff` is already computed there).
- **Modify `export_pdf.go`**: `writePDF`'s Manuscript branch reads font/line-height/margins
  from `meta` instead of the hardcoded `pdfStyle{...}` literal; add a `resolvePdfFont`
  helper (name → fpdf font name, isFixedPitch) and a `resolveManuscriptMargins` helper
  (CPL → left/right margin for Courier, direct pass-through for Times). The two `"Courier"`
  literals in the running header and the title page also switch to the resolved font name.
- **Modify `properties.go`**: add 7 `propKind` constants, 7 form fields on
  `propertiesModel`, conditional field-list rebuild on font toggle, dirty-tracking, save
  logic, view rendering.
- **Test files**: extend `settings_test.go`, `export_pdf_test.go`, `properties_test.go`
  (no new test files — each extends an existing suite for the file it tests).

---

## Task 1: `projectSettings` / `effectiveSettings` — data model and resolution

**Files:**
- Modify: `settings.go`
- Test: `settings_test.go`

**Interfaces:**
- Produces: `projectSettings.PdfFont *string`, `.PdfLineHeight *float64`,
  `.PdfCharsPerLine *int`, `.PdfMarginTop/.PdfMarginBottom/.PdfMarginLeft/.PdfMarginRight
  *float64`; `effectiveSettings.PdfFont string`, `.PdfLineHeight float64`,
  `.PdfCharsPerLine int`, `.PdfMarginTop/.PdfMarginBottom/.PdfMarginLeft/.PdfMarginRight
  float64`; constants `defaultPdfFont = "courier"`, `defaultPdfLineHeight = 24.0`,
  `defaultPdfCharsPerLine = 62`, `defaultPdfMargin = 72.0`; helpers `clampPdfLineHeight(f
  float64) float64`, `clampPdfCharsPerLine(n int) int`, `clampPdfMargin(f float64)
  float64`.

- [ ] **Step 1: Write the failing tests**

Append to `settings_test.go`:

```go
func TestMergeSettingsPdfDefaults(t *testing.T) {
	eff := mergeSettings(userConfig{}, projectSettings{})
	if eff.PdfFont != "courier" {
		t.Errorf("PdfFont default = %q, want %q", eff.PdfFont, "courier")
	}
	if eff.PdfLineHeight != 24 {
		t.Errorf("PdfLineHeight default = %v, want 24", eff.PdfLineHeight)
	}
	if eff.PdfCharsPerLine != 62 {
		t.Errorf("PdfCharsPerLine default = %v, want 62", eff.PdfCharsPerLine)
	}
	for name, got := range map[string]float64{
		"top": eff.PdfMarginTop, "bottom": eff.PdfMarginBottom,
		"left": eff.PdfMarginLeft, "right": eff.PdfMarginRight,
	} {
		if got != 72 {
			t.Errorf("PdfMargin%s default = %v, want 72", name, got)
		}
	}
}

func TestMergeSettingsPdfFileOverrides(t *testing.T) {
	font := "times"
	lh := 20.0
	top, bottom, left, right := 50.0, 60.0, 80.0, 90.0
	eff := mergeSettings(userConfig{}, projectSettings{
		PdfFont: &font, PdfLineHeight: &lh,
		PdfMarginTop: &top, PdfMarginBottom: &bottom, PdfMarginLeft: &left, PdfMarginRight: &right,
	})
	if eff.PdfFont != "times" || eff.PdfLineHeight != 20 {
		t.Fatalf("font/line height override: %+v", eff)
	}
	if eff.PdfMarginTop != 50 || eff.PdfMarginBottom != 60 || eff.PdfMarginLeft != 80 || eff.PdfMarginRight != 90 {
		t.Fatalf("margin override: %+v", eff)
	}
}

func TestMergeSettingsPdfCharsPerLineOverride(t *testing.T) {
	cpl := 80
	eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cpl})
	if eff.PdfCharsPerLine != 80 {
		t.Fatalf("PdfCharsPerLine override: %+v", eff)
	}
}

func TestMergeSettingsPdfClamping(t *testing.T) {
	lhLow, lhHigh := 1.0, 999.0
	if eff := mergeSettings(userConfig{}, projectSettings{PdfLineHeight: &lhLow}); eff.PdfLineHeight != 10 {
		t.Errorf("line height low clamp = %v, want 10", eff.PdfLineHeight)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfLineHeight: &lhHigh}); eff.PdfLineHeight != 40 {
		t.Errorf("line height high clamp = %v, want 40", eff.PdfLineHeight)
	}

	cplLow, cplHigh := 1, 999
	if eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cplLow}); eff.PdfCharsPerLine != 40 {
		t.Errorf("cpl low clamp = %v, want 40", eff.PdfCharsPerLine)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfCharsPerLine: &cplHigh}); eff.PdfCharsPerLine != 120 {
		t.Errorf("cpl high clamp = %v, want 120", eff.PdfCharsPerLine)
	}

	mLow, mHigh := 1.0, 999.0
	if eff := mergeSettings(userConfig{}, projectSettings{PdfMarginTop: &mLow}); eff.PdfMarginTop != 20 {
		t.Errorf("margin low clamp = %v, want 20", eff.PdfMarginTop)
	}
	if eff := mergeSettings(userConfig{}, projectSettings{PdfMarginTop: &mHigh}); eff.PdfMarginTop != 200 {
		t.Errorf("margin high clamp = %v, want 200", eff.PdfMarginTop)
	}
}

func TestProjectSettingsPdfRoundTrip(t *testing.T) {
	dir := t.TempDir()
	font := "times"
	lh := 22.0
	cpl := 70
	top, bottom, left, right := 60.0, 60.0, 90.0, 90.0
	ps := projectSettings{
		PdfFont: &font, PdfLineHeight: &lh, PdfCharsPerLine: &cpl,
		PdfMarginTop: &top, PdfMarginBottom: &bottom, PdfMarginLeft: &left, PdfMarginRight: &right,
	}
	if err := saveProjectSettings(dir, ps); err != nil {
		t.Fatal(err)
	}
	got := loadProjectSettings(dir)
	if got.PdfFont == nil || *got.PdfFont != "times" {
		t.Fatalf("PdfFont round-trip: %+v", got)
	}
	if got.PdfLineHeight == nil || *got.PdfLineHeight != 22 {
		t.Fatalf("PdfLineHeight round-trip: %+v", got)
	}
	if got.PdfCharsPerLine == nil || *got.PdfCharsPerLine != 70 {
		t.Fatalf("PdfCharsPerLine round-trip: %+v", got)
	}
	if got.PdfMarginLeft == nil || *got.PdfMarginLeft != 90 {
		t.Fatalf("PdfMarginLeft round-trip: %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestMergeSettingsPdf|TestProjectSettingsPdfRoundTrip' -v`
Expected: FAIL — `projectSettings`/`effectiveSettings` have no `PdfFont` field (compile
error).

- [ ] **Step 3: Extend the structs and resolution logic**

In `settings.go`, add to `projectSettings` (after `Cover`):

```go
	// PDF Manuscript-style typography (Manuscript only; Tufte is unaffected). All nil =
	// today's hardcoded defaults, so an unconfigured project renders identically.
	PdfFont         *string  `json:"pdfFont,omitempty"`         // "courier" | "times"
	PdfLineHeight   *float64 `json:"pdfLineHeight,omitempty"`   // points
	PdfCharsPerLine *int     `json:"pdfCharsPerLine,omitempty"` // Courier only; ignored for Times
	PdfMarginTop    *float64 `json:"pdfMarginTop,omitempty"`    // points
	PdfMarginBottom *float64 `json:"pdfMarginBottom,omitempty"` // points
	PdfMarginLeft   *float64 `json:"pdfMarginLeft,omitempty"`   // points; ignored for Courier (derived from CPL)
	PdfMarginRight  *float64 `json:"pdfMarginRight,omitempty"`  // points; ignored for Courier (derived from CPL)
```

Add to `effectiveSettings` (after `Cover`):

```go
	PdfFont                       string
	PdfLineHeight                 float64
	PdfCharsPerLine                int
	PdfMarginTop, PdfMarginBottom float64
	PdfMarginLeft, PdfMarginRight float64
```

Add constants near `defaultColumnWidth` in `main.go`... actually keep them local to
`settings.go` since they're PDF-settings-specific, right above `mergeSettings`:

```go
const (
	defaultPdfFont         = "courier"
	defaultPdfLineHeight   = 24.0
	defaultPdfCharsPerLine = 62
	defaultPdfMargin       = 72.0
)

// clampPdfLineHeight constrains PDF line height to the supported [10,40]pt range.
func clampPdfLineHeight(f float64) float64 {
	if f < 10 {
		return 10
	}
	if f > 40 {
		return 40
	}
	return f
}

// clampPdfCharsPerLine constrains chars-per-line to the supported [40,120] range.
func clampPdfCharsPerLine(n int) int {
	if n < 40 {
		return 40
	}
	if n > 120 {
		return 120
	}
	return n
}

// clampPdfMargin constrains a PDF margin to the supported [20,200]pt range.
func clampPdfMargin(f float64) float64 {
	if f < 20 {
		return 20
	}
	if f > 200 {
		return 200
	}
	return f
}
```

In `mergeSettings`, set defaults in the initial `eff := effectiveSettings{...}` literal
and overlay file values, following the existing `Width`/`Smartquotes` pattern:

```go
func mergeSettings(uc userConfig, ps projectSettings) effectiveSettings {
	eff := effectiveSettings{
		Author:      os.Getenv("OKASHI_AUTHOR"),
		Contact:     os.Getenv("OKASHI_CONTACT"),
		Width:       defaultColumnWidth,
		Smartquotes: true,

		PdfFont:         defaultPdfFont,
		PdfLineHeight:   defaultPdfLineHeight,
		PdfCharsPerLine: defaultPdfCharsPerLine,
		PdfMarginTop:    defaultPdfMargin,
		PdfMarginBottom: defaultPdfMargin,
		PdfMarginLeft:   defaultPdfMargin,
		PdfMarginRight:  defaultPdfMargin,
	}
	if w, ok := resolveColumnWidthEnv(); ok {
		eff.Width = w
	}
	if sq, ok := resolveSmartQuotesEnv(); ok {
		eff.Smartquotes = sq
	}
	if uc.Author != "" {
		eff.Author = uc.Author
	}
	if uc.Contact != "" {
		eff.Contact = uc.Contact
	}
	if ps.Width != nil {
		eff.Width = clampWidth(*ps.Width)
	}
	if ps.Smartquotes != nil {
		eff.Smartquotes = *ps.Smartquotes
	}
	if ps.Cover != nil {
		eff.Cover = *ps.Cover
	}
	if ps.PdfFont != nil {
		eff.PdfFont = *ps.PdfFont
	}
	if ps.PdfLineHeight != nil {
		eff.PdfLineHeight = clampPdfLineHeight(*ps.PdfLineHeight)
	}
	if ps.PdfCharsPerLine != nil {
		eff.PdfCharsPerLine = clampPdfCharsPerLine(*ps.PdfCharsPerLine)
	}
	if ps.PdfMarginTop != nil {
		eff.PdfMarginTop = clampPdfMargin(*ps.PdfMarginTop)
	}
	if ps.PdfMarginBottom != nil {
		eff.PdfMarginBottom = clampPdfMargin(*ps.PdfMarginBottom)
	}
	if ps.PdfMarginLeft != nil {
		eff.PdfMarginLeft = clampPdfMargin(*ps.PdfMarginLeft)
	}
	if ps.PdfMarginRight != nil {
		eff.PdfMarginRight = clampPdfMargin(*ps.PdfMarginRight)
	}
	return eff
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestMergeSettingsPdf|TestProjectSettingsPdfRoundTrip' -v`
Expected: PASS

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all packages PASS (this task only adds fields/defaults — no existing behavior
changes yet).

- [ ] **Step 6: Commit**

```bash
git add settings.go settings_test.go
git commit -m "settings: add PDF Manuscript typography fields (font, line height, CPL, margins)"
```

---

## Task 2: `Meta` carries the resolved PDF fields; `runExport` populates them

**Files:**
- Modify: `export_ast.go` (the `Meta` struct)
- Modify: `export.go` (`runExport`)
- Test: `export_wiring_test.go` (or wherever `runExport`/`Meta` construction is already
  tested — verify with `grep -n "Meta{" export_wiring_test.go export_ast_test.go` before
  writing; add to whichever file already covers `Meta` construction)

**Interfaces:**
- Consumes: `effectiveSettings.PdfFont/.PdfLineHeight/.PdfCharsPerLine/.PdfMarginTop/
  .PdfMarginBottom/.PdfMarginLeft/.PdfMarginRight` (Task 1).
- Produces: `Meta.PdfFont string`, `.PdfLineHeight float64`, `.PdfCharsPerLine int`,
  `.PdfMarginTop/.PdfMarginBottom/.PdfMarginLeft/.PdfMarginRight float64` — consumed by
  Task 3's `writePDF`.

- [ ] **Step 1: Write the failing test**

First check where `Meta` literals are already tested:

```bash
grep -n "Meta{" export.go export_wiring_test.go export_ast_test.go 2>/dev/null
```

Add to `export_ast_test.go` (or create the assertion inline if no existing test touches
`Meta` construction directly — this test only needs the struct to compile with the new
fields, since `runExport`'s wiring is exercised indirectly by the PDF tests in Task 3):

```go
func TestMetaCarriesPdfFields(t *testing.T) {
	m := Meta{
		Title: "T", PdfFont: "times", PdfLineHeight: 22, PdfCharsPerLine: 70,
		PdfMarginTop: 60, PdfMarginBottom: 60, PdfMarginLeft: 90, PdfMarginRight: 90,
	}
	if m.PdfFont != "times" || m.PdfLineHeight != 22 || m.PdfCharsPerLine != 70 {
		t.Fatalf("Meta PDF fields not set: %+v", m)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestMetaCarriesPdfFields -v`
Expected: FAIL — compile error, `Meta` has no field `PdfFont`.

- [ ] **Step 3: Add the fields to `Meta` and wire `runExport`**

In `export_ast.go`, extend `Meta`:

```go
type Meta struct {
	Author  string
	Title   string
	Contact string // OKASHI_CONTACT: free-text address/email block for the title page
	// TitlePage requests a Shunn-style manuscript title page (whole-manuscript exports only).
	TitlePage bool

	// PDF Manuscript-style typography (StyleManuscript only; StyleTufte ignores these).
	PdfFont                                            string
	PdfLineHeight                                       float64
	PdfCharsPerLine                                     int
	PdfMarginTop, PdfMarginBottom                       float64
	PdfMarginLeft, PdfMarginRight                       float64
}
```

In `export.go`, inside `runExport`, extend the `meta := Meta{...}` literal (it already
has `eff := resolveSettings(dir)` in scope just above it):

```go
	eff := resolveSettings(dir)
	meta := Meta{
		Author:    eff.Author,
		Title:     title,
		Contact:   eff.Contact,
		TitlePage: m.exportWholeManuscript(),

		PdfFont:         eff.PdfFont,
		PdfLineHeight:   eff.PdfLineHeight,
		PdfCharsPerLine: eff.PdfCharsPerLine,
		PdfMarginTop:    eff.PdfMarginTop,
		PdfMarginBottom: eff.PdfMarginBottom,
		PdfMarginLeft:   eff.PdfMarginLeft,
		PdfMarginRight:  eff.PdfMarginRight,
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run TestMetaCarriesPdfFields -v`
Expected: PASS

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all PASS. `writePDF` and the other 4 writers still ignore the new `Meta`
fields at this point (Task 3 wires PDF), so behavior is unchanged.

- [ ] **Step 6: Commit**

```bash
git add export_ast.go export.go export_ast_test.go
git commit -m "export: thread resolved PDF typography settings through Meta"
```

---

## Task 3: `writePDF` uses `Meta`'s PDF fields on the Manuscript branch

**Files:**
- Modify: `export_pdf.go`
- Test: `export_pdf_test.go`

**Interfaces:**
- Consumes: `Meta.PdfFont/.PdfLineHeight/.PdfCharsPerLine/.PdfMarginTop/.PdfMarginBottom/
  .PdfMarginLeft/.PdfMarginRight` (Task 2).
- Produces: `resolvePdfFontName(font string) string` (maps `"times"` → `"Times"`,
  anything else → `"Courier"` — the fpdf core-font name), `resolveManuscriptMargins(pdf
  *fpdf.Fpdf, meta Meta) (top, bottom, left, right float64)`.

Existing tests call `writePDF(doc, StyleManuscript, Meta{Author: "Doe", Title: "..."})`
with zero-value `Meta` for the new fields — **zero-value `PdfFont` is `""`**, which must
resolve to Courier (not error), so `resolvePdfFontName` treats anything other than
exactly `"times"` as Courier. This keeps all 9 existing `export_pdf_test.go` cases
passing unmodified, since none of them set the new `Meta` fields.

- [ ] **Step 1: Write the failing tests**

Append to `export_pdf_test.go`:

```go
func TestWritePDFTimesFont(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Times body text."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T", PdfFont: "times"})
	if err != nil {
		t.Fatalf("times font should not error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestWritePDFZeroValueMetaMatchesDefaults(t *testing.T) {
	// A Meta with no PDF fields set (as every pre-existing call site produces) must
	// render without error, on the Courier path, identically to before this change.
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Plain line."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T"})
	if err != nil {
		t.Fatalf("zero-value Meta should render fine: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestResolvePdfFontName(t *testing.T) {
	cases := map[string]string{
		"":        "Courier",
		"courier": "Courier",
		"times":   "Times",
		"bogus":   "Courier",
	}
	for in, want := range cases {
		if got := resolvePdfFontName(in); got != want {
			t.Errorf("resolvePdfFontName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveManuscriptMarginsCourierDerivesFromCPL(t *testing.T) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	meta := Meta{PdfFont: "courier", PdfCharsPerLine: 62, PdfMarginTop: 72, PdfMarginBottom: 72}
	top, bottom, left, right := resolveManuscriptMargins(pdf, meta)
	if top != 72 || bottom != 72 {
		t.Fatalf("top/bottom should pass through: got %v/%v", top, bottom)
	}
	if left != right {
		t.Fatalf("courier margins should be symmetric: left=%v right=%v", left, right)
	}
	// CPL 62 at Courier 12pt on A4 (595.28pt wide) reproduces ~72pt margins (today's default).
	if left < 68 || left > 76 {
		t.Fatalf("left margin = %v, want close to 72 (default CPL)", left)
	}
}

func TestResolveManuscriptMarginsTimesUsesDirectValues(t *testing.T) {
	pdf := fpdf.New("P", "pt", "A4", "")
	pdf.AddPage()
	meta := Meta{PdfFont: "times", PdfMarginTop: 50, PdfMarginBottom: 60, PdfMarginLeft: 80, PdfMarginRight: 90}
	top, bottom, left, right := resolveManuscriptMargins(pdf, meta)
	if top != 50 || bottom != 60 || left != 80 || right != 90 {
		t.Fatalf("times margins should pass through directly: got %v/%v/%v/%v", top, bottom, left, right)
	}
}

func TestWritePDFCustomLineHeightNoError(t *testing.T) {
	doc := ManuscriptDoc{{Title: "one", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Line."}}},
	}}}
	out, err := writePDF(doc, StyleManuscript, Meta{Title: "T", PdfFont: "courier", PdfLineHeight: 30, PdfCharsPerLine: 62, PdfMarginTop: 72, PdfMarginBottom: 72})
	if err != nil {
		t.Fatalf("custom line height should not error: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
}

func TestWritePDFTufteIgnoresPdfMetaFields(t *testing.T) {
	// Tufte must render identically regardless of what's in the new Meta fields —
	// they're Manuscript-only.
	doc := ManuscriptDoc{{Title: "t", Blocks: []Block{
		Paragraph{Runs: []Run{{Text: "Tufte body."}}},
	}}}
	baseline, err := writePDF(doc, StyleTufte, Meta{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	withPdfFields, err := writePDF(doc, StyleTufte, Meta{
		Title: "T", PdfFont: "times", PdfLineHeight: 30, PdfCharsPerLine: 100,
		PdfMarginTop: 200, PdfMarginBottom: 200, PdfMarginLeft: 200, PdfMarginRight: 200,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline) != len(withPdfFields) {
		t.Fatalf("Tufte output size differs when Manuscript-only fields are set: %d vs %d", len(baseline), len(withPdfFields))
	}
}
```

Add `"codeberg.org/go-pdf/fpdf"` to `export_pdf_test.go`'s imports if not already
present (check with `grep -n '"codeberg.org/go-pdf/fpdf"' export_pdf_test.go` first).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestWritePDFTimesFont|TestWritePDFZeroValueMetaMatchesDefaults|TestResolvePdfFontName|TestResolveManuscriptMargins|TestWritePDFCustomLineHeightNoError|TestWritePDFTufteIgnoresPdfMetaFields' -v`
Expected: FAIL — `resolvePdfFontName`/`resolveManuscriptMargins` undefined (compile
error).

- [ ] **Step 3: Implement the resolution helpers and wire `writePDF`**

In `export_pdf.go`, add near the top (after `stripAstral`, before `plainText`):

```go
// resolvePdfFontName maps a stored/effective PdfFont value to the fpdf core-font name.
// Anything other than exactly "times" resolves to Courier — this includes the empty
// string, so a zero-value Meta (every call site that predates this feature) renders on
// the Courier path exactly as before.
func resolvePdfFontName(font string) string {
	if font == "times" {
		return "Times"
	}
	return "Courier"
}

// resolveManuscriptMargins computes the 4 page margins for the Manuscript PDF style.
// Courier (fixed-pitch) derives left/right from PdfCharsPerLine so the body text wraps
// at exactly that column count; Times (variable-pitch) has no meaningful "chars per
// line", so its margins are used directly.
func resolveManuscriptMargins(pdf *fpdf.Fpdf, meta Meta) (top, bottom, left, right float64) {
	top, bottom = meta.PdfMarginTop, meta.PdfMarginBottom
	if resolvePdfFontName(meta.PdfFont) != "Courier" {
		return top, bottom, meta.PdfMarginLeft, meta.PdfMarginRight
	}
	pdf.SetFont("Courier", "", 12)
	charWidth := pdf.GetStringWidth("0") // fixed-pitch: any character has the same width
	pageWidth, _ := pdf.GetPageSize()
	textWidth := float64(meta.PdfCharsPerLine) * charWidth
	margin := (pageWidth - textWidth) / 2
	if margin < 20 {
		margin = 20 // guard; unreachable with CharsPerLine clamped to [40,120]
	}
	return top, bottom, margin, margin
}
```

Replace `writePDF`'s Manuscript setup (the `else` branch and the `cfg` literal) — full
function body after the change:

```go
func writePDF(doc ManuscriptDoc, st ExportStyle, meta Meta) (out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			out, err = nil, fmt.Errorf("pdf render failed: %v", r)
		}
	}()
	pdf := fpdf.New("P", "pt", "A4", "")
	fontName := resolvePdfFontName(meta.PdfFont)
	lineHeight := meta.PdfLineHeight
	if lineHeight == 0 {
		lineHeight = defaultPdfLineHeight // zero-value Meta (pre-existing call sites)
	}
	cfg := pdfStyle{font: fontName, bodySize: 12, titleSize: 14, lineHeight: lineHeight, indent: "     "}
	autoBreakMargin := 72.0
	if st == StyleTufte {
		registerETBook(pdf)
		cfg = pdfStyle{font: "etbook", bodySize: 12, titleSize: 16, lineHeight: 17, indent: ""}
		pdf.SetMargins(108, 90, 108)
	} else {
		top, bottom, left, right := resolveManuscriptMargins(pdf, withManuscriptMarginDefaults(meta))
		pdf.SetMargins(left, top, right)
		autoBreakMargin = bottom
		pdf.AliasNbPages("{nb}")
		hasTitle := meta.TitlePage
		pdf.SetHeaderFunc(func() {
			if hasTitle && pdf.PageNo() == 1 {
				return // the title page carries no running header
			}
			pdf.SetFont(fontName, "", 12)
			hdr := fmt.Sprintf("%s / %s / %d", meta.Author, strings.ToUpper(meta.Title), pdf.PageNo())
			pdf.CellFormat(0, 14, pdfEnc(st, hdr), "", 0, "R", false, 0, "")
			pdf.Ln(24)
		})
	}
	pdf.SetAutoPageBreak(true, autoBreakMargin)

	if st == StyleManuscript && meta.TitlePage {
		writeTitlePagePDF(pdf, st, meta, manuscriptWordCount(doc), fontName)
	}
	for _, sec := range doc {
		pdf.AddPage()
		pdf.SetFont(cfg.font, "B", cfg.titleSize)
		pdf.CellFormat(0, cfg.lineHeight, pdfEnc(st, sec.Title), "", 1, "C", false, 0, "")
		pdf.Ln(cfg.lineHeight)
		pdf.SetFont(cfg.font, "", cfg.bodySize)
		for _, blk := range sec.Blocks {
			writeBlockPDF(pdf, blk, st, cfg)
		}
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// withManuscriptMarginDefaults fills in the Manuscript-style PDF defaults for any
// zero-value field in meta, so pre-existing call sites (a bare Meta{Title: ...}) render
// identically to before this feature — a zero float64 must not be treated as "margin 0".
func withManuscriptMarginDefaults(meta Meta) Meta {
	if meta.PdfCharsPerLine == 0 {
		meta.PdfCharsPerLine = defaultPdfCharsPerLine
	}
	if meta.PdfMarginTop == 0 {
		meta.PdfMarginTop = defaultPdfMargin
	}
	if meta.PdfMarginBottom == 0 {
		meta.PdfMarginBottom = defaultPdfMargin
	}
	if meta.PdfMarginLeft == 0 {
		meta.PdfMarginLeft = defaultPdfMargin
	}
	if meta.PdfMarginRight == 0 {
		meta.PdfMarginRight = defaultPdfMargin
	}
	return meta
}
```

Note `pdf.SetAutoPageBreak(true, autoBreakMargin)` stays a single unconditional call
after the if/else, same as the original `pdf.SetAutoPageBreak(true, 72)` — only the
value is now a variable, set to the resolved Manuscript `bottom` margin inside the
Manuscript branch, left at its `72.0` initialization for Tufte. Don't introduce a second
call site.

Update `writeTitlePagePDF` to accept and use the resolved font name instead of the
hardcoded `"Courier"` literal:

```go
func writeTitlePagePDF(pdf *fpdf.Fpdf, st ExportStyle, meta Meta, words int, fontName string) {
	pdf.AddPage()
	pdf.SetFont(fontName, "", 12)
	topY := pdf.GetY()
	// Word count, top-right.
	pdf.CellFormat(0, 14, pdfEnc(st, approxWords(words)), "", 1, "R", false, 0, "")
	// Contact block, top-left, starting on the same band.
	pdf.SetXY(72, topY)
	var left []string
	if meta.Author != "" {
		left = append(left, meta.Author)
	}
	left = append(left, contactLines(meta.Contact)...)
	for _, ln := range left {
		pdf.CellFormat(0, 14, pdfEnc(st, ln), "", 1, "L", false, 0, "")
	}
	// Title + byline near the vertical middle (A4 is 841.89pt tall).
	pdf.SetY(383)
	pdf.CellFormat(0, 14, pdfEnc(st, meta.Title), "", 1, "C", false, 0, "")
	if meta.Author != "" {
		pdf.CellFormat(0, 14, pdfEnc(st, "by "+meta.Author), "", 1, "C", false, 0, "")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestWritePDF|TestResolvePdfFontName|TestResolveManuscriptMargins' -v`
Expected: PASS — including all pre-existing `TestWritePDF*` cases (they pass zero-value
PDF `Meta` fields, which now resolve to the same Courier/24pt/72pt-margins output as
before).

- [ ] **Step 5: Run the full suite**

Run: `go test ./...`
Expected: all PASS, including the whole-manuscript export wiring tests
(`export_wiring_test.go`) which build `Meta` via `runExport` (Task 2's change) and now
flow through this writer unchanged for the default case.

- [ ] **Step 6: Commit**

```bash
git add export_pdf.go export_pdf_test.go
git commit -m "export: writePDF reads font/line-height/margins from Meta on the Manuscript branch"
```

---

## Task 4: Properties screen — font toggle, line height, conditional CPL/margins

**Files:**
- Modify: `properties.go`
- Test: `properties_test.go`

**Interfaces:**
- Consumes: `effectiveSettings.PdfFont/.PdfLineHeight/.PdfCharsPerLine/.PdfMarginTop/
  .PdfMarginBottom/.PdfMarginLeft/.PdfMarginRight` (Task 1); `clampPdfLineHeight`,
  `clampPdfCharsPerLine`, `clampPdfMargin` (Task 1); `projectSettings.PdfFont` etc.
  (Task 1).
- Produces: `propKind` constants `propPdfFont`, `propPdfLineHeight`,
  `propPdfCharsPerLine`, `propPdfMarginTop`, `propPdfMarginBottom`, `propPdfMarginLeft`,
  `propPdfMarginRight`; `propertiesModel.rebuildFields()` (recomputes `p.fields` — called
  on init and whenever `propPdfFont` toggles).

- [ ] **Step 1: Write the failing tests**

First inspect `properties_test.go`'s existing structure to match its style:

```bash
grep -n "^func Test" properties_test.go
```

Append to `properties_test.go`:

```go
func TestNewPropertiesModelPdfDefaults(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	if p.pdfFont != "courier" {
		t.Errorf("pdfFont default = %q, want %q", p.pdfFont, "courier")
	}
	if p.pdfLineHeight.Value() != "24" {
		t.Errorf("pdfLineHeight default = %q, want %q", p.pdfLineHeight.Value(), "24")
	}
	if p.pdfCharsPerLine.Value() != "62" {
		t.Errorf("pdfCharsPerLine default = %q, want %q", p.pdfCharsPerLine.Value(), "62")
	}
}

func TestPropertiesFieldsShowCplForCourierMarginsForTimes(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)

	hasField := func(k propKind) bool {
		for _, f := range p.fields {
			if f == k {
				return true
			}
		}
		return false
	}

	if !hasField(propPdfCharsPerLine) {
		t.Error("courier (default) should show propPdfCharsPerLine")
	}
	if hasField(propPdfMarginLeft) || hasField(propPdfMarginRight) {
		t.Error("courier (default) should not show margin fields")
	}

	p.pdfFont = "times"
	p.rebuildFields()

	if hasField(propPdfCharsPerLine) {
		t.Error("times should not show propPdfCharsPerLine")
	}
	if !hasField(propPdfMarginLeft) || !hasField(propPdfMarginRight) || !hasField(propPdfMarginTop) || !hasField(propPdfMarginBottom) {
		t.Error("times should show all 4 margin fields")
	}
}

func TestPropertiesPdfDirtyTracking(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	if p.dirty() {
		t.Fatal("freshly loaded model should not be dirty")
	}
	p.pdfFont = "times"
	if !p.dirty() {
		t.Fatal("changing pdfFont should mark dirty")
	}
}

func TestPropertiesSavePdfFields(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	p.pdfFont = "times"
	p.rebuildFields()
	p.pdfLineHeight.SetValue("20")
	p.pdfMarginTop.SetValue("50")
	p.pdfMarginBottom.SetValue("50")
	p.pdfMarginLeft.SetValue("85")
	p.pdfMarginRight.SetValue("85")

	if _, err := p.save(); err != nil {
		t.Fatal(err)
	}

	ps := loadProjectSettings(dir)
	if ps.PdfFont == nil || *ps.PdfFont != "times" {
		t.Fatalf("saved PdfFont: %+v", ps.PdfFont)
	}
	if ps.PdfLineHeight == nil || *ps.PdfLineHeight != 20 {
		t.Fatalf("saved PdfLineHeight: %+v", ps.PdfLineHeight)
	}
	if ps.PdfMarginLeft == nil || *ps.PdfMarginLeft != 85 {
		t.Fatalf("saved PdfMarginLeft: %+v", ps.PdfMarginLeft)
	}
}

func TestPropertiesSaveInvalidPdfFieldReverts(t *testing.T) {
	dir := t.TempDir()
	p := newPropertiesModel(dir)
	p.pdfLineHeight.SetValue("not-a-number")
	// Simulate the commit path used by updatePropertiesEditing for an out-of-range/invalid value.
	p.commitPdfLineHeight()
	if p.pdfLineHeight.Value() != "24" {
		t.Fatalf("invalid line height should revert to original 24, got %q", p.pdfLineHeight.Value())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./... -run 'TestNewPropertiesModelPdf|TestPropertiesFieldsShowCpl|TestPropertiesPdfDirtyTracking|TestPropertiesSavePdfFields|TestPropertiesSaveInvalidPdfFieldReverts' -v`
Expected: FAIL — compile errors (`p.pdfFont`, `propPdfFont`, `p.rebuildFields`,
`p.commitPdfLineHeight` undefined).

- [ ] **Step 3: Implement the model, view, and save logic**

In `properties.go`, extend `propKind`:

```go
const (
	propTitle propKind = iota
	propAuthor
	propContact
	propWidth
	propSmartquotes
	propCover
	propPdfFont
	propPdfLineHeight
	propPdfCharsPerLine
	propPdfMarginTop
	propPdfMarginBottom
	propPdfMarginLeft
	propPdfMarginRight
)
```

Extend `propertiesModel`:

```go
type propertiesModel struct {
	dir          string
	isManuscript bool

	title       textinput.Model
	author      textinput.Model
	width       textinput.Model
	contact     textarea.Model
	cover       textinput.Model
	smartquotes bool

	pdfFont         string // "courier" | "times"
	pdfLineHeight   textinput.Model
	pdfCharsPerLine textinput.Model
	pdfMarginTop    textinput.Model
	pdfMarginBottom textinput.Model
	pdfMarginLeft   textinput.Model
	pdfMarginRight  textinput.Model

	fields      []propKind // editable field order (Title omitted for a non-manuscript dir)
	focus       int        // index into fields
	editing     bool       // the focused field is capturing keys
	confirmExit bool       // esc-with-unsaved-changes prompt is up

	// snapshots of loaded values, for dirty-tracking and per-store save decisions
	origTitle       string
	origAuthor      string
	origContact     string
	origWidth       int
	origSmartquotes bool
	origCover       string

	origPdfFont                                         string
	origPdfLineHeight                                    float64
	origPdfCharsPerLine                                  int
	origPdfMarginTop, origPdfMarginBottom               float64
	origPdfMarginLeft, origPdfMarginRight               float64
}
```

Extend `newPropertiesModel` (after the existing field constructions, before building `p`):

```go
func newPropertiesModel(dir string) propertiesModel {
	eff := resolveSettings(dir)
	isMs := hasManifest(dir)
	title := deriveTitle(dir)

	ca := textarea.New()
	ca.Prompt = ""
	ca.ShowLineNumbers = false
	ca.CharLimit = 0
	ca.MaxHeight = 0
	ca.SetWidth(40)
	ca.SetHeight(4)
	ca.SetValue(eff.Contact)

	p := propertiesModel{
		dir:             dir,
		isManuscript:    isMs,
		title:           newPropInput(title, 40),
		author:          newPropInput(eff.Author, 40),
		width:           newPropInput(strconv.Itoa(eff.Width), 6),
		contact:         ca,
		cover:           newPropInput(eff.Cover, 40),
		smartquotes:     eff.Smartquotes,
		origTitle:       title,
		origAuthor:      eff.Author,
		origContact:     eff.Contact,
		origWidth:       eff.Width,
		origSmartquotes: eff.Smartquotes,
		origCover:       eff.Cover,

		pdfFont:         eff.PdfFont,
		pdfLineHeight:   newPropInput(formatPdfFloat(eff.PdfLineHeight), 6),
		pdfCharsPerLine: newPropInput(strconv.Itoa(eff.PdfCharsPerLine), 6),
		pdfMarginTop:    newPropInput(formatPdfFloat(eff.PdfMarginTop), 6),
		pdfMarginBottom: newPropInput(formatPdfFloat(eff.PdfMarginBottom), 6),
		pdfMarginLeft:   newPropInput(formatPdfFloat(eff.PdfMarginLeft), 6),
		pdfMarginRight:  newPropInput(formatPdfFloat(eff.PdfMarginRight), 6),
		origPdfFont:          eff.PdfFont,
		origPdfLineHeight:    eff.PdfLineHeight,
		origPdfCharsPerLine:  eff.PdfCharsPerLine,
		origPdfMarginTop:     eff.PdfMarginTop,
		origPdfMarginBottom:  eff.PdfMarginBottom,
		origPdfMarginLeft:    eff.PdfMarginLeft,
		origPdfMarginRight:   eff.PdfMarginRight,
	}
	p.rebuildFields()
	return p
}

// formatPdfFloat renders a PDF layout float without a trailing ".0" for whole numbers
// (24, not 24.0), matching how a user types these values.
func formatPdfFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// rebuildFields recomputes p.fields: the base fields (unchanged), plus the PDF font/line
// height (always present), plus either chars-per-line (Courier) or the 4 margins
// (Times) depending on the currently-selected p.pdfFont. Called on init and whenever the
// font toggle changes p.pdfFont, so the visible field list tracks the in-progress edit
// (not the saved value).
func (p *propertiesModel) rebuildFields() {
	var base []propKind
	if p.isManuscript {
		base = []propKind{propTitle, propAuthor, propContact, propWidth, propCover, propSmartquotes}
	} else {
		base = []propKind{propAuthor, propContact, propWidth, propCover, propSmartquotes}
	}
	base = append(base, propPdfFont, propPdfLineHeight)
	if p.pdfFont == "times" {
		base = append(base, propPdfMarginTop, propPdfMarginBottom, propPdfMarginLeft, propPdfMarginRight)
	} else {
		base = append(base, propPdfCharsPerLine)
	}
	p.fields = base
	if p.focus >= len(p.fields) {
		p.focus = len(p.fields) - 1
	}
}
```

Extend `dirty()`:

```go
func (p *propertiesModel) dirty() bool {
	if p.isManuscript && p.title.Value() != p.origTitle {
		return true
	}
	if p.author.Value() != p.origAuthor || p.contact.Value() != p.origContact {
		return true
	}
	if p.smartquotes != p.origSmartquotes {
		return true
	}
	if w, err := strconv.Atoi(strings.TrimSpace(p.width.Value())); err != nil || w != p.origWidth {
		return true
	}
	if p.cover.Value() != p.origCover {
		return true
	}
	if p.pdfFont != p.origPdfFont {
		return true
	}
	if lh, err := strconv.ParseFloat(strings.TrimSpace(p.pdfLineHeight.Value()), 64); err != nil || lh != p.origPdfLineHeight {
		return true
	}
	if cpl, err := strconv.Atoi(strings.TrimSpace(p.pdfCharsPerLine.Value())); err != nil || cpl != p.origPdfCharsPerLine {
		return true
	}
	for _, pair := range []struct {
		ti  textinput.Model
		orig float64
	}{
		{p.pdfMarginTop, p.origPdfMarginTop},
		{p.pdfMarginBottom, p.origPdfMarginBottom},
		{p.pdfMarginLeft, p.origPdfMarginLeft},
		{p.pdfMarginRight, p.origPdfMarginRight},
	} {
		if v, err := strconv.ParseFloat(strings.TrimSpace(pair.ti.Value()), 64); err != nil || v != pair.orig {
			return true
		}
	}
	return false
}
```

Extend `focusInput`/`blurInputs`:

```go
func (p *propertiesModel) focusInput() {
	switch p.fields[p.focus] {
	case propTitle:
		p.title.Focus()
	case propAuthor:
		p.author.Focus()
	case propWidth:
		p.width.Focus()
	case propContact:
		p.contact.Focus()
	case propCover:
		p.cover.Focus()
	case propPdfLineHeight:
		p.pdfLineHeight.Focus()
	case propPdfCharsPerLine:
		p.pdfCharsPerLine.Focus()
	case propPdfMarginTop:
		p.pdfMarginTop.Focus()
	case propPdfMarginBottom:
		p.pdfMarginBottom.Focus()
	case propPdfMarginLeft:
		p.pdfMarginLeft.Focus()
	case propPdfMarginRight:
		p.pdfMarginRight.Focus()
	}
}

func (p *propertiesModel) blurInputs() {
	p.title.Blur()
	p.author.Blur()
	p.width.Blur()
	p.contact.Blur()
	p.cover.Blur()
	p.pdfLineHeight.Blur()
	p.pdfCharsPerLine.Blur()
	p.pdfMarginTop.Blur()
	p.pdfMarginBottom.Blur()
	p.pdfMarginLeft.Blur()
	p.pdfMarginRight.Blur()
}
```

Add commit/validation helpers (mirroring the existing inline `propWidth` validation in
`updatePropertiesEditing`, extracted here to be independently testable per Task 4's
Step 1 test):

```go
// commitPdfLineHeight validates and clamps the line-height field on commit; invalid
// input reverts to the original loaded value (mirrors the propWidth pattern).
func (p *propertiesModel) commitPdfLineHeight() {
	f, err := strconv.ParseFloat(strings.TrimSpace(p.pdfLineHeight.Value()), 64)
	if err != nil || f < 10 || f > 40 {
		p.pdfLineHeight.SetValue(formatPdfFloat(p.origPdfLineHeight))
	}
}

func (p *propertiesModel) commitPdfCharsPerLine() {
	n, err := strconv.Atoi(strings.TrimSpace(p.pdfCharsPerLine.Value()))
	if err != nil || n < 40 || n > 120 {
		p.pdfCharsPerLine.SetValue(strconv.Itoa(p.origPdfCharsPerLine))
	}
}

// commitPdfMargin validates and clamps one margin field in place, reverting to orig on
// invalid input.
func commitPdfMargin(ti *textinput.Model, orig float64) {
	f, err := strconv.ParseFloat(strings.TrimSpace(ti.Value()), 64)
	if err != nil || f < 20 || f > 200 {
		ti.SetValue(formatPdfFloat(orig))
	}
}
```

Extend `updatePropertiesEditing`'s commit branch (the block that currently only
special-cases `propWidth`):

```go
	commit := ks == "esc" || (kind != propContact && ks == "enter")
	if commit {
		p.editing = false
		p.blurInputs()
		switch kind {
		case propWidth:
			if n, err := strconv.Atoi(strings.TrimSpace(p.width.Value())); err != nil || n < 20 || n > 200 {
				p.width.SetValue(strconv.Itoa(p.origWidth))
				m.status = "la largeur doit être comprise entre 20 et 200"
			}
		case propPdfLineHeight:
			p.commitPdfLineHeight()
		case propPdfCharsPerLine:
			p.commitPdfCharsPerLine()
		case propPdfMarginTop:
			commitPdfMargin(&p.pdfMarginTop, p.origPdfMarginTop)
		case propPdfMarginBottom:
			commitPdfMargin(&p.pdfMarginBottom, p.origPdfMarginBottom)
		case propPdfMarginLeft:
			commitPdfMargin(&p.pdfMarginLeft, p.origPdfMarginLeft)
		case propPdfMarginRight:
			commitPdfMargin(&p.pdfMarginRight, p.origPdfMarginRight)
		}
		return m, nil
	}

	var cmd tea.Cmd
	switch kind {
	case propTitle:
		p.title, cmd = p.title.Update(key)
	case propAuthor:
		p.author, cmd = p.author.Update(key)
	case propWidth:
		p.width, cmd = p.width.Update(key)
	case propContact:
		p.contact, cmd = p.contact.Update(key)
	case propCover:
		p.cover, cmd = p.cover.Update(key)
	case propPdfLineHeight:
		p.pdfLineHeight, cmd = p.pdfLineHeight.Update(key)
	case propPdfCharsPerLine:
		p.pdfCharsPerLine, cmd = p.pdfCharsPerLine.Update(key)
	case propPdfMarginTop:
		p.pdfMarginTop, cmd = p.pdfMarginTop.Update(key)
	case propPdfMarginBottom:
		p.pdfMarginBottom, cmd = p.pdfMarginBottom.Update(key)
	case propPdfMarginLeft:
		p.pdfMarginLeft, cmd = p.pdfMarginLeft.Update(key)
	case propPdfMarginRight:
		p.pdfMarginRight, cmd = p.pdfMarginRight.Update(key)
	}
	return m, cmd
```

Extend the `updateProperties` `enter`/`space` handler so `propPdfFont` toggles (like
`propSmartquotes`) instead of opening a text editor, and rebuilds fields on toggle:

```go
	case "enter", " ":
		switch p.fields[p.focus] {
		case propSmartquotes:
			p.smartquotes = !p.smartquotes
		case propPdfFont:
			if p.pdfFont == "courier" {
				p.pdfFont = "times"
			} else {
				p.pdfFont = "courier"
			}
			p.rebuildFields()
		default:
			p.editing = true
			p.focusInput()
		}
```

Extend `save()` — add to the existing per-project-settings block (after the
`coverChanged` check, inside the same `if widthChanged || sqChanged || coverChanged`
condition, which must now include the new dirty flags too):

```go
func (p *propertiesModel) save() (projectChanged bool, err error) {
	// Title → manifest (manuscript only; read-modify-write to keep items/schema intact).
	if p.isManuscript && p.title.Value() != p.origTitle {
		mani, ok, rerr := readManifest(p.dir)
		if rerr != nil {
			return false, rerr
		}
		if ok {
			mani.Title = p.title.Value()
			if werr := writeManifest(p.dir, mani); werr != nil {
				return false, werr
			}
		}
	}
	// Author/contact → personal global config.
	if p.author.Value() != p.origAuthor || p.contact.Value() != p.origContact {
		if werr := saveUserConfig(userConfigPath(), userConfig{Author: p.author.Value(), Contact: p.contact.Value()}); werr != nil {
			return false, werr
		}
	}
	// Width/smartquotes/cover/PDF typography → per-project .okashi.json (overlay onto existing file fields).
	w, werr := strconv.Atoi(strings.TrimSpace(p.width.Value()))
	widthChanged := werr == nil && w != p.origWidth
	sqChanged := p.smartquotes != p.origSmartquotes
	coverChanged := p.cover.Value() != p.origCover

	pdfFontChanged := p.pdfFont != p.origPdfFont
	lh, lhErr := strconv.ParseFloat(strings.TrimSpace(p.pdfLineHeight.Value()), 64)
	lhChanged := lhErr == nil && lh != p.origPdfLineHeight
	cpl, cplErr := strconv.Atoi(strings.TrimSpace(p.pdfCharsPerLine.Value()))
	cplChanged := cplErr == nil && cpl != p.origPdfCharsPerLine
	mTop, mTopErr := strconv.ParseFloat(strings.TrimSpace(p.pdfMarginTop.Value()), 64)
	mTopChanged := mTopErr == nil && mTop != p.origPdfMarginTop
	mBottom, mBottomErr := strconv.ParseFloat(strings.TrimSpace(p.pdfMarginBottom.Value()), 64)
	mBottomChanged := mBottomErr == nil && mBottom != p.origPdfMarginBottom
	mLeft, mLeftErr := strconv.ParseFloat(strings.TrimSpace(p.pdfMarginLeft.Value()), 64)
	mLeftChanged := mLeftErr == nil && mLeft != p.origPdfMarginLeft
	mRight, mRightErr := strconv.ParseFloat(strings.TrimSpace(p.pdfMarginRight.Value()), 64)
	mRightChanged := mRightErr == nil && mRight != p.origPdfMarginRight

	if widthChanged || sqChanged || coverChanged || pdfFontChanged || lhChanged || cplChanged ||
		mTopChanged || mBottomChanged || mLeftChanged || mRightChanged {
		ps := loadProjectSettings(p.dir)
		if widthChanged {
			wv := clampWidth(w)
			ps.Width = &wv
		}
		if sqChanged {
			sv := p.smartquotes
			ps.Smartquotes = &sv
		}
		if coverChanged {
			cv := p.cover.Value()
			ps.Cover = &cv
		}
		if pdfFontChanged {
			fv := p.pdfFont
			ps.PdfFont = &fv
		}
		if lhChanged {
			lv := clampPdfLineHeight(lh)
			ps.PdfLineHeight = &lv
		}
		if cplChanged {
			cv := clampPdfCharsPerLine(cpl)
			ps.PdfCharsPerLine = &cv
		}
		if mTopChanged {
			v := clampPdfMargin(mTop)
			ps.PdfMarginTop = &v
		}
		if mBottomChanged {
			v := clampPdfMargin(mBottom)
			ps.PdfMarginBottom = &v
		}
		if mLeftChanged {
			v := clampPdfMargin(mLeft)
			ps.PdfMarginLeft = &v
		}
		if mRightChanged {
			v := clampPdfMargin(mRight)
			ps.PdfMarginRight = &v
		}
		if serr := saveProjectSettings(p.dir, ps); serr != nil {
			return false, serr
		}
		projectChanged = true
	}
	// Reset baselines so dirty clears.
	p.origTitle = p.title.Value()
	p.origAuthor = p.author.Value()
	p.origContact = p.contact.Value()
	if werr == nil {
		p.origWidth = clampWidth(w)
	}
	p.origSmartquotes = p.smartquotes
	p.origCover = p.cover.Value()
	p.origPdfFont = p.pdfFont
	if lhErr == nil {
		p.origPdfLineHeight = clampPdfLineHeight(lh)
	}
	if cplErr == nil {
		p.origPdfCharsPerLine = clampPdfCharsPerLine(cpl)
	}
	if mTopErr == nil {
		p.origPdfMarginTop = clampPdfMargin(mTop)
	}
	if mBottomErr == nil {
		p.origPdfMarginBottom = clampPdfMargin(mBottom)
	}
	if mLeftErr == nil {
		p.origPdfMarginLeft = clampPdfMargin(mLeft)
	}
	if mRightErr == nil {
		p.origPdfMarginRight = clampPdfMargin(mRight)
	}
	return projectChanged, nil
}
```

Extend `propertiesView` to render the new rows (add cases to the `switch kind` in the
existing `for i, kind := range p.fields` loop):

```go
		case propPdfFont:
			label = "Police PDF"
			if p.pdfFont == "times" {
				val = "Times"
			} else {
				val = "Courier"
			}
		case propPdfLineHeight:
			label, val = "Interligne (pt)", fieldVal(p.pdfLineHeight, editing)
		case propPdfCharsPerLine:
			label, val = "Caract./ligne", fieldVal(p.pdfCharsPerLine, editing)
		case propPdfMarginTop:
			label, val = "Marge haute (pt)", fieldVal(p.pdfMarginTop, editing)
		case propPdfMarginBottom:
			label, val = "Marge basse (pt)", fieldVal(p.pdfMarginBottom, editing)
		case propPdfMarginLeft:
			label, val = "Marge gauche (pt)", fieldVal(p.pdfMarginLeft, editing)
		case propPdfMarginRight:
			label, val = "Marge droite (pt)", fieldVal(p.pdfMarginRight, editing)
```

Update the footer hint to mention the font toggle (extend the existing footer string):

```go
	foot := lipgloss.NewStyle().Foreground(subtle).Render("⇥ champ · ⏎ éditer/basculer · ctrl+s enregistrer · esc retour · F1 aide")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... -run 'TestNewPropertiesModelPdf|TestPropertiesFieldsShowCpl|TestPropertiesPdfDirtyTracking|TestPropertiesSavePdfFields|TestPropertiesSaveInvalidPdfFieldReverts' -v`
Expected: PASS

- [ ] **Step 5: Run the full suite**

Run: `go test ./... && go vet ./... && go build ./...`
Expected: all PASS, vet clean, build succeeds.

- [ ] **Step 6: Commit**

```bash
git add properties.go properties_test.go
git commit -m "properties: PDF font toggle, line height, and conditional CPL/margins fields"
```

---

## Task 5: Manual verification pass

**Files:** none (manual QA against the real binary)

- [ ] **Step 1: Build and run**

```bash
go build -o /tmp/okashi-pdf-check . && /tmp/okashi-pdf-check
```

- [ ] **Step 2: Verify default export is unchanged**

Open any project with no `.okashi.json` PDF fields set. Export PDF (Manuscript style).
Open the resulting PDF and confirm it looks identical to a pre-change export (Courier,
same margins/line spacing) — this is the critical regression check the whole plan is
built around.

- [ ] **Step 3: Verify the Properties screen**

Press `i` on a project. Tab to "Police PDF", press space/enter — confirm it toggles
Courier ↔ Times and the field list below it swaps between "Caract./ligne" (Courier) and
the 4 margin fields (Times) live, without leaving the screen. Edit "Interligne" to an
out-of-range value (e.g. `999`), commit with enter, confirm it reverts and a status
message appears matching the width-field precedent (or silently reverts — no message is
required by the spec, only the revert).

- [ ] **Step 4: Verify Times export**

Set font to Times, set 4 margins to distinct values, save (`ctrl+s`), export PDF. Open
the PDF and confirm the body text is Times (variable-pitch, distinguishable from
Courier at a glance) and the margins visually match what was set.

- [ ] **Step 5: Verify Courier CPL export**

Set font back to Courier, set "Caract./ligne" to e.g. `50` (narrower than the default
62), save, export. Confirm the resulting PDF has visibly wider margins (fewer characters
fit per line) than the default export from Step 2.

- [ ] **Step 6: Verify Tufte is unaffected**

With the same project's PDF fields still set to non-default Times/margins values from
Step 4, export using the Tufte style instead. Confirm the Tufte PDF renders with ET Book
and its usual 108/90/108 margins, unaffected by the Manuscript-only settings.

- [ ] **Step 7: Clean up the manual-check binary**

```bash
rm /tmp/okashi-pdf-check
```

---

## Task 6: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Add a note under Properties in the Project model section**

In `CLAUDE.md`, find the paragraph starting `**Properties** (`i` on a hub project):
okashi's one *editable* metadata surface...` and extend it (or add a sentence
immediately after it) to note: PDF Manuscript-style typography (font: Courier/Times;
line height; characters-per-line for Courier or 4 margins for Times) is now also
editable there, resolved via the same `file → env → default` precedence in
`.okashi.json` (no env knob added — file/default only, since there was no prior
`OKASHI_*` var for PDF typography), and applies only to the PDF Manuscript style (Tufte
is unaffected).

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: document PDF Manuscript typography settings (font, line height, CPL/margins) in CLAUDE.md"
```

---

## Self-Review Notes (for the plan author, already applied above)

- **Spec coverage:** all 7 fields (font, line height, CPL, 4 margins), the
  Courier-CPL-derives-margins / Times-direct-margins split, the Manuscript-only scope,
  the frozen default CPL=62, Properties-screen location, and the "no other format
  touched" constraint are each covered by a task. The spec's "hors périmètre" items
  (other formats, Tufte, custom TTF fonts, alternate units) have no task — correctly, they
  are exclusions.
- **Placeholder scan:** no TBD/TODO; every step has literal code, not a description of
  code.
- **Type consistency:** `Meta.PdfFont string` (Task 2) matches `resolvePdfFontName(font
  string) string` (Task 3) and `p.pdfFont string` (Task 4) — all plain strings, no enum
  mismatch. `effectiveSettings.PdfCharsPerLine int` (Task 1) matches
  `Meta.PdfCharsPerLine int` (Task 2) and `meta.PdfCharsPerLine` usage in
  `resolveManuscriptMargins` (Task 3). Margin fields are `float64` throughout (Task 1
  `effectiveSettings`, Task 2 `Meta`, Task 3 `resolveManuscriptMargins` return values,
  Task 4 `orig*` fields) — no int/float64 mix.
- **Zero-value safety:** Task 3 explicitly handles the case where `Meta`'s new fields are
  the Go zero value (empty string / 0.0), since Task 2 only populates them from
  `runExport`'s call site — any other/future caller of `writePDF` with a bare
  `Meta{Title: ...}` (as all 9 pre-existing tests do) must still render the pre-feature
  default output. `resolvePdfFontName("")` → Courier and
  `withManuscriptMarginDefaults` cover this.
