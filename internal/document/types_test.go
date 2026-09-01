package document

import (
	"testing"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

// An empty source must be omitted entirely, not supplied as an empty block,
// so the library's authored placeholder survives.
func TestSourceDataOmitsEmpty(t *testing.T) {
	var empty Data
	if got := empty.SourceData(nil); len(got) != 0 {
		t.Errorf("SourceData() on a zero Data = %v, want empty", got)
	}
}

func TestSourceDataPopulates(t *testing.T) {
	d := Data{
		ProductName: "Caustic Soda 50%",
		Identification: Identification{
			ProductCodes: []string{"CS-50", "CS-50D"},
			CASNumber:    "1310-73-2",
		},
		Supplier: Supplier{
			Name:           "Acme Chemical Co.",
			Phone:          "555-0100",
			EmergencyPhone: "1-800-424-9300",
		},
		Materials: []Material{
			{Name: "Sodium hydroxide", CASNumber: "1310-73-2", Percentage: "50"},
			{Name: "Water", CASNumber: "7732-18-5", Percentage: "50"},
		},
		Revisions: []Revision{
			{Version: "1.0", RevisionDate: "2026-01-01", Description: "Initial issue"},
		},
	}

	got := d.SourceData(nil)

	for _, name := range []string{
		sections.SourceIdentification,
		sections.SourceSupplier,
		sections.SourceMaterials,
		sections.SourceRevisions,
	} {
		if _, ok := got.Block(name); !ok {
			t.Errorf("SourceData() has no usable block for %q", name)
		}
	}

	materials := got[sections.SourceMaterials].(*sections.Table)
	if len(materials.Rows) != 2 || materials.Rows[1][0] != "Water" {
		t.Errorf("materials rows = %q", materials.Rows)
	}

	ident := got[sections.SourceIdentification].(*sections.Prose)
	if len(ident.Text) != 3 { // product name, codes, CAS -- no synonyms given
		t.Errorf("identification lines = %q, want 3", ident.Text)
	}
}

// Partial data yields partial content, not blanks.
func TestSourceDataPartial(t *testing.T) {
	d := Data{Supplier: Supplier{Name: "Acme Chemical Co."}}

	got := d.SourceData(nil)
	if _, ok := got.Block(sections.SourceSupplier); !ok {
		t.Fatal("supplier block missing")
	}
	if _, ok := got.Block(sections.SourceMaterials); ok {
		t.Error("materials block present despite no materials")
	}
	if lines := got[sections.SourceSupplier].(*sections.Prose).Text; len(lines) != 1 {
		t.Errorf("supplier lines = %q, want just the name", lines)
	}
	if _, ok := got.Block(sections.SourceEmergencyPhone); ok {
		t.Error("emergency phone block present despite none given")
	}
}
