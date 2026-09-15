package sections

import (
	"strings"
	"testing"
)

// realLibrary builds a Library over the embedded OSHA content -- no overlay.
func realLibrary(t *testing.T) *Library {
	t.Helper()
	lib, err := NewLibrary(LibraryOptions{})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}
	return lib
}

func loadDef(t *testing.T, lib *Library, dir string) SectionDef {
	t.Helper()
	def, err := LoadSection(lib, dir)
	if err != nil {
		t.Fatalf("LoadSection(%s) error = %v", dir, err)
	}
	return def
}

// firstParagraph is a small helper so the assertions below stay readable.
func firstParagraph(t *testing.T, sub ResolvedSubsection) string {
	t.Helper()
	p, ok := sub.Body.(*Prose)
	if !ok {
		t.Fatalf("subsection %q body is %T, want *Prose", sub.ID, sub.Body)
	}
	if len(p.Text) == 0 {
		return ""
	}
	return p.Text[0]
}

func find(t *testing.T, sec ResolvedSection, id string) ResolvedSubsection {
	t.Helper()
	for _, sub := range sec.Subsections {
		if sub.ID == id {
			return sub
		}
	}
	t.Fatalf("section %q has no subsection %q", sec.ID, id)
	return ResolvedSubsection{}
}

// No selection at all: every subsection falls back to its default variant.
func TestResolveDefaults(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if sec.ID != "first_aid" || sec.Number != 4 {
		t.Errorf("got ID=%q Number=%d", sec.ID, sec.Number)
	}
	if len(sec.Subsections) != len(def.Subsections) {
		t.Fatalf("resolved %d subsections, want %d", len(sec.Subsections), len(def.Subsections))
	}
	for _, sub := range sec.Subsections {
		if sub.Variant != DefaultVariant {
			t.Errorf("subsection %q used variant %q, want %q", sub.ID, sub.Variant, DefaultVariant)
		}
	}
	// Manifest order is render order.
	if sec.Subsections[0].ID != "general" {
		t.Errorf("first subsection = %q, want %q", sec.Subsections[0].ID, "general")
	}
}

// A preset switches every subsection it names, and nothing else.
func TestResolvePreset(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{Variant: "corrosive"}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	ingestion := find(t, sec, "ingestion")
	if ingestion.Variant != "corrosive" {
		t.Errorf("ingestion variant = %q, want %q", ingestion.Variant, "corrosive")
	}
	if !strings.Contains(firstParagraph(t, ingestion), "Do NOT induce vomiting") {
		t.Errorf("ingestion did not pick up corrosive wording: %q", firstParagraph(t, ingestion))
	}
}

// A per-subsection variant beats the preset's pick for that subsection only.
func TestResolveOverrideBeatsPreset(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{
		Variant: "corrosive",
		Subsections: map[string]SubsectionOverride{
			"inhalation": {Variant: "acute_toxicity"},
		},
	}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if got := find(t, sec, "inhalation").Variant; got != "acute_toxicity" {
		t.Errorf("inhalation variant = %q, want %q (override must beat preset)", got, "acute_toxicity")
	}
	// The preset still governs everything it was not overridden on. This is
	// the whole design: two hazard profiles combine without a hand-written
	// "corrosive plus acute inhalation" file existing anywhere.
	if got := find(t, sec, "skin").Variant; got != "corrosive" {
		t.Errorf("skin variant = %q, want %q (preset still applies)", got, "corrosive")
	}
}

// Append, including the bare-list shorthand.
func TestResolveAppend(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	var shorthand Block
	if err := yamlInto(`["Call the site medical officer."]`, &shorthand); err != nil {
		t.Fatalf("decoding shorthand: %v", err)
	}

	base, err := Resolve(lib, def, SectionSelection{}, ResolveContext{})
	if err != nil {
		t.Fatalf("baseline Resolve() error = %v", err)
	}
	baseLen := len(base.Subsections[0].Body.(*Prose).Text)

	sec, err := Resolve(lib, def, SectionSelection{
		Subsections: map[string]SubsectionOverride{
			"general": {Append: &shorthand},
		},
	}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	general := find(t, sec, "general").Body.(*Prose)
	if len(general.Text) != baseLen+1 {
		t.Fatalf("len(Text) = %d, want %d", len(general.Text), baseLen+1)
	}
	if general.Text[len(general.Text)-1] != "Call the site medical officer." {
		t.Errorf("appended paragraph = %q", general.Text[len(general.Text)-1])
	}
}

// Replace discards the variant content entirely.
func TestResolveReplace(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	var replacement Block
	if err := yamlInto("\"Consult the on-site physician immediately.\"", &replacement); err != nil {
		t.Fatalf("decoding replacement: %v", err)
	}

	sec, err := Resolve(lib, def, SectionSelection{
		Variant: "corrosive",
		Subsections: map[string]SubsectionOverride{
			"general": {Replace: &replacement},
		},
	}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	general := find(t, sec, "general").Body.(*Prose)
	if len(general.Text) != 1 || general.Text[0] != "Consult the on-site physician immediately." {
		t.Errorf("Text = %q, want the replacement only", general.Text)
	}
}

// An empty body must surface EmptyText rather than rendering blank.
func TestResolveEmptyBody(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	var blank Block
	if err := yamlInto(`[]`, &blank); err != nil {
		t.Fatalf("decoding blank: %v", err)
	}

	sec, err := Resolve(lib, def, SectionSelection{
		Subsections: map[string]SubsectionOverride{
			"symptoms": {Replace: &blank},
		},
	}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	symptoms := find(t, sec, "symptoms")
	if !symptoms.Empty {
		t.Error("Empty = false, want true for a blanked subsection")
	}
	if symptoms.EmptyText != "No data available." {
		t.Errorf("EmptyText = %q", symptoms.EmptyText)
	}
}

// Typos in a document must fail loudly, not vanish.
func TestResolveRejectsUnknownNames(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	tests := []struct {
		name string
		sel  SectionSelection
		want string
	}{
		{
			"unknown preset",
			SectionSelection{Variant: "definitely_not_a_preset"},
			"definitely_not_a_preset",
		},
		{
			"unknown variant",
			SectionSelection{Subsections: map[string]SubsectionOverride{
				"inhalation": {Variant: "nope"},
			}},
			"nope",
		},
		{
			"unknown subsection",
			SectionSelection{Subsections: map[string]SubsectionOverride{
				"inhalatoin": {Variant: "corrosive"}, // transposed letters
			}},
			"inhalatoin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(lib, def, tt.sel, ResolveContext{})
			if err == nil {
				t.Fatal("Resolve() = nil error, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should name %q", err, tt.want)
			}
			t.Logf("message:\n%v", err)
		})
	}
}

// A kind mismatch between an override and the manifest must be caught.
func TestResolveRejectsKindMismatch(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "08_exposure_controls")

	var proseBlock Block
	if err := yamlInto(`["this is prose, not a table"]`, &proseBlock); err != nil {
		t.Fatalf("decoding: %v", err)
	}

	_, err := Resolve(lib, def, SectionSelection{
		Subsections: map[string]SubsectionOverride{
			"exposure_limits": {Append: &proseBlock}, // declared kind is table
		},
	}, ResolveContext{})
	if err == nil {
		t.Fatal("Resolve() = nil error, want a kind mismatch")
	}
	if !strings.Contains(err.Error(), "table") || !strings.Contains(err.Error(), "prose") {
		t.Errorf("error should name both kinds: %v", err)
	}
}

// The full 16-section run over the real library.
func TestResolveAllSections(t *testing.T) {
	lib := realLibrary(t)

	sections, err := ResolveAll(lib, map[string]SectionSelection{
		"hazards":           {Variant: "corrosive"},
		"first_aid":         {Variant: "corrosive"},
		"exposure_controls": {Variant: "acute_inhalation"},
	}, ResolveContext{})
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}

	if len(sections) != 16 {
		t.Fatalf("resolved %d sections, want 16", len(sections))
	}
	for i, sec := range sections {
		if sec.Number != i+1 {
			t.Errorf("section %d has Number %d; layout order must match numbering", i, sec.Number)
		}
		if len(sec.Subsections) == 0 {
			t.Errorf("section %q resolved with no subsections", sec.ID)
		}
	}
}

func TestResolveAllRejectsUnknownSection(t *testing.T) {
	lib := realLibrary(t)
	_, err := ResolveAll(lib, map[string]SectionSelection{"frist_aid": {}}, ResolveContext{})
	if err == nil || !strings.Contains(err.Error(), "frist_aid") {
		t.Fatalf("error = %v, want it to name the unknown section", err)
	}
}

// resolveEcological is the shared setup for the multi-kind tests below:
// Section 12's subsections accept prose OR a table, so each of these drives one
// override through the real library.
func resolveEcological(t *testing.T, subID string, override SubsectionOverride) (ResolvedSection, error) {
	t.Helper()
	lib := realLibrary(t)
	def := loadDef(t, lib, "12_ecological")
	return Resolve(lib, def, SectionSelection{
		Subsections: map[string]SubsectionOverride{subID: override},
	}, ResolveContext{})
}

func tableReplace(t *testing.T, yamlSrc string) SubsectionOverride {
	t.Helper()
	var block Block
	if err := yamlInto(yamlSrc, &block); err != nil {
		t.Fatalf("decoding block: %v", err)
	}
	return SubsectionOverride{Replace: &block}
}

// The point of the whole change: a prose-defaulted ecological subsection
// accepts a table from the document.
func TestResolveMultiKindAcceptsTable(t *testing.T) {
	sec, err := resolveEcological(t, "ecotoxicity", tableReplace(t, `
kind: table
headers: ["Species", "Endpoint", "Value"]
rows:
  - ["Fish (O. mykiss)", "96h LC50", "5.5 mg/L"]
`))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "ecotoxicity")
	tbl, ok := sub.Body.(*Table)
	if !ok {
		t.Fatalf("body is %T, want *Table", sub.Body)
	}
	if len(tbl.Rows) != 1 || tbl.Rows[0][2] != "5.5 mg/L" {
		t.Errorf("Rows = %v", tbl.Rows)
	}
	// Kind must follow the BODY, or block.html.tmpl renders the table through
	// the prose partial and it silently vanishes from the sheet.
	if sub.Kind != "table" {
		t.Errorf("Kind = %q, want %q", sub.Kind, "table")
	}
	if sub.Empty {
		t.Error("Empty = true for a table with a row")
	}
}

// Headers are optional: the renderer omits <thead> and the width check falls
// back to the first row.
func TestResolveMultiKindTableWithoutHeaders(t *testing.T) {
	sec, err := resolveEcological(t, "bioaccumulation", tableReplace(t, `
kind: table
rows:
  - ["log Kow", "2.73"]
  - ["BCF", "90"]
`))
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	tbl, ok := find(t, sec, "bioaccumulation").Body.(*Table)
	if !ok {
		t.Fatalf("body is %T, want *Table", find(t, sec, "bioaccumulation").Body)
	}
	if len(tbl.Headers) != 0 {
		t.Errorf("Headers = %v, want none", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Errorf("Rows = %v, want 2", tbl.Rows)
	}
}

// The prose side must keep working, shorthand included.
func TestResolveMultiKindStillAcceptsProse(t *testing.T) {
	var block Block
	if err := yamlInto(`"Readily biodegradable."`, &block); err != nil {
		t.Fatalf("decoding shorthand: %v", err)
	}

	sec, err := resolveEcological(t, "persistence", SubsectionOverride{Replace: &block})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "persistence")
	if got := firstParagraph(t, sub); got != "Readily biodegradable." {
		t.Errorf("first paragraph = %q", got)
	}
	if sub.Kind != "prose" {
		t.Errorf("Kind = %q, want %q", sub.Kind, "prose")
	}
}

// Widening to prose-or-table must not widen to everything.
func TestResolveMultiKindRejectsUnlistedKind(t *testing.T) {
	_, err := resolveEcological(t, "mobility", tableReplace(t, `
kind: images
images:
  - { src: "x.png", alt: "x" }
`))
	if err == nil {
		t.Fatal("Resolve() = nil error, want a rejection")
	}
	for _, frag := range []string{"replace block is images content", "accepts prose or table"} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("error missing %q\nfull message:\n%v", frag, err)
		}
	}
}

// Appending across kinds cannot work, so the error has to point at replace
// rather than leave the author staring at a bare kind mismatch.
func TestResolveMultiKindAppendAcrossKinds(t *testing.T) {
	var block Block
	if err := yamlInto("kind: table\nrows:\n  - [\"a\", \"b\"]\n", &block); err != nil {
		t.Fatalf("decoding block: %v", err)
	}

	_, err := resolveEcological(t, "ecotoxicity", SubsectionOverride{Append: &block})
	if err == nil {
		t.Fatal("Resolve() = nil error, want a rejection")
	}
	for _, frag := range []string{
		"cannot append table content to the prose content that resolved here",
		"use `replace` to supply table instead",
	} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("error missing %q\nfull message:\n%v", frag, err)
		}
	}
}
