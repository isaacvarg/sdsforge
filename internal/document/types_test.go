package document

import (
	"testing"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

// An empty source must be omitted entirely, not supplied as an empty block,
// so the library's authored placeholder survives.
func TestSourceDataOmitsEmpty(t *testing.T) {
	var empty Data
	if got := empty.SourceData(nil, config.Config{}); len(got) != 0 {
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
		Materials: []Material{
			{Name: "Sodium hydroxide", CASNumber: "1310-73-2", Percentage: "50"},
			{Name: "Water", CASNumber: "7732-18-5", Percentage: "50"},
		},
		Revisions: []Revision{
			{Version: "1.0", RevisionDate: "2026-01-01", Description: "Initial issue"},
		},
	}

	// Supplier and emergency details come from configuration, not from the
	// document.
	cfg := config.Config{
		Company: config.Company{Name: "Acme Chemical Co.", Phone: "555-0100"},
		Emergency: config.Emergency{Contacts: []config.Contact{
			{Name: "CHEMTREC (24 hr)", Phone: "1-800-424-9300", Note: "USA"},
		}},
	}
	got := d.SourceData(nil, cfg)

	for _, name := range []string{
		sections.SourceIdentification,
		sections.SourceSupplier,
		sections.SourceEmergencyPhone,
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
	var d Data
	cfg := config.Config{Company: config.Company{Name: "Acme Chemical Co."}}

	got := d.SourceData(nil, cfg)
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

// A document written before company details moved into configuration still
// loads, but nothing on the sheet comes from its supplier block.
func TestLegacySupplierIsIgnored(t *testing.T) {
	d := Data{Supplier: Supplier{Name: "Old Co.", EmergencyPhone: "555-0199"}}

	if !d.HasLegacySupplier() {
		t.Error("HasLegacySupplier() = false, want true so generate can warn")
	}

	got := d.SourceData(nil, config.Config{})
	if _, ok := got.Block(sections.SourceSupplier); ok {
		t.Error("supplier block built from the document's own supplier: details")
	}
	if _, ok := got.Block(sections.SourceEmergencyPhone); ok {
		t.Error("emergency block built from the document's own supplier: details")
	}

	if (Data{}).HasLegacySupplier() {
		t.Error("HasLegacySupplier() = true on a document with no supplier block")
	}
}
