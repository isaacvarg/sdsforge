# `document.yaml` reference

Every document sdsforge manages has one `document.yaml`, living at
`~/.local/share/sdsforge/documents/<id>/document.yaml`. `sdsforge document
create <name>` writes a starting copy, annotated with every section, preset
and variant currently in the content library; `sdsforge document edit <id>`
opens it and re-reads it once you close your editor, reporting any parse
error straight away. Unknown keys are rejected, so a typo in a field name is
caught the same way.

This page covers every field. For the two most-asked-about pieces —
Proposition 65 warnings and the exposure-limits table — see
[California Proposition 65](#california-proposition-65-prop65) and
[Exposure limits (the "Basis" column)](#exposure-limits-the-basis-column).

## Top-level keys

| Key | Type | Purpose |
| --- | --- | --- |
| `product_name` | string | The product's name. Feeds Section 1's product identifier. |
| `hazard_codes` | list of strings | GHS hazard codes for the product as a whole. **The main control** — see [Hazard codes](#hazard-codes). |
| `materials` | list | The composition table for Section 3. See [Materials](#materials). |
| `identification` | mapping | Product codes, synonyms, CAS number, recommended use. See [Identification](#identification). |
| `precautionary_text` | mapping | Exact wording for manufacturer-specified precautionary statements. See [Precautionary text](#precautionary-text). |
| `sections` | mapping | Per-section/subsection overrides — presets, variants, replace, append. See [The `sections:` override system](#the-sections-override-system). |
| `prop65` | list | California Proposition 65 warnings. See [California Proposition 65](#california-proposition-65-prop65). |
| `right_to_know` | list | State Right-to-Know disclosures. See [State Right-to-Know](#state-right-to-know). |
| `sara_hazards` | list | SARA 311/312 hazard-category disclosures. See [SARA 311/312](#sara-311312). |
| `supplier` | mapping | **Deprecated, ignored.** Supplier and emergency-contact details come from your config file instead — run `sdsforge config init` and fill in `[company]` and `[[emergency.contacts]]`. Kept only so documents written before this change still load; `document generate` warns if it finds one populated. |

A field left out entirely (or an empty list/mapping) falls back to the
content library's default wording — a minimal `document.yaml` can be just
`product_name` and `hazard_codes`.

## Hazard codes

```yaml
hazard_codes: [H314, H318]
```

This is the main control. Section 2 (classification, signal word,
pictograms, precautionary statements) is computed entirely from these codes,
and they also auto-select the wording of Sections 4, 6, 8 and 11 wherever a
variant declares an `applies_when` that matches. Codes may be written as
`H315`, `h315`, or a bare `315` — YAML would otherwise parse the last form as
an integer, so both work. Codes can also be given per material (see below);
the two sets are combined.

Run `sdsforge document classify <id>` to see exactly what a given set of
codes produces before generating anything.

## Materials

```yaml
materials:
  - name: Sodium hydroxide
    cas_number: "1310-73-2"
    percentage: "50"
    hazard_codes: [H314]
```

Each entry becomes a row in Section 3's ingredient table (`Chemical name`,
`CAS No.`, `Concentration (% w/w)`). Fields:

| Field | Purpose |
| --- | --- |
| `name` | Chemical name. |
| `cas_number` | CAS registry number (quote it — a bare number like `108-88-3` parses fine, but leading-zero CAS numbers need quoting). |
| `percentage` | Free text, e.g. `"50"` or `"10-30"`. |
| `hazard_codes` | GHS codes for this material specifically; unioned into the document's overall hazard set. |

`sequence` and `hazards_triggered` also exist on this struct for
backward-compatibility with documents written before `hazard_codes` was
added, but neither is read for anything — use `hazard_codes`.

## Identification

```yaml
identification:
  product_codes: ["SKU-1234"]
  synonyms: "Caustic soda"
  cas_number: "1310-73-2"
  recommended_use: "Industrial cleaning agent"
  restrictions: "Not for consumer use"
```

Feeds Section 1's product identifier and recommended-use subsections.
Section 1 expects both `recommended_use` and `restrictions` filled in.

## Precautionary text

```yaml
precautionary_text:
  P260: "Do not breathe mist or spray."
  P280: "Wear protective gloves and eye protection."
```

A P-code → wording map. Some precautionary statements are
manufacturer-specified rather than fixed regulatory text; `sdsforge document
classify <id>` marks those with an asterisk so you know which ones need an
entry here.

## Section 15 disclosures

Section 15 ("Regulatory information") has seven subsections. Three are
driven directly by document.yaml fields; the rest (`us_federal`, `sara_313`,
`state`, `inventories`) are static library prose, customizable only through
the generic [`sections:` override](#the-sections-override-system) — there is
no dedicated field for SARA 313, for instance.

| Subsection | Driven by |
| --- | --- |
| US federal regulations | library default / `sections:` override only |
| SARA 311/312 hazard categories | `sara_hazards` |
| SARA 313 | library default / `sections:` override only |
| State regulations | library default / `sections:` override only |
| State Right to Know | `right_to_know` |
| California Proposition 65 | `prop65` |
| International inventories | library default / `sections:` override only |

### California Proposition 65 (`prop65`)

```yaml
prop65:
  - chemical: "Carbon black"
    exposure: carcinogen
  - chemical: "Toluene"
    exposure: reproductive_toxicant
```

One entry per chemical requiring a Cal. Prop 65 disclosure (Cal. Code Regs.
tit. 27, §25603). Fields:

- `chemical` — the chemical's name.
- `exposure` — one of `carcinogen`, `reproductive_toxicant`, or `both`.
  (`reproductive toxicant`, with a space, and plural forms are also
  accepted.) An unrecognized value is silently dropped rather than failing
  the whole document.

This renders the Prop 65 warning symbol plus a caption composed to match the
regulation's required wording, which varies with how many chemicals are
listed and which hazard type(s) they trigger — for example, a single
carcinogen produces:

> This product can expose you to Carbon black, which is known to the State
> of California to cause cancer. For more information go to
> www.P65Warnings.ca.gov.

A chemical listed as `both` (or triggering both an entry under
`carcinogen` and one under `reproductive_toxicant`) is folded into a
single combined-hazard sentence when the two lists match exactly, and
into a two-clause sentence otherwise. Leave `prop65` empty or omit it and
Section 15 states instead:

> No components of this product are known to the State of California to
> cause cancer or reproductive harm.

### State Right-to-Know (`right_to_know`)

```yaml
right_to_know:
  - chemical: "Toluene"
    cas_number: "108-88-3"
    nj: true
    pa: true
    ca: false
```

One entry per chemical. `chemical` and `cas_number` are ordinary fields;
state flags are **plain sibling keys**, not a nested map — write a lowercase
two-letter USPS postal code (`nj`, `pa`, `ca`, `dc`, …) as a key set to
`true` or `false` directly alongside `chemical`/`cas_number`. Only
recognized US state/DC codes flagged `true` count.

This produces one table per state that ends up with at least one chemical
flagged `true`, titled `"<State> Right to Know"` with columns `Chemical
name` / `CAS No.`, sorted alphabetically by state name. Omit `right_to_know`
(or flag nothing `true`) and Section 15 states instead:

> No components of this product are subject to state Right-to-Know
> disclosure requirements.

### SARA 311/312 (`sara_hazards`)

```yaml
sara_hazards:
  - chemical: "Toluene"
    cas_number: "108-88-3"
    hazard: "Fire hazard"
  - chemical: "Toluene"
    cas_number: "108-88-3"
    hazard: "Immediate (acute) health hazard"
```

One entry per chemical/hazard-category pair. A chemical with more than one
hazard category just gets more than one entry — repeat the chemical and CAS
number, changing only `hazard`. This produces Section 15's SARA 311/312
table (`Chemical name` / `CAS No.` / `Hazard category`). Leave it empty and
the table falls back to a "No SARA 311/312 hazard categories identified"
placeholder row.

SARA 313 is separate, static prose with no document.yaml field of its own —
override it via `sections.regulatory.subsections.sara_313` if it needs to
change for a given product.

## Exposure limits (the "Basis" column)

There is **no `exposure_limits` field in `document.yaml`.** Section 8's
exposure-limits table is ordinary section content — the same generic `table`
content block every other tabular subsection uses — so it's set through the
`sections:` override mechanism described below, not through a dedicated
top-level key.

The default table is a single placeholder row with a `Basis` column (any
authority — OSHA PEL, ACGIH TLV, a state limit, a manufacturer AEL — cited
as free text) and an `Exposure Limit` column next to it. Give a chemical
each exposure limit it has as its own row:

```yaml
sections:
  exposure_controls:
    subsections:
      exposure_limits:
        replace:
          kind: table
          headers: ["Chemical name", "CAS No.", "Basis", "Exposure Limit"]
          rows:
            - ["Acetone", "67-64-1", "OSHA PEL (TWA)", "1000 ppm"]
            - ["Acetone", "67-64-1", "ACGIH TLV (TWA)", "750 ppm"]
            - ["Toluene", "108-88-3", "OSHA PEL (TWA)", "200 ppm"]
```

`exposure_controls` is the section id (Section 8), `exposure_limits` is the
subsection id, and `replace` supplies a whole new `table` block — headers
plus rows, every cell a plain string ("N/E", "not established", etc. are all
valid cell values; nothing is parsed as a number).

## The `sections:` override system

Every section resolves to library defaults unless a document overrides it.
The scaffold `document create` writes lists every section's real presets and
subsection variants as commented-out examples, generated live from the
content library at the moment the document was created — run `sdsforge
sections list [section-id]` any time to see the current options, and
`sdsforge sections validate` to check a custom library layer.

```yaml
sections:
  first_aid:
    variant: corrosive
    subsections:
      inhalation: { variant: acute_toxicity }
      ingestion:  { append: ["Do not induce vomiting."] }
      general:    { replace: { kind: prose, text: ["Custom guidance here."] } }
```

- **`sections.<section-id>`** — keyed by the section's stable **id**
  (`first_aid`, `exposure_controls`, `regulatory`, …), never by number or
  directory name, so renumbering the library never invalidates a saved
  document. See the [section reference](#section-reference) below for every
  id.
- **`variant`** — names a *preset*: a bundle of per-subsection picks for the
  whole section (e.g. a `corrosive` preset for First-aid measures). Leave it
  out and every subsection falls back to its own default (or whatever
  `hazard_codes` derives).
- **`subsections.<subsection-id>.variant`** — overrides a single
  subsection's variant, beating whatever the preset (or hazard-code
  derivation) chose.
- **`subsections.<subsection-id>.replace`** — discards the resolved content
  for that subsection entirely and substitutes this block.
- **`subsections.<subsection-id>.append`** — adds this block on top of
  whatever content survived (variant, document data, etc.), instead of
  replacing it.

`replace` and `append` both take a *content block*. There are four kinds; a
subsection only accepts the one kind its manifest declares (a `prose`
subsection can't be replaced with a `table`, for example):

```yaml
# prose — one paragraph per list entry
kind: prose
text:
  - "First paragraph."
  - "Second paragraph."

# table — every cell is a string
kind: table
headers: ["Chemical", "CAS No.", "Basis", "Exposure Limit"]
rows:
  - ["Acetone", "67-64-1", "OSHA PEL (TWA)", "1000 ppm"]

# images
kind: images
images:
  - src: "path/to/image.svg"
    alt: "Description for screen readers"
    caption: "Printed under the image"

# tables — several independently-headed tables, e.g. one per state
kind: tables
tables:
  - title: "New Jersey Right to Know"
    headers: ["Chemical name", "CAS No."]
    rows:
      - ["Toluene", "108-88-3"]
```

For `prose` subsections only, `append` (and `replace`) accept a shorthand: a
bare list of strings, or a single bare string, is treated as `text:` — this
is what `append: ["Do not induce vomiting."]` above is doing. Table, image
and named-table blocks must always use the full `kind:`-tagged form, since
there's no shorthand to infer a table's headers from.

Resolution order for one subsection, lowest to highest precedence:

```
default → derived (from hazard_codes) → preset variant
        → per-subsection variant → source (document data)
        → replace → append
```

"Source" is how document fields like `materials`, `prop65`,
`right_to_know` and `sara_hazards` reach their subsections in the first
place — a subsection that declares a `source` uses the document's data
there once it beats the authored variant, but before any explicit
`replace`/`append` an author wrote.

## Section reference

| id | # | Title |
| --- | --- | --- |
| `identification` | 1 | Identification |
| `hazards` | 2 | Hazard(s) identification |
| `composition` | 3 | Composition/information on ingredients |
| `first_aid` | 4 | First-aid measures |
| `firefighting` | 5 | Fire-fighting measures |
| `accidental_release` | 6 | Accidental release measures |
| `handling_storage` | 7 | Handling and storage |
| `exposure_controls` | 8 | Exposure controls/personal protection |
| `properties` | 9 | Physical and chemical properties |
| `stability` | 10 | Stability and reactivity |
| `toxicological` | 11 | Toxicological information |
| `ecological` | 12 | Ecological information |
| `disposal` | 13 | Disposal considerations |
| `transport` | 14 | Transport information |
| `regulatory` | 15 | Regulatory information |
| `other` | 16 | Other information |

Use these ids as the keys under `sections:`. Run `sdsforge sections list
<section-id>` to see a section's actual subsection ids, presets and
variants.

## Full example

```yaml
product_name: Acetone-Toluene Blend

hazard_codes: [H225, H319, H336]

identification:
  product_codes: ["ATB-100"]
  synonyms: "Solvent blend"
  cas_number: ""
  recommended_use: "Industrial degreasing solvent"
  restrictions: "Not for consumer use"

precautionary_text:
  P210: "Keep away from heat, hot surfaces, sparks, open flames."

materials:
  - name: Acetone
    cas_number: "67-64-1"
    percentage: "60"
    hazard_codes: [H225, H319, H336]
  - name: Toluene
    cas_number: "108-88-3"
    percentage: "40"
    hazard_codes: [H225, H304, H315, H336, H361, H373]

prop65:
  - chemical: "Toluene"
    exposure: reproductive_toxicant

right_to_know:
  - chemical: "Toluene"
    cas_number: "108-88-3"
    nj: true
    pa: true
  - chemical: "Acetone"
    cas_number: "67-64-1"
    nj: true

sara_hazards:
  - chemical: "Toluene"
    cas_number: "108-88-3"
    hazard: "Fire hazard"
  - chemical: "Toluene"
    cas_number: "108-88-3"
    hazard: "Immediate (acute) health hazard"

sections:
  exposure_controls:
    subsections:
      exposure_limits:
        replace:
          kind: table
          headers: ["Chemical name", "CAS No.", "Basis", "Exposure Limit"]
          rows:
            - ["Acetone", "67-64-1", "OSHA PEL (TWA)", "1000 ppm"]
            - ["Acetone", "67-64-1", "ACGIH TLV (TWA)", "750 ppm"]
            - ["Toluene", "108-88-3", "OSHA PEL (TWA)", "200 ppm"]
            - ["Toluene", "108-88-3", "ACGIH TLV (TWA)", "20 ppm"]
```
