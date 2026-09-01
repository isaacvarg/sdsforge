package sections

import (
	"strings"
	"testing"
)

func codes(list ...string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, c := range list {
		set[c] = true
	}
	return set
}

// The whole point of the feature: hazard codes alone select the right wording,
// with no preset named anywhere in the document.
func TestDeriveSelectsVariant(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{
		HazardCodes: codes("H314"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "ingestion")
	if sub.Variant != "corrosive" {
		t.Errorf("ingestion variant = %q, want %q derived from H314", sub.Variant, "corrosive")
	}
	if sub.DerivedFrom != "H314" {
		t.Errorf("DerivedFrom = %q, want %q", sub.DerivedFrom, "H314")
	}
	if !strings.Contains(firstParagraph(t, sub), "Do NOT induce vomiting") {
		t.Errorf("wording was not the corrosive one: %q", firstParagraph(t, sub))
	}
}

// No codes must reproduce the old behaviour exactly.
func TestDeriveNoCodesKeepsDefaults(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	for _, sub := range sec.Subsections {
		if sub.Variant != DefaultVariant {
			t.Errorf("subsection %q derived %q with no hazard codes", sub.ID, sub.Variant)
		}
		if sub.DerivedFrom != "" {
			t.Errorf("subsection %q reports DerivedFrom %q", sub.ID, sub.DerivedFrom)
		}
	}
}

// A code matching nothing leaves the default in place rather than erroring.
func TestDeriveUnmatchedCodeKeepsDefault(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{
		HazardCodes: codes("H412"), // no first-aid variant is predicated on it
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := find(t, sec, "ingestion").Variant; got != DefaultVariant {
		t.Errorf("variant = %q, want the default when nothing matches", got)
	}
}

// Manual selection outranks anything computed -- at both the preset and the
// per-subsection level.
func TestDeriveYieldsToManualSelection(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	t.Run("preset beats derivation", func(t *testing.T) {
		sec, err := Resolve(lib, def,
			SectionSelection{Variant: "acute_inhalation"},
			ResolveContext{HazardCodes: codes("H314")})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		sub := find(t, sec, "inhalation")
		if sub.Variant != "acute_toxicity" {
			t.Errorf("variant = %q, want the preset's pick over the derived one", sub.Variant)
		}
		if sub.DerivedFrom != "" {
			t.Errorf("DerivedFrom = %q, want empty for a manual pick", sub.DerivedFrom)
		}
	})

	t.Run("explicit variant beats derivation", func(t *testing.T) {
		sec, err := Resolve(lib, def,
			SectionSelection{Subsections: map[string]SubsectionOverride{
				"ingestion": {Variant: DefaultVariant},
			}},
			ResolveContext{HazardCodes: codes("H314")})
		if err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
		if got := find(t, sec, "ingestion").Variant; got != DefaultVariant {
			t.Errorf("variant = %q, want the explicit choice to win", got)
		}
	})
}

// Highest priority wins when several predicates match.
func TestDerivePriorityWins(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	// H314 -> corrosive (priority 30); H331 -> acute_toxicity (priority 20).
	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{
		HazardCodes: codes("H314", "H331"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := find(t, sec, "inhalation").Variant; got != "corrosive" {
		t.Errorf("variant = %q, want corrosive (priority 30 over 20)", got)
	}
}

// Derivation applies across the whole document, replacing what used to be five
// repeated preset selections.
func TestDeriveAcrossAllSections(t *testing.T) {
	lib := realLibrary(t)

	resolved, err := ResolveAll(lib, nil, ResolveContext{HazardCodes: codes("H314")})
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}

	want := map[string]map[string]string{
		"first_aid":          {"ingestion": "corrosive", "eye": "corrosive"},
		"accidental_release": {"personal_precautions": "corrosive"},
		"exposure_controls":  {"eye_protection": "corrosive", "skin_protection": "corrosive"},
		"toxicological":      {"effects": "corrosive"},
	}
	for _, sec := range resolved {
		expect, ok := want[sec.ID]
		if !ok {
			continue
		}
		for _, sub := range sec.Subsections {
			if v, ok := expect[sub.ID]; ok && sub.Variant != v {
				t.Errorf("%s/%s derived %q, want %q", sec.ID, sub.ID, sub.Variant, v)
			}
		}
	}
}

// A priority tie is an error, not an arbitrary pick between hazard profiles.
func TestDeriveRejectsPriorityTie(t *testing.T) {
	// A custom layer can introduce a tie that ValidateLibrary would reject.
	dir := t.TempDir()
	writeVariant(t, dir, "04_first_aid/skin/tie_a.yaml", `
variant: tie_a
applies_when: { any_of: [H314] }
priority: 30
content: { kind: prose, text: ["A"] }
`)
	writeVariant(t, dir, "04_first_aid/skin/tie_b.yaml", `
variant: tie_b
applies_when: { any_of: [H314] }
priority: 30
content: { kind: prose, text: ["B"] }
`)

	lib, err := NewLibrary(LibraryOptions{CustomVariants: true, CustomDir: dir})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}
	def, err := LoadSection(lib, "04_first_aid")
	if err != nil {
		t.Fatal(err)
	}

	_, err = Resolve(lib, def, SectionSelection{}, ResolveContext{HazardCodes: codes("H314")})
	if err == nil {
		t.Fatal("Resolve() = nil error, want a priority tie error")
	}
	for _, want := range []string{"tie_a", "tie_b", "same priority"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q: %v", want, err)
		}
	}
}

// A manual pick that displaces a derived one must be recorded, so a reviewer
// can see where a human overruled the automatic classification.
func TestDeriveRecordsManualOverride(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "04_first_aid")

	sec, err := Resolve(lib, def,
		SectionSelection{Subsections: map[string]SubsectionOverride{
			"ingestion": {Variant: DefaultVariant},
		}},
		ResolveContext{HazardCodes: codes("H314")})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "ingestion")
	if sub.Variant != DefaultVariant {
		t.Errorf("Variant = %q, want the manual choice", sub.Variant)
	}
	if sub.SupersededDerived != "corrosive" {
		t.Errorf("SupersededDerived = %q, want %q", sub.SupersededDerived, "corrosive")
	}

	// A manual pick landing on the same variant derivation would have chosen
	// is not a disagreement and must not be flagged as one.
	sec, err = Resolve(lib, def,
		SectionSelection{Subsections: map[string]SubsectionOverride{
			"ingestion": {Variant: "corrosive"},
		}},
		ResolveContext{HazardCodes: codes("H314")})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := find(t, sec, "ingestion").SupersededDerived; got != "" {
		t.Errorf("SupersededDerived = %q, want empty when the pick agrees", got)
	}
}

// Section 2 is computed, so a derived variant there is superseded by the source.
func TestDeriveSupersededBySource(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "02_hazards")

	table := &Table{
		Headers: []string{"Hazard class", "Category", "Hazard statement"},
		Rows:    [][]string{{"Skin corrosion/irritation", "1", "H314: Causes severe skin burns."}},
	}
	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{
		Sources:     SourceData{SourceClassification: table},
		HazardCodes: codes("H314"),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "classification")
	if sub.Source != SourceClassification {
		t.Errorf("Source = %q, want the computed block to win", sub.Source)
	}
	if got := sub.Body.(*Table).Rows[0][2]; !strings.Contains(got, "H314") {
		t.Errorf("body = %q, want the computed classification", got)
	}
}
