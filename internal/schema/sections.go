package schema

import (
	"fmt"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

// buildSections builds the `sections:` branch and every definition under it.
//
// The point of generating rather than hand-writing the schema is concentrated
// here. Each section id gets its own definition carrying only what is valid
// THERE: its own presets in `variant`, its own subsection ids as keys, and for
// each subsection only the variant files that exist on disk. A flat schema would
// accept `first_aid.subsections.skin.variant: flammable_liquid`, which resolve
// rejects at generate time; this one rejects it while you type.
func buildSections(lib *sections.Library, layout sections.Layout) (*Node, map[string]*Node, error) {
	defs := map[string]*Node{}
	props := map[string]*Node{}

	for _, dir := range layout.Sections {
		def, err := sections.LoadSection(lib, dir)
		if err != nil {
			return nil, nil, err
		}
		presets, err := lib.ListPresets(dir)
		if err != nil {
			return nil, nil, err
		}

		subProps := map[string]*Node{}
		for _, sub := range def.Subsections {
			variants, err := lib.ListVariants(def.Dir, sub.ID)
			if err != nil {
				return nil, nil, err
			}
			name := subsectionDefName(def.ID, sub.ID)
			defs[name] = subsectionNode(def, sub, variants)
			subProps[sub.ID] = ref(name)
		}

		name := sectionDefName(def.ID)
		defs[name] = sectionNode(def, presets, subProps)
		props[def.ID] = ref(name)
	}

	node := &Node{
		Type:  orNull("object"),
		Title: "Section overrides",
		Description: "Per-section choices, keyed by the section's stable id -- never by number or directory name, so renumbering the library " +
			"never invalidates a saved document. A section left out resolves entirely to its defaults.",
		Properties:           props,
		AdditionalProperties: denyExtra(),
	}
	return node, defs, nil
}

func sectionDefName(sectionID string) string { return "section_" + sectionID }

func subsectionDefName(sectionID, subID string) string {
	return "sub_" + sectionID + "_" + subID
}

func sectionNode(def sections.SectionDef, presets []string, subProps map[string]*Node) *Node {
	props := map[string]*Node{
		"subsections": {
			Type:                 orNull("object"),
			Description:          "Overrides for individual subsections. These beat whatever the preset, or hazard-code derivation, chose.",
			Properties:           subProps,
			AdditionalProperties: denyExtra(),
		},
	}

	desc := fmt.Sprintf("Section %d. %s.", def.Number, def.Title)
	if len(presets) > 0 {
		props["variant"] = &Node{
			Type: "string",
			Description: "A preset: a bundle of per-subsection picks for the whole section. Leave it out and every subsection falls back to " +
				"its own default, or to whatever hazard_codes derives.",
			Enum: anyStrings(presets),
		}
		desc += fmt.Sprintf(" Presets: %s.", strings.Join(presets, ", "))
	} else {
		// Naming a preset here would fail with ErrUnknownPreset at generate
		// time, so `variant` is simply not a key this section has. Saying why in
		// the description turns "property variant is not allowed" from a riddle
		// into an answer.
		desc += " This section has no presets, so it takes no `variant` -- override its subsections individually instead."
	}

	return &Node{
		Type:                 orNull("object"),
		Title:                fmt.Sprintf("%d. %s", def.Number, def.Title),
		Description:          desc,
		Properties:           props,
		AdditionalProperties: denyExtra(),
	}
}

// subsectionNode builds one subsection's override schema, constrained to the
// content kind its manifest declares.
//
// That constraint is not decorative: resolveSubsection rejects a replace or
// append block whose Kind() disagrees with the manifest, so a table dropped into
// a prose subsection is an error either way -- better to see it in the editor
// than at generate time.
func subsectionNode(def sections.SectionDef, sub sections.SubsectionDef, variants []string) *Node {
	block := blockNode(sub.Kind)

	desc := sub.Title + "."
	if sub.Source != "" {
		desc += fmt.Sprintf(" Populated from the document's own data (source: %s) when there is any; an explicit replace or append still wins.", sub.Source)
	}

	props := map[string]*Node{
		"replace": withDescription(block,
			"Discard the resolved content entirely and use this instead. Must be "+kindArticle(sub.Kind)+" block, matching the subsection's declared kind."),
		"append": withDescription(block,
			"Add to whatever content survived -- the variant, or the document's own data. Must be "+kindArticle(sub.Kind)+" block, matching the subsection's declared kind."),
	}
	if len(variants) > 0 {
		props["variant"] = &Node{
			Type:        "string",
			Description: "Pick a different variant for this subsection than the preset or hazard-code derivation chose.",
			Enum:        anyStrings(variants),
		}
	}

	return &Node{
		Type:                 orNull("object"),
		Title:                sub.Title,
		Description:          desc,
		Properties:           props,
		AdditionalProperties: denyExtra(),
	}
}

// blockNode is the schema for a content block of one kind.
//
// Only prose gets a oneOf. Block.UnmarshalYAML accepts a bare string or a bare
// list of strings as shorthand, but both decode to PROSE -- there is no `kind`
// to dispatch on -- so the shorthand is simply not valid anywhere else. Keeping
// the three branches type-disjoint (string, array, object) also keeps
// yaml-language-server's completion working; a oneOf of several object shapes is
// what degrades it.
func blockNode(kind string) *Node {
	switch kind {
	case "prose":
		return &Node{
			OneOf: []*Node{
				{Type: "string", Description: "Shorthand for a single paragraph."},
				{Type: "array", Description: "Shorthand for one paragraph per entry.", Items: &Node{Type: "string"}},
				ref("block_prose"),
			},
		}
	case "table":
		return ref("block_table")
	case "tables":
		return ref("block_tables")
	case "images":
		return ref("block_images")
	default:
		// Unreachable through the shipped library: SectionDef.validate rejects
		// a manifest whose kind is not in the registry. A custom layer that
		// registers a new kind would land here, and an unconstrained node is the
		// right answer -- better to permit what we cannot describe than to
		// declare a user's own content invalid.
		return &Node{Description: "Content of kind " + kind + "."}
	}
}

// withDescription copies a node so the same block schema can be reused for
// `replace` and `append` with different wording. Shallow is enough: nothing
// mutates the nested nodes afterwards.
func withDescription(n *Node, desc string) *Node {
	copied := *n
	copied.Description = desc
	return &copied
}

func kindArticle(kind string) string {
	if kind == "images" {
		return "an " + kind
	}
	return "a " + kind
}

func anyStrings(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}
