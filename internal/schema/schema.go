// Package schema generates a JSON Schema for document.yaml.
//
// The interesting parts of that format are not in any Go struct: which section
// ids exist, which presets a section offers, and which variants a subsection
// has are all FILENAMES in the content library. So the schema is generated from
// a live [sections.Library] rather than hand-written, the same way
// [document.Scaffold] generates the annotated starter document -- and for the
// same reason. A hand-maintained schema would be wrong within two commits.
//
// The output is committed to docs/document.schema.json so editors have a stable
// path and URL to point at, and TestCommittedSchemaIsCurrent fails when the two
// disagree.
package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/ghs"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

const (
	// Dialect is the JSON Schema version the output declares. 2020-12 is what
	// yaml-language-server implements.
	Dialect = "https://json-schema.org/draft/2020-12/schema"

	// CanonicalURL is where the generated file is published. It doubles as the
	// schema's $id and as the fallback an editor can fetch on a machine with no
	// checkout -- see docs/editor-setup.md.
	CanonicalURL = "https://raw.githubusercontent.com/isaacvarg/sdsforge/main/docs/document.schema.json"
)

// Node is one JSON Schema node.
//
// Modelled as a struct rather than a map[string]any so each node's keys come
// out in a fixed, readable order. Determinism is not cosmetic here: the golden
// test compares bytes, so anything that varies run to run would make it useless.
type Node struct {
	Schema      string `json:"$schema,omitempty"`
	ID          string `json:"$id,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Deprecated  bool   `json:"deprecated,omitempty"`

	// Type is a string, or a []string when a value may also be null. YAML lets
	// any key be written bare -- `sections:` with nothing under it -- and
	// yaml.v3 decodes that to the zero value without complaint, which the
	// scaffold relies on. Every optional container therefore has to accept null,
	// or a freshly created document would be flagged in its entirety.
	Type    any    `json:"type,omitempty"`
	Const   any    `json:"const,omitempty"`
	Enum    []any  `json:"enum,omitempty"`
	Pattern string `json:"pattern,omitempty"`

	// EnumDescriptions is a yaml-language-server extension, not part of the
	// 2020-12 vocabulary: it puts documentation on individual enum members in
	// the completion list. A validator that does not know it ignores it, which
	// is why this is preferred over an anyOf of 80 const branches -- that would
	// carry the same documentation at the cost of an unreadable error message
	// every time a code is wrong.
	EnumDescriptions []string `json:"enumDescriptions,omitempty"`

	Items      *Node            `json:"items,omitempty"`
	Properties map[string]*Node `json:"properties,omitempty"`
	Required   []string         `json:"required,omitempty"`

	// PropertyNames constrains the shape of keys on an open map, catching a
	// malformed key that additionalProperties alone would wave through.
	PropertyNames *Node `json:"propertyNames,omitempty"`

	AdditionalProperties *AdditionalProperties `json:"additionalProperties,omitempty"`

	AnyOf []*Node `json:"anyOf,omitempty"`
	OneOf []*Node `json:"oneOf,omitempty"`

	Examples []any            `json:"examples,omitempty"`
	Defs     map[string]*Node `json:"$defs,omitempty"`

	// DoNotSuggest and DefaultSnippets are the other two yaml-language-server
	// extensions worth carrying: the first keeps a deprecated key valid but out
	// of the completion list, the second offers a filled-in skeleton for a shape
	// nobody remembers (a table's headers and rows, say).
	DoNotSuggest    bool      `json:"doNotSuggest,omitempty"`
	DefaultSnippets []Snippet `json:"defaultSnippets,omitempty"`
}

// Snippet is one entry of the defaultSnippets extension.
type Snippet struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Body        any    `json:"body"`
}

// AdditionalProperties is JSON Schema's `additionalProperties`, which is either
// a boolean or a schema. Go has no union, so this marshals one or the other.
type AdditionalProperties struct {
	Schema *Node
}

// MarshalJSON writes the nested schema when there is one, and `false` otherwise.
func (a AdditionalProperties) MarshalJSON() ([]byte, error) {
	if a.Schema != nil {
		return json.Marshal(a.Schema)
	}
	return []byte("false"), nil
}

// denyExtra closes an object to keys it does not declare, which is what turns a
// typo into a diagnostic instead of a silently ignored line.
func denyExtra() *AdditionalProperties { return &AdditionalProperties{} }

// allowExtra leaves an object open, constraining whatever else appears.
func allowExtra(n *Node) *AdditionalProperties { return &AdditionalProperties{Schema: n} }

// orNull is the type of an optional value that may also be written as a bare
// key. See the Type field's comment.
func orNull(t string) []string { return []string{t, "null"} }

func str(desc string) *Node { return &Node{Type: "string", Description: desc} }
func ref(name string) *Node { return &Node{Ref: "#/$defs/" + name} }
func strs(desc string) *Node {
	return &Node{Type: orNull("array"), Description: desc, Items: &Node{Type: "string"}}
}

// rows is the shape every table body shares: a list of rows, each a list of
// string cells. Deliberately not typed further -- SDS tables are full of values
// like "1000 ppm" and "not established" that are not numbers.
func rows() *Node {
	return &Node{
		Type:        orNull("array"),
		Description: "Rows, each a list of string cells. Nothing is parsed as a number: \"1000 ppm\" and \"N/E\" are ordinary cell values.",
		Items:       &Node{Type: "array", Items: &Node{Type: "string"}},
	}
}

// Generate builds the schema for the format the given library describes and
// returns it as formatted JSON with a trailing newline.
func Generate(lib *sections.Library) ([]byte, error) {
	layout, err := sections.LoadLayout(lib)
	if err != nil {
		return nil, err
	}
	tables, err := ghs.LoadTables(lib)
	if err != nil {
		return nil, err
	}

	defs := map[string]*Node{
		"hazard_code":         hazardCodeNode(tables),
		"material":            materialNode(),
		"identification":      identificationNode(),
		"supplier":            supplierNode(),
		"prop65_warning":      prop65Node(),
		"right_to_know_entry": rightToKnowNode(),
		"sara_hazard":         saraHazardNode(),
		"block_prose":         proseBlockNode(),
		"block_table":         tableBlockNode(),
		"block_tables":        tablesBlockNode(),
		"block_images":        imagesBlockNode(),
	}

	sectionsNode, sectionDefs, err := buildSections(lib, layout)
	if err != nil {
		return nil, err
	}
	for name, node := range sectionDefs {
		if _, clash := defs[name]; clash {
			return nil, fmt.Errorf("schema: duplicate definition %q", name)
		}
		defs[name] = node
	}

	root := &Node{
		Schema:      Dialect,
		ID:          CanonicalURL,
		Title:       "sdsforge document.yaml",
		Description: "The source document for one Safety Data Sheet. Generated from the " + layout.Jurisdiction + " content library; see docs/document-yaml.md for the prose reference.",
		Type:        "object",
		Required:    []string{"product_name"},
		Properties: map[string]*Node{
			"product_name": str("The product's name. Feeds Section 1's product identifier, and titles the rendered sheet."),
			"hazard_codes": {
				Type: orNull("array"),
				Description: "GHS hazard codes for the product as a whole. This is the main control: Section 2 is computed entirely from these, " +
					"and they auto-select the wording of Sections 4, 6, 8 and 11. Run `sdsforge document classify <id>` to see what a set produces.",
				Items:    ref("hazard_code"),
				Examples: []any{[]string{"H314", "H318"}},
			},
			"materials": {
				Type:        orNull("array"),
				Description: "The composition table for Section 3. One entry per ingredient.",
				Items:       ref("material"),
			},
			"identification":     ref("identification"),
			"precautionary_text": precautionaryTextNode(tables),
			"sections":           sectionsNode,
			"prop65": {
				Type:        orNull("array"),
				Description: "California Proposition 65 warnings, one entry per chemical requiring disclosure under Cal. Code Regs. tit. 27 §25603. Omit it and Section 15 states that no components are known to require a warning.",
				Items:       ref("prop65_warning"),
			},
			"right_to_know": {
				Type:        orNull("array"),
				Description: "State Right-to-Know disclosures. Produces one table per state that ends up with at least one chemical flagged true.",
				Items:       ref("right_to_know_entry"),
			},
			"sara_hazards": {
				Type:        orNull("array"),
				Description: "SARA 311/312 hazard-category disclosures. A chemical with more than one hazard category gets one entry per hazard.",
				Items:       ref("sara_hazard"),
			},
			"supplier": ref("supplier"),
		},
		AdditionalProperties: denyExtra(),
		Defs:                 defs,
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	// Chemical names and regulatory prose contain <, > and & often enough that
	// escaping them would make the file unreadable in a diff.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// hazardCodeNode enumerates the reference table.
//
// Unknown codes are a hard error in ghs.Classify, not a silent drop, so the
// schema is strict about WHICH codes exist. It is lenient only about their
// FORM: NormalizeCode accepts "H315", "h315" and a bare 315, because YAML parses
// the last as an integer and people write all three.
func hazardCodeNode(t *ghs.Tables) *Node {
	codes := t.Codes()

	enum := make([]any, 0, len(codes))
	descs := make([]string, 0, len(codes))
	ints := make([]any, 0, len(codes))
	alternates := make([]byte, 0, len(codes)*4)

	for i, code := range codes {
		enum = append(enum, code)

		desc := code
		if h, ok := t.Lookup(code); ok {
			desc = h.Statement
			if h.Class != "" {
				desc = fmt.Sprintf("%s (%s, category %s)", h.Statement, h.Class, h.Category)
			}
		}
		descs = append(descs, desc)

		// Every code is H followed by three digits -- verified against the
		// table, which holds H200 through H420 and nothing else. The bare and
		// lowercase spellings are built from that numeric part so they stay as
		// strict about the code set as the canonical enum is.
		digits := code[1:]
		var n int
		if _, err := fmt.Sscanf(digits, "%d", &n); err == nil {
			ints = append(ints, n)
		}
		if i > 0 {
			alternates = append(alternates, '|')
		}
		alternates = append(alternates, digits...)
	}

	return &Node{
		Description: "A GHS hazard code. Written as H315, h315, or a bare 315 -- YAML parses the last as an integer, and all three are accepted.",
		AnyOf: []*Node{
			{
				Type:             "string",
				Description:      "Canonical form.",
				Enum:             enum,
				EnumDescriptions: descs,
			},
			{
				Type:        "string",
				Description: "Lowercase form, e.g. h315.",
				Pattern:     "^h(" + string(alternates) + ")$",
			},
			{
				Type:        "integer",
				Description: "Bare form, e.g. 315.",
				Enum:        ints,
			},
		},
	}
}

func materialNode() *Node {
	return &Node{
		Type:        "object",
		Title:       "Material",
		Description: "One row of Section 3's ingredient table.",
		Properties: map[string]*Node{
			"name":       str("Chemical name."),
			"cas_number": str("CAS registry number. Quote it -- a leading-zero CAS number needs quoting to survive YAML."),
			"percentage": str("Concentration as free text, e.g. \"50\" or \"10-30\"."),
			"hazard_codes": {
				Type:        "array",
				Description: "GHS codes for this material specifically. Unioned into the document's overall hazard set.",
				Items:       ref("hazard_code"),
			},
			"sequence": {
				Type:         "integer",
				Deprecated:   true,
				DoNotSuggest: true,
				Description:  "Deprecated and unread. Kept so documents written before hazard_codes existed still load.",
			},
			"hazards_triggered": {
				Type:         "array",
				Deprecated:   true,
				DoNotSuggest: true,
				Description:  "Deprecated and unread. Use hazard_codes instead.",
				Items:        &Node{Type: "integer"},
			},
		},
		AdditionalProperties: denyExtra(),
	}
}

func identificationNode() *Node {
	return &Node{
		Type:        orNull("object"),
		Title:       "Identification",
		Description: "Feeds Section 1's product identifier and recommended-use subsections. Section 1 expects both recommended_use and restrictions filled in.",
		Properties: map[string]*Node{
			"product_codes":   strs("Product codes or SKUs."),
			"synonyms":        str("Other names the product goes by."),
			"cas_number":      str("CAS registry number for the product itself, where it has one."),
			"recommended_use": str("What the product is for, e.g. \"Industrial cleaning agent\"."),
			"restrictions":    str("Restrictions on use, e.g. \"Not for consumer use\"."),
		},
		AdditionalProperties: denyExtra(),
	}
}

func supplierNode() *Node {
	return &Node{
		Type:         orNull("object"),
		Title:        "Supplier (deprecated)",
		Deprecated:   true,
		DoNotSuggest: true,
		Description: "Deprecated and ignored. Supplier and emergency-contact details come from the config file instead -- " +
			"run `sdsforge config init` and fill in [company] and [[emergency.contacts]]. Kept only so older documents still load; " +
			"`document generate` warns when it finds one populated.",
		Properties: map[string]*Node{
			"name":            str("Deprecated. Use [company] name in the config file."),
			"address":         str("Deprecated. Use [company] address in the config file."),
			"phone":           str("Deprecated. Use [company] phone in the config file."),
			"email":           str("Deprecated. Use [company] email in the config file."),
			"emergency_phone": str("Deprecated. Use [[emergency.contacts]] in the config file."),
		},
		AdditionalProperties: denyExtra(),
	}
}

// precautionaryTextNode enumerates only the codes that actually want an entry.
//
// Most precautionary statements are fixed regulatory text; only those flagged
// supplier_specified contain manufacturer-chosen wording, and they are exactly
// the ones `document classify` marks with an asterisk. Listing all 90 would bury
// the two dozen that matter. The map stays open so an unlisted code still
// validates.
func precautionaryTextNode(t *ghs.Tables) *Node {
	props := map[string]*Node{}
	for _, code := range t.PrecautionaryCodes() {
		p, ok := t.LookupPrecautionary(code)
		if !ok || !p.SupplierSpecified {
			continue
		}
		props[code] = &Node{Type: "string", Description: p.Statement}
	}

	return &Node{
		Type:  orNull("object"),
		Title: "Precautionary text",
		Description: "Wording for precautionary statements, keyed by P-code. Any statement the classification selects may be overridden here, " +
			"but the ones that NEED an entry are those whose regulatory text is manufacturer-specified -- the ones " +
			"`sdsforge document classify <id>` marks with an asterisk. Only those are offered as completions; the rest are still accepted. " +
			"An override for a code the classification does not select is an error.",
		Properties: props,
		// Open, because which codes are valid depends on the document's own
		// hazard codes -- ApplyText accepts any SELECTED statement, which no
		// static schema can know. propertyNames still catches a malformed key
		// such as a lowercase p260.
		PropertyNames:        &Node{Pattern: `^P[0-9]{3}(\+P[0-9]{3})*$`},
		AdditionalProperties: allowExtra(&Node{Type: "string"}),
		Examples:             []any{map[string]string{"P260": "Do not breathe mist or spray."}},
	}
}

func prop65Node() *Node {
	values := document.ExposureValues()
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, v)
	}

	return &Node{
		Type:        "object",
		Title:       "Proposition 65 warning",
		Description: "One chemical requiring a California Prop 65 disclosure.",
		Required:    []string{"chemical", "exposure"},
		Properties: map[string]*Node{
			"chemical": str("The chemical's name, as it should appear in the warning sentence."),
			"exposure": {
				Type: "string",
				Description: "Which hazard the chemical is listed for. An unrecognized value is silently dropped rather than failing the document, " +
					"so a typo here costs you the whole warning.",
				Enum: enum,
			},
		},
		AdditionalProperties: denyExtra(),
	}
}

// rightToKnowNode spells out all 51 state keys as ordinary properties.
//
// RightToKnowEntry.States is a yaml:",inline" map, so state flags are plain
// SIBLING keys of chemical/cas_number rather than a nested mapping. Declaring
// each one explicitly is what makes `nj` complete in an editor; patternProperties
// would validate identically and complete nothing, which is the whole point of
// the exercise.
//
// This is the one place the schema is stricter than the CLI: an unrecognized
// state code is currently dropped in silence at render time.
func rightToKnowNode() *Node {
	props := map[string]*Node{
		"chemical":   str("The chemical's name."),
		"cas_number": str("CAS registry number. Quote it."),
	}
	for _, code := range document.StateCodes() {
		name, _ := document.StateName(code)
		props[code] = &Node{
			Type:        "boolean",
			Description: "Subject to " + name + " Right-to-Know disclosure.",
		}
	}

	return &Node{
		Type:  "object",
		Title: "Right-to-Know entry",
		Description: "One chemical's state Right-to-Know disclosure. State flags are plain sibling keys of chemical/cas_number, " +
			"not a nested map: write a lowercase two-letter postal code set to true or false directly alongside them.",
		Required:             []string{"chemical"},
		Properties:           props,
		AdditionalProperties: denyExtra(),
	}
}

func saraHazardNode() *Node {
	return &Node{
		Type:        "object",
		Title:       "SARA 311/312 hazard",
		Description: "One chemical/hazard-category pair. A chemical with several categories gets several entries, changing only `hazard`.",
		Required:    []string{"chemical", "hazard"},
		Properties: map[string]*Node{
			"chemical":   str("The chemical's name."),
			"cas_number": str("CAS registry number. Quote it."),
			"hazard": {
				Type:        "string",
				Description: "The hazard category, e.g. \"Fire hazard\" or \"Immediate (acute) health hazard\".",
				Examples:    []any{"Fire hazard", "Immediate (acute) health hazard", "Delayed (chronic) health hazard"},
			},
		},
		AdditionalProperties: denyExtra(),
	}
}

func proseBlockNode() *Node {
	return &Node{
		Type:        orNull("object"),
		Title:       "Prose block",
		Description: "Paragraphs of text, one per list entry.",
		Required:    []string{"kind", "text"},
		Properties: map[string]*Node{
			"kind": {Type: "string", Const: "prose"},
			"text": strs("One paragraph per entry."),
		},
		AdditionalProperties: denyExtra(),
	}
}

func tableBlockNode() *Node {
	return &Node{
		Type:        orNull("object"),
		Title:       "Table block",
		Description: "A single table. Every cell is a string.",
		Required:    []string{"kind", "headers", "rows"},
		Properties: map[string]*Node{
			"kind":    {Type: "string", Const: "table"},
			"headers": strs("Column headings."),
			"rows":    rows(),
		},
		AdditionalProperties: denyExtra(),
		DefaultSnippets: []Snippet{{
			Label:       "table",
			Description: "A table with one header row and one data row to fill in.",
			Body: map[string]any{
				"kind":    "table",
				"headers": []string{"$1"},
				"rows":    [][]string{{"$2"}},
			},
		}},
	}
}

func tablesBlockNode() *Node {
	return &Node{
		Type:        orNull("object"),
		Title:       "Named tables block",
		Description: "Several independently-headed tables, e.g. one per state.",
		Required:    []string{"kind", "tables"},
		Properties: map[string]*Node{
			"kind": {Type: "string", Const: "tables"},
			"tables": {
				Type: "array",
				Items: &Node{
					Type:     "object",
					Required: []string{"title", "headers", "rows"},
					Properties: map[string]*Node{
						"title":   str("Heading printed above this table."),
						"headers": strs("Column headings."),
						"rows":    rows(),
					},
					AdditionalProperties: denyExtra(),
				},
			},
		},
		AdditionalProperties: denyExtra(),
		DefaultSnippets: []Snippet{{
			Label:       "tables",
			Description: "One named table to fill in.",
			Body: map[string]any{
				"kind": "tables",
				"tables": []map[string]any{{
					"title":   "$1",
					"headers": []string{"$2"},
					"rows":    [][]string{{"$3"}},
				}},
			},
		}},
	}
}

func imagesBlockNode() *Node {
	return &Node{
		Type:        orNull("object"),
		Title:       "Images block",
		Description: "A row of pictures. Artwork referenced by path is embedded as a data: URI when the sheet is generated, so the finished document stands alone.",
		Required:    []string{"kind", "images"},
		Properties: map[string]*Node{
			"kind": {Type: "string", Const: "images"},
			"images": {
				Type: "array",
				Items: &Node{
					Type:     "object",
					Required: []string{"src", "alt"},
					Properties: map[string]*Node{
						"src":     str("Path to the artwork, or a data: URI."),
						"alt":     str("What a screen reader announces, and what a text-only rendering falls back to. Required: an image without it is treated as invalid."),
						"caption": str("Printed under the image."),
					},
					AdditionalProperties: denyExtra(),
				},
			},
		},
		AdditionalProperties: denyExtra(),
		DefaultSnippets: []Snippet{{
			Label:       "images",
			Description: "One image with the alt text it requires.",
			Body: map[string]any{
				"kind":   "images",
				"images": []map[string]any{{"src": "$1", "alt": "$2"}},
			},
		}},
	}
}
