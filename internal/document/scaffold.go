package document

import (
	"fmt"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

// Scaffold renders an annotated document.yaml for a new document.
//
// Everything below `sections:` is generated from the LIVE content library --
// section ids, presets, and per-subsection variants are read at the moment the
// document is created. The template therefore cannot drift as the library
// grows, and a user never has to guess what is selectable.
//
// The generated file is text rather than a marshalled struct because YAML
// comments are the whole point, and they are what makes the feature
// discoverable. A round-trip test decodes the result with KnownFields(true) to
// guarantee every key still matches the Data struct.
//
// # Comment convention
//
// The file mixes prose with commented-out YAML, so the two must be
// distinguishable at a glance and by a script:
//
//	"# text"      explanatory -- exactly one space after the hash
//	"#  yaml:"    enabled by deleting the single leading hash; two or more
//	              spaces follow, because this YAML is nested and indented
//
// Nothing that is not valid YAML may use the second form.
func Scaffold(lib *sections.Library, name string) ([]byte, error) {
	layout, err := sections.LoadLayout(lib)
	if err != nil {
		return nil, err
	}

	var b strings.Builder

	fmt.Fprintf(&b, `# Safety Data Sheet source document.
#
# Fill in the fields below, then run:  sdsforge document generate <id>
#
# Anything left blank falls back to the content library's default wording.
# Run 'sdsforge sections list' to see every section, or
# 'sdsforge sections list <section-id>' to drill into one.

product_name: %s

# The version number, issue date and revision history are NOT set here. They are
# recorded when you issue a version:
#
#     sdsforge document version create <id> --minor -m "what changed"
#
# Run 'sdsforge document version list <id>' to see them.

# Section 1. Feeds the "Product identifier" subsection.
identification:
  product_codes: []
  synonyms: ""
  cas_number: ""
  recommended_use: ""
  restrictions: ""

# Section 1's "Supplier details" and "Emergency telephone number" come from
# your config file, not from here -- they are the same on every sheet you
# issue. Run 'sdsforge config init', then fill in [company] and
# [[emergency.contacts]] there once.

# GHS hazard codes for the product. THIS IS THE MAIN CONTROL.
#
# Section 2 is computed entirely from these -- hazard statements, signal word,
# pictograms and precautionary statements -- and they also select the wording of
# sections 4, 6, 8 and 11 automatically. You normally do not need to touch the
# "sections:" block below at all.
#
#     hazard_codes: [H314, H318]
#
# Run 'sdsforge document classify <id>' to see exactly what they produce.
# Codes may also be given per material below; the two sets are combined.
hazard_codes: []

# Concrete wording for precautionary statements whose regulatory text is
# manufacturer-specified. 'document classify' marks those with an asterisk.
#
#     precautionary_text:
#       P260: "Do not breathe mist or spray."
precautionary_text: {}

# Section 3. Each entry becomes a row in the ingredients table, e.g.
# - {name: Sodium hydroxide, cas_number: "1310-73-2", percentage: "50", hazard_codes: [H314]}
# hazard_codes drive nothing yet; they are the input for automatic variant
# selection once that lands.
materials: []

# ---------------------------------------------------------------------------
# Section content
#
# Most of this is unnecessary if you set hazard_codes above -- these sections
# derive their wording from the codes automatically. Use this block to override
# what was derived, or when you are not using hazard codes at all.
#
# Every section resolves to its default wording unless you say otherwise.
# Each entry below is commented out. To enable one, delete ONLY the leading
# "#" -- the spaces after it are YAML indentation and must stay.
#
# Within a section, "variant: <preset>" picks a whole-section preset. To adjust
# one subsection instead, nest it under "subsections:" and give it any of:
# "variant: <name>" to swap wording, "append: [...]" to add to it, or
# "replace: [...]" to discard it. A per-subsection choice beats the preset.
# ---------------------------------------------------------------------------
sections:
`, yamlString(name))

	for _, dir := range layout.Sections {
		def, err := sections.LoadSection(lib, dir)
		if err != nil {
			return nil, err
		}
		block, err := scaffoldSection(lib, def)
		if err != nil {
			return nil, err
		}
		b.WriteString(block)
	}

	return []byte(b.String()), nil
}

// scaffoldSection renders one commented section entry: a prose header
// describing what is available, then the enableable YAML.
func scaffoldSection(lib *sections.Library, def sections.SectionDef) (string, error) {
	presets, err := lib.ListPresets(def.Dir)
	if err != nil {
		return "", err
	}

	// Only mention subsections that offer a real choice or accept document
	// data; listing every subsection would bury the interesting ones.
	var (
		notable    []string
		firstWith  string // first subsection offering a variant choice
		firstProse string // first prose subsection, for an append example
	)
	for _, sub := range def.Subsections {
		if sub.Kind == "prose" && firstProse == "" {
			firstProse = sub.ID
		}
		variants, err := lib.ListVariants(def.Dir, sub.ID)
		if err != nil {
			return "", err
		}
		switch {
		case len(variants) > 1:
			notable = append(notable, fmt.Sprintf("# - %s: %s",
				sub.ID, strings.Join(variants, " | ")))
			if firstWith == "" {
				firstWith = sub.ID
			}
		case sub.Source != "":
			notable = append(notable, fmt.Sprintf("# - %s: filled from `%s` above",
				sub.ID, sub.Source))
		}
	}

	var b strings.Builder

	// Prose header: single space after the hash throughout.
	fmt.Fprintf(&b, "\n# %d. %s\n", def.Number, def.Title)
	if len(presets) > 0 {
		fmt.Fprintf(&b, "# presets: %s\n", strings.Join(presets, ", "))
	}
	if len(notable) > 0 {
		b.WriteString("# subsections with options:\n")
		for _, line := range notable {
			b.WriteString(line + "\n")
		}
	}
	if len(presets) == 0 && len(notable) == 0 {
		b.WriteString("# defaults only; adjust with replace or append\n")
	}

	// Enableable YAML: "#" then the exact indented line.
	//
	// The example must RESOLVE, not merely parse. The bare-list append
	// shorthand is prose, so offering it for a table subsection would hand the
	// user an example that fails the moment they enable it.
	fmt.Fprintf(&b, "#  %s:\n", def.ID)
	switch {
	case len(presets) > 0:
		fmt.Fprintf(&b, "#    variant: %s\n", presets[0])
	case firstWith != "":
		fmt.Fprintf(&b, "#    subsections:\n#      %s:\n#        variant: default\n", firstWith)
	case firstProse != "":
		fmt.Fprintf(&b, "#    subsections:\n#      %s:\n#        append: [\"...\"]\n", firstProse)
	default:
		// Every subsection here is tabular, and a table override needs
		// explicit headers and a matching row width, which no generic example
		// can supply. Enabling the bare key is valid and resolves to defaults.
		b.WriteString("# every subsection here is a table; override with an explicit\n")
		b.WriteString("# block, e.g. replace: {kind: table, headers: [...], rows: [[...]]}\n")
	}

	return b.String(), nil
}

// yamlString quotes a value when it would otherwise be ambiguous YAML.
func yamlString(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`") || strings.TrimSpace(s) != s {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}
