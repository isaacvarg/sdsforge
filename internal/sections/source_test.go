package sections

import (
	"slices"
	"strings"
	"testing"
)

func TestSourceNames(t *testing.T) {
	want := []string{
		"classification", "emergency_phone", "identification", "materials",
		"pictograms", "precautionary", "recommended_use", "revisions",
		"signal_word", "supplier",
	}
	if got := SourceNames(); !slices.Equal(got, want) {
		t.Errorf("SourceNames() = %v, want %v", got, want)
	}
}

// A source with no usable content must be invisible to the resolver, so the
// library's authored placeholder survives.
func TestSourceDataBlock(t *testing.T) {
	tests := []struct {
		name   string
		data   SourceData
		source string
		want   bool
	}{
		{"populated", SourceData{"materials": &Prose{Text: []string{"x"}}}, "materials", true},
		{"empty block", SourceData{"materials": &Prose{}}, "materials", false},
		{"blank cells only", SourceData{"materials": &Table{Rows: [][]string{{" "}}}}, "materials", false},
		{"nil block", SourceData{"materials": nil}, "materials", false},
		{"absent", SourceData{}, "materials", false},
		{"nil map", nil, "materials", false},
		{"no source declared", SourceData{"materials": &Prose{Text: []string{"x"}}}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.data.Block(tt.source)
			if ok != tt.want {
				t.Errorf("Block(%q) ok = %v, want %v", tt.source, ok, tt.want)
			}
		})
	}
}

// A source populates a subsection that declares one.
func TestResolveSourcePopulates(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "03_composition")

	data := SourceData{SourceMaterials: &Table{
		Headers: []string{"Chemical name", "CAS No.", "Concentration (% w/w)"},
		Rows:    [][]string{{"Sodium hydroxide", "1310-73-2", "50"}},
	}}

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{Sources: data})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "ingredients")
	if sub.Source != SourceMaterials {
		t.Errorf("Source = %q, want %q", sub.Source, SourceMaterials)
	}
	tbl := sub.Body.(*Table)
	if len(tbl.Rows) != 1 || tbl.Rows[0][0] != "Sodium hydroxide" {
		t.Errorf("Rows = %q, want the document's materials", tbl.Rows)
	}
}

// No data means the authored placeholder stays, rather than a blank table.
func TestResolveSourceAbsentKeepsDefault(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "03_composition")

	sec, err := Resolve(lib, def, SectionSelection{}, ResolveContext{})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	sub := find(t, sec, "ingredients")
	if sub.Source != "" {
		t.Errorf("Source = %q, want empty when no data was supplied", sub.Source)
	}
	if sub.Empty {
		t.Error("subsection resolved empty; the library placeholder should have survived")
	}
	if got := sub.Body.(*Table).Rows[0][0]; !strings.Contains(got, "No hazardous ingredients") {
		t.Errorf("Rows[0][0] = %q, want the placeholder", got)
	}
}

// An explicit override in the document still beats the source.
func TestResolveOverrideBeatsSource(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "03_composition")

	data := SourceData{SourceMaterials: &Table{
		Headers: []string{"Chemical name", "CAS No.", "Concentration (% w/w)"},
		Rows:    [][]string{{"From materials", "-", "-"}},
	}}

	sel := SectionSelection{Subsections: map[string]SubsectionOverride{
		"ingredients": {Replace: &Block{Body: &Table{
			Headers: []string{"Chemical name", "CAS No.", "Concentration (% w/w)"},
			Rows:    [][]string{{"Hand written", "-", "-"}},
		}}},
	}}

	sec, err := Resolve(lib, def, sel, ResolveContext{Sources: data})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got := find(t, sec, "ingredients").Body.(*Table).Rows[0][0]; got != "Hand written" {
		t.Errorf("Rows[0][0] = %q, want the explicit override to win over the source", got)
	}
}

// Append composes on top of the source rather than discarding it.
func TestResolveAppendOnTopOfSource(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "03_composition")

	data := SourceData{SourceMaterials: &Table{
		Headers: []string{"Chemical name", "CAS No.", "Concentration (% w/w)"},
		Rows:    [][]string{{"Sodium hydroxide", "1310-73-2", "50"}},
	}}
	sel := SectionSelection{Subsections: map[string]SubsectionOverride{
		"ingredients": {Append: &Block{Body: &Table{
			Rows: [][]string{{"Water", "7732-18-5", "50"}},
		}}},
	}}

	sec, err := Resolve(lib, def, sel, ResolveContext{Sources: data})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	rows := find(t, sec, "ingredients").Body.(*Table).Rows
	if len(rows) != 2 || rows[0][0] != "Sodium hydroxide" || rows[1][0] != "Water" {
		t.Errorf("Rows = %q, want the source row then the appended row", rows)
	}
}

// A source supplying the wrong shape must be rejected, not rendered.
func TestResolveSourceKindMismatch(t *testing.T) {
	lib := realLibrary(t)
	def := loadDef(t, lib, "03_composition")

	data := SourceData{SourceMaterials: &Prose{Text: []string{"prose, but the subsection is a table"}}}

	_, err := Resolve(lib, def, SectionSelection{}, ResolveContext{Sources: data})
	if err == nil {
		t.Fatal("Resolve() = nil error, want a kind mismatch")
	}
	if !strings.Contains(err.Error(), "materials") || !strings.Contains(err.Error(), "table") {
		t.Errorf("error should name the source and the expected kind: %v", err)
	}
}

// A misspelled source in a manifest must fail at load, listing the valid names.
func TestManifestRejectsUnknownSource(t *testing.T) {
	_, err := LoadSection(testFS(t), "bad_source")
	if err == nil {
		t.Fatal("LoadSection(bad_source) = nil error, want an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `unknown source "materialz"`) {
		t.Errorf("error should name the bad source: %s", msg)
	}
	if !strings.Contains(msg, strings.Join(SourceNames(), ", ")) {
		t.Errorf("error should list the valid sources: %s", msg)
	}
}
