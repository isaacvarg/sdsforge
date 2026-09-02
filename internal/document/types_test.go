package document

import (
	"reflect"
	"testing"
	"time"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

// An empty source must be omitted entirely, not supplied as an empty block,
// so the library's authored placeholder survives.
func TestSourceDataOmitsEmpty(t *testing.T) {
	var empty Data
	if got := empty.SourceData(nil, config.Config{}, VersionIndex{}); len(got) != 0 {
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
	}

	// Supplier and emergency details come from configuration, not from the
	// document.
	cfg := config.Config{
		Company: config.Company{Name: "Acme Chemical Co.", Phone: "555-0100"},
		Emergency: config.Emergency{Contacts: []config.Contact{
			{Name: "CHEMTREC (24 hr)", Phone: "1-800-424-9300", Note: "USA"},
		}},
	}
	got := d.SourceData(nil, cfg, testVersions())

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

	// Section 16's revision history is the version history, not anything the
	// document carries of its own.
	revisions := got[sections.SourceRevisions].(*sections.Table)
	want := [][]string{
		{"1.0.0", "2026-01-05", "Authored document"},
		{"1.1.0", "2026-03-02", "Added H314"},
	}
	if !reflect.DeepEqual(revisions.Rows, want) {
		t.Errorf("revision rows = %q, want %q", revisions.Rows, want)
	}
}

// With no versions recorded there is no revisions block at all, so the
// library's "No revision history recorded" default survives.
func TestSourceDataOmitsRevisionsWithoutVersions(t *testing.T) {
	d := Data{ProductName: "Acetone"}

	got := d.SourceData(nil, config.Config{}, VersionIndex{NextID: 1})
	if _, ok := got.Block(sections.SourceRevisions); ok {
		t.Error("revisions block present despite no versions recorded")
	}
}

// testVersions is a two-entry history, the shape section 16 renders from.
func testVersions() VersionIndex {
	return VersionIndex{
		NextID: 3,
		Versions: []Version{
			{
				ID:        1,
				Label:     "1.0.0",
				Timestamp: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC),
				Memo:      "Authored document",
			},
			{
				ID:        2,
				Label:     "1.1.0",
				Timestamp: time.Date(2026, 3, 2, 16, 45, 0, 0, time.UTC),
				Memo:      "Added H314",
			},
		},
	}
}

// Partial data yields partial content, not blanks.
func TestSourceDataPartial(t *testing.T) {
	var d Data
	cfg := config.Config{Company: config.Company{Name: "Acme Chemical Co."}}

	got := d.SourceData(nil, cfg, VersionIndex{})
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

	got := d.SourceData(nil, config.Config{}, VersionIndex{})
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
