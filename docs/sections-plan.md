# SDS Sections: Variant System — Implementation Plan

Status: complete, including derivation from GHS hazard codes.
`sdsforge document generate <id>` produces a full 16-section SDS; §2 is computed
and every other section's wording is derived from `hazard_codes:`.
Module: `github.com/isaacvarg/sdsforge` · Go 1.27 · `gopkg.in/yaml.v3`

## The design in one page

A Safety Data Sheet has 16 sections; each section has subsections. Authors need to
swap wording per hazard profile ("more corrosive", "acute inhalation toxicity")
without hand-writing every combination.

**The subsection is the only unit of variance.** A "section variant" is demoted to a
*preset*: a named bundle that picks a variant for each subsection. This avoids the
combinatorial blowup of `corrosive_plus_acute_inhalation.yaml` files.

**Content is a typed block, not free-form data.** Subsections differ by *shape*
(prose, table, list, key/value), not by section. So we type the ~4 shapes and keep
sections/subsections/variants fully data-driven. Adding a section or variant = zero
Go code. Adding a content *kind* = one Go file + one template partial.

### Decisions locked in

| # | Decision |
|---|----------|
| 1 | Variant selection is manual now; derivation from H-codes is roadmap. `applies_when` + `priority` are written into the file format now and ignored by the resolver. |
| 2 | Exactly one variant wins per subsection. Under derivation, highest `priority` wins; a tie is a **validation error**, never an arbitrary pick. |
| 3 | Override semantics: `replace` and `append` only. |
| 4 | Content shape varies per subsection: prose (§4) vs table (§8). |
| 5 | Hybrid typing: generic container, typed content blocks behind a `Content` interface + kind registry. |
| 6 | Built-in library embedded via `go:embed`; optional user overlay gated by a config bool. |
| 7 | OSHA only today, but a `osha/` path segment exists from day one so jurisdiction is never retrofitted. |
| 8 | A subsection resolving to nothing emits `empty_text` ("No data available."). |

Also: string ids in document YAML (`first_aid`), numeric prefixes only in directory
names (`04_first_aid/`). Order comes from `layout.yaml`. Renumbering never breaks
existing documents.

### Directory layout

```
internal/sections/osha/
  layout.yaml                      # section order
  04_first_aid/
    section.yaml                   # subsection order, titles, kinds, empty_text
    presets/corrosive.yaml
    inhalation/{default,acute_toxicity,corrosive}.yaml
    ingestion/default.yaml
  08_exposure_controls/
    section.yaml
    exposure_limits/default.yaml   # kind: table
```

### File formats

`section.yaml`
```yaml
id: first_aid
title: First-aid measures
empty_text: "No data available."
subsections:
  - { id: general,    title: General advice, kind: prose }
  - { id: inhalation, title: Inhalation,     kind: prose }
```

variant file, e.g. `inhalation/acute_toxicity.yaml`
```yaml
variant: acute_toxicity
applies_when: { any_of: [H330, H331, H332] }   # unused until derivation
priority: 20
content:
  kind: prose
  text:
    - "Move victim to fresh air."
    - "If breathing is difficult, give oxygen."
```

`presets/corrosive.yaml`
```yaml
preset: corrosive
picks:
  inhalation: corrosive
  skin: corrosive
  eye: corrosive
```

document YAML
```yaml
sections:
  first_aid:
    variant: corrosive
    subsections:
      inhalation: { variant: acute_toxicity }
      ingestion:  { append: ["Do not induce vomiting."] }
      general:    { replace: { kind: prose, text: "..." } }
```

### Resolution algorithm (one deterministic pass per subsection)

1. `variant := doc.override ?? preset.pick ?? "default"`
   (derivation later slots between preset and default; manual always wins)
2. Load `<subsection>/<variant>.yaml`. Missing file is a hard error.
3. `replace` swaps the block wholesale; `append` calls `Body.Append`.
   Reject if the override's kind differs from the subsection's declared kind.
4. Empty result -> `empty_text`.

---

## Phases

Each phase is independently testable. Do not start a phase before the previous
one's "done when" passes.

### Phase 0 — groundwork  ✅ DONE
- Promote `gopkg.in/yaml.v3` from `// indirect` to a direct require (`go mod tidy`).
- Confirm `go test ./...` runs green on an empty suite.
- **Done when:** `go build ./... && go test ./...` is clean.

### Phase 1 — content blocks and the kind registry  ✅ DONE
Files: `internal/sections/content.go`, `content_test.go`
- `Content` interface: `Append(Content) (Content, error)`, `IsEmpty() bool`, `Kind() string`.
- `Prose` implementing it (paragraph slice; Append concatenates paragraphs).
- `Block` struct wrapping a `Content` plus a custom `UnmarshalYAML` that peeks at
  `kind` and decodes into the registered concrete type.
- `registry map[string]func() Content` + `Register(kind, ctor)`.
- **Go concepts:** interfaces, method sets, pointer vs value receivers, custom
  `yaml.Unmarshaler`, `yaml.Node` two-pass decoding, package-level init.
- **Done when:** a table-driven test round-trips a `kind: prose` YAML doc into a
  `Block`, appends to it, and rejects an unknown kind.

### Phase 2 — a second content kind: table  ✅ DONE
Files: `internal/sections/content_table.go`, tests
- `Table` with `headers []string` and `rows [][]string`.
- `Append` adds rows; error if column count mismatches.
- **Go concepts:** slices of slices, error values vs sentinel errors, `errors.Is`.
- **Done when:** the registry handles both kinds with no changes to `content.go`,
  and appending a wrong-width row returns a typed error.

### Phase 3 — section manifest  ✅ DONE
Files: `internal/sections/manifest.go`, `testdata/`
- `SectionDef` / `SubsectionDef` mirroring `section.yaml`.
- `LoadSection(fsys fs.FS, dir string) (SectionDef, error)`.
- Validate: unique subsection ids, every `kind` is registered, non-empty title.
- **Go concepts:** `io/fs` (not `os`) for testability, struct tags, error wrapping
  with `%w`, `testdata/` conventions.
- **Done when:** loads a real `04_first_aid/section.yaml` from `testdata/` and
  returns a useful error for a duplicate subsection id.

### Phase 4 — the library (embedded + optional overlay)  ✅ DONE
Files: `internal/sections/library.go`
- `//go:embed osha` on an `embed.FS`.
- `Library{ layers []fs.FS }`; `Open(path)` walks layers in order, first hit wins.
- `NewLibrary(customDir string, enabled bool)` prepends an `os.DirFS` overlay only
  when enabled.
- **Go concepts:** `embed`, `fs.FS` / `fs.Sub`, composition over inheritance,
  functional options if you want them.
- **Done when:** a test proves an overlay file shadows the embedded one when
  enabled, and is ignored when disabled.

### Phase 5 — variants and presets  ✅ DONE
Files: `internal/sections/variant.go`
- `VariantFile{ Variant, AppliesWhen, Priority, Content Block }`.
- `Preset{ Preset string, Picks map[string]string }`.
- Loaders for both, reading through the `Library`.
- **Go concepts:** embedding a `Block` in another struct, zero values, maps.
- **Done when:** loading `inhalation/acute_toxicity.yaml` yields a `Prose` body and
  `Priority == 20`; a missing file returns a wrapped `fs.ErrNotExist`.

### Phase 6 — the resolver  ✅ DONE
Files: `internal/sections/resolve.go`
- `ResolvedSection` / `ResolvedSubsection` (title + final `Block`).
- `Resolve(lib *Library, def SectionDef, sel SectionSelection) (ResolvedSection, error)`
  implementing the 4-step algorithm above.
- **Go concepts:** keeping functions pure and dependency-injected, `errors.Join`
  for collecting multiple failures, deterministic map iteration (sort keys!).
- **Done when:** table-driven tests cover: no selection -> defaults; preset only;
  preset + per-subsection override; append; replace; empty -> `empty_text`.

### Phase 7 — document schema wiring  ✅ DONE
Files: `internal/document/types.go`
- Add `Sections map[string]SectionSelection` to `Data`.
- `SectionSelection{ Variant string; Subsections map[string]SubsectionOverride }`.
- `SubsectionOverride{ Variant string; Replace *Block; Append *Block }`.
- Normalize author shorthand: `append: ["..."]` (a bare string list) into a real
  `Block` of the subsection's declared kind.
- **Go concepts:** pointer fields to distinguish "absent" from "empty", package
  boundaries (does `document` import `sections`, or the reverse? decide and record).
- **Done when:** an example `document.yaml` unmarshals into `Data` losslessly.

### Phase 8 — validation pass  ✅ DONE
Files: `internal/sections/validate.go`
- Every referenced variant and preset exists.
- Override kind matches declared subsection kind.
- No duplicate `priority` within one subsection's variant set (future derivation).
- **Done when:** `sdsforge validate <id>` reports all problems at once, not just the first.

### Phase 9 — rendering  ✅ DONE
Files: `internal/generation/render.go`, `templates/`
- `layout.html.tmpl` iterates resolved sections; one partial per content kind
  (`prose.html.tmpl`, `table.html.tmpl`).
- Dispatch on `Kind()` from the template.
- **Go concepts:** `html/template`, `ParseFS`, template composition, `template.FuncMap`.
- **Done when:** a fixture document renders to HTML with a prose section and a table
  section, golden-file tested.

### Phase 10 — seed real content and wire the CLI  ✅ DONE
- Author `01_identification`, `04_first_aid`, `08_exposure_controls` for real.
- Wire `cmd/generate.go` to load document -> resolve -> render -> write.
- **Done when:** `sdsforge generate <id>` produces a real SDS HTML file.

### Later — derivation from hazard codes
- `Material.HazardsTriggered` -> H-code set for the document.
- `derive()` evaluates `applies_when` across a subsection's variants, picks highest
  `priority`, errors on a tie.
- Slots into resolution step 1 between preset and default. Manual still wins.

---

## Built (as of 2026-09-01)

| File | Role |
|---|---|
| `internal/sections/content.go` | `Content` interface, `Prose`, `Block` + two-pass `UnmarshalYAML`, kind registry |
| `internal/sections/content_table.go` | `Table` kind, registered from its own `init()` |
| `internal/sections/errors.go` | Sentinel errors (`ErrKindMismatch`, `ErrHeaderMismatch`, `ErrRaggedRow`) |
| `internal/sections/manifest.go` | `section.yaml` / `layout.yaml` loading and validation |
| `internal/sections/library.go` | `go:embed` built-in library + optional user overlay, layered as `fs.FS` |
| `internal/sections/variant.go` | Variant files, presets, `Predicate` (parsed, not yet evaluated) |
| `internal/sections/resolve.go` | The resolution algorithm; selection types |
| `internal/sections/validate.go` | Whole-library validation |
| `internal/config/config.go` | `~/.config/sdsforge/config.toml`: `[library]` (jurisdiction, `custom_variants` toggle, custom library path), `[company]`, `[emergency]` |
| `internal/generation/render.go` | `html/template` rendering, one partial per content kind |
| `internal/generation/logo.go` | Measures, fits and embeds the company logo |
| `cmd/generate.go` | `document generate <id>` |
| `cmd/sections.go` | `sections list`, `sections validate` |
| `cmd/config.go` | `config path`, `config init`, `config show` |

Content library: `internal/sections/osha/` — 16 sections, 77 variant files, 8 presets.

### Document data as a content source (added 2026-09-01)

A subsection may declare `source:` in section.yaml, naming document data that
populates it. Resolution order is now:

    default -> preset pick -> subsection variant -> source -> replace -> append

The source sits after the authored variant (so library content is the fallback
when a document carries no data) and before replace/append (so an explicit
override still wins). `internal/sections/source.go` owns the vocabulary;
`Data.SourceData()` in `internal/document/types.go` populates it. `sections`
never imports `document`.

| Section | Subsection | source |
|---|---|---|
| 01 | `product_identifier` | `identification` |
| 01 | `recommended_use` | `recommended_use` |
| 01 | `supplier` | `supplier` |
| 01 | `emergency_phone` | `emergency_phone` |
| 03 | `ingredients` | `materials` |
| 16 | `revision` | `revisions` |

`sdsforge document create` now writes an annotated document.yaml generated from
the live library (`internal/document/scaffold.go`), listing every section with
its presets and per-subsection variants. `--minimal` keeps the bare form.

Comment convention in that file, relied on by tests: `# text` is prose (exactly
one space after the hash); `#  yaml:` is enabled by deleting the single leading
hash. Nothing that is not valid, resolvable YAML may use the second form.

### Derivation from GHS hazard codes (added 2026-09-01)

One declaration now drives the sheet:

```yaml
hazard_codes: [H315, H350]     # bare 315 and h315 also accepted
```

Two mechanisms, both live:

**§2 is computed**, not selected. `internal/ghs` loads three reference tables from
the library (`osha/ghs/*.yaml` — 71 H-statements, 90 P-statements, and the
Appendix C assignment map) and returns a `Classification`. `Data.SourceData` turns
it into blocks for the `classification`, `signal_word`, `pictograms` and
`precautionary` sources. No new resolver machinery was needed; §2 uses the same
`source:` mechanism as §1/§3/§16.

**Every other section derives its variant.** `applies_when` and `priority` are now
evaluated in `resolveSubsection`. Full precedence:

    default -> derived -> preset pick -> subsection variant -> source
            -> replace -> append

Manual selection always beats derivation, and `ResolvedSubsection.SupersededDerived`
records where it did, so a reviewer can see every place a human overruled the
automatic classification.

`sdsforge document classify <id>` prints the whole computation: hazard statements,
signal word, pictograms, the selected P-statements with their type, review warnings,
and which variant each section derived and why.

Guardrails, all tested:
- An unknown hazard code is an error naming it — never silently dropped.
- Exceeding App C's six-statement guidance warns; it never truncates.
- A priority tie during derivation is an error, not an arbitrary pick.
- Statements whose regulatory text is manufacturer-specified are flagged until
  `precautionary_text:` supplies concrete wording.
- Cross-reference tests tie all three GHS tables together and check every
  `applies_when` code in the content library against the H-statement table.

> The GHS tables are transcribed reference data for a document carrying legal and
> safety weight. They must be reviewed by a qualified person before production use.

### Hazard pictograms (added 2026-09-01)

The nine GHS pictograms ship as SVG in `internal/sections/osha/ghs/pictograms/`,
converted from the official EPS artwork (`gs -dEPSCrop -sDEVICE=pdfwrite`, then
`pdf2svg`). True vector, ~11 KB each. See the README beside them for provenance.

The code-to-symbol mapping was verified three ways before use: filename, the title
embedded in the source EPS ("EXPLODING BOMB", "CORROSION", ...), and visual
inspection of the rendered artwork. Re-verify visually if these files are ever
replaced -- a pictogram attached to the wrong hazard class is a serious error.

A fourth content kind, `images`, carries them (`content_images.go` plus
`templates/images.html.tmpl`) -- one Go file and one partial, the extension path
the registry was built for. It is deliberately generic rather than GHS-specific,
so section 14 transport labels can reuse it.

Two details that matter:

- **Artwork is embedded as a data: URI**, resolved in `embedImages` at resolve
  time (where the library filesystem is in hand, unlike the renderer). Variant
  files reference artwork by library-relative path; the computed pictograms
  arrive already embedded from `ghs.Pictogram.DataURI`. A sheet gets emailed and
  printed, so it has to stand alone. Only the pictograms a product triggers are
  embedded: ~52 KB for a typical sheet, 156 KB with all nine.
- **`html/template` rejects data: URIs in `src`** by default, replacing them with
  a placeholder. The `imageURI` FuncMap in `render.go` allows only base64 data
  URIs of known image types through as `template.URL` -- validated, not
  blanket-trusted, so a hostile value renders as a broken image instead.

Alt text always carries the code and name, so "GHS05 (corrosion)" survives a
screen reader, images-off, and a text-only print.

### Notes for later

- Derivation coverage is bounded by which variants exist. H314/H318/H330/H331/H332
  and H225/H226 have predicated variants; other codes classify correctly in §2 but
  leave §4/§6/§8/§11 at their defaults until variants are authored for them.
- `Material.HazardsTriggered []int` is deprecated: still parsed so old documents
  load, consumed by nothing. Use `hazard_codes`.
- Section 3's ingredient table is filled from `Data.Materials` through the
  `source:` mechanism above; the old hardcoded `withMaterialsTable` is gone.
- The seeded content is generic OSHA-format boilerplate. It is placeholder text
  for a real SDS and must be reviewed by a qualified person before use.

### Company details in configuration (added 2026-09-02)

Who issues a sheet and who to call about an incident are the same on every
document a company produces, so they moved out of `document.yaml` and into the
user's config file, which became TOML in the process:

```toml
[library]
jurisdiction = "osha"

[company]
name  = "Acme Chemical Co."
phone = "+1-555-0100"

[[emergency.contacts]]
name  = "CHEMTREC (24 hr)"
phone = "1-800-424-9300"
note  = "USA"
```

| # | Decision |
|---|---|
| 1 | TOML, not YAML. The file is a small set of named tables edited by hand; `[company]` says what it is without indentation rules. Documents stay YAML. |
| 2 | `[emergency]` holds a LIST of contacts. A sheet routinely carries several numbers -- a 24-hour service, its international line, the site's own officer -- and a single string could hold only one. |
| 3 | Config always wins. A document's `supplier:` block is no longer read; the struct is kept so old documents still load, and `generate` warns on stderr rather than ignoring it silently. |
| 4 | `config.Company.Lines()` / `Emergency.Lines()` own the formatting, so section 1 and the sheet header cannot disagree about it. |
| 5 | A contact with no phone is a validation error, not a dropped entry: it is a typo, and dropping it leaves a gap on a printed sheet. |
| 6 | A lone `config.yaml` is an error naming both paths. Falling back to defaults would silently discard a working `custom_dir`. |
| 7 | XDG throughout: config under `$XDG_CONFIG_HOME` (via `os.UserConfigDir`), documents under `$XDG_DATA_HOME`. |

An empty `[company]` or `[emergency]` still resolves to the library's authored
placeholder, exactly as an empty `supplier:` block used to.

### Company logo (added 2026-09-02)

`[logo]` in the config file puts the issuer's mark in the sheet header, beside
the product name. Sizing is automatic — the artwork is measured and fitted —
with two bounds to adjust the result:

```toml
[logo]
path = "acme-logo.svg"    # absolute, ~-relative, or beside config.toml
# max_height = "16mm"
# max_width  = "50mm"
```

| # | Decision |
|---|---|
| 1 | Measure and fit, rather than a CSS box alone. The artwork's own dimensions are read (`image.DecodeConfig` for PNG/JPEG, the root element's `width`/`height` or `viewBox` for SVG) and an exact print size computed, so what lands on paper is knowable before printing. |
| 2 | `max_height`/`max_width` bound a BOX the image is fitted inside, never a size it is forced to. The aspect ratio cannot be broken by a config file. |
| 3 | Never upscaled. Scale is capped at 1, because an enlarged raster mark prints blurred; a user who wants it bigger raises `max_height`. |
| 4 | Unmeasurable artwork (WebP, an SVG with no dimensions) degrades to a bounded box rather than failing. Nothing about a company mark is regulated content, so a working logo beats a refused build. A missing FILE is still an error, matching how library artwork is treated. |
| 5 | The style attribute is built in Go from measured numbers and typed `template.CSS`. `html/template` blanks a style it cannot verify, and going through validated floats means no user string reaches a CSS context. The `imageURI` helper does the same for `src`. |
| 6 | Embedded as a `data:` URI like the pictograms, so the sheet stands alone; over 1 MB encoded, `generate` warns on stderr rather than refusing. |
| 7 | Lengths are parsed to millimetres and require a unit (`internal/config/length.go`) — a bare number on a printed document is ambiguous. |
| 8 | Existence of the logo file is not checked at load: `Load` runs for every command, and `sections list` has no business failing over a logo. |
