package document

import (
	"testing"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

func TestSaraHazardsBlockOmittedWhenEmpty(t *testing.T) {
	if block := saraHazardsBlock(nil); block != nil {
		t.Errorf("saraHazardsBlock(nil) = %v, want nil", block)
	}
}

func TestSaraHazardsBlockSingleHazard(t *testing.T) {
	block := saraHazardsBlock([]SARAHazard{
		{Chemical: "Toluene", CASNumber: "108-88-3", Hazard: "Fire hazard"},
	})
	tbl, ok := block.(*sections.Table)
	if !ok {
		t.Fatalf("saraHazardsBlock() = %T, want *sections.Table", block)
	}
	if len(tbl.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(tbl.Rows))
	}
	if got := tbl.Rows[0]; got[0] != "Toluene" || got[1] != "108-88-3" || got[2] != "Fire hazard" {
		t.Errorf("Rows[0] = %v", got)
	}
}

func TestSaraHazardsBlockMultipleHazardsPerChemical(t *testing.T) {
	block := saraHazardsBlock([]SARAHazard{
		{Chemical: "Toluene", CASNumber: "108-88-3", Hazard: "Fire hazard"},
		{Chemical: "Toluene", CASNumber: "108-88-3", Hazard: "Acute health hazard"},
	})
	tbl := block.(*sections.Table)
	if len(tbl.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(tbl.Rows))
	}
	if tbl.Rows[0][2] != "Fire hazard" || tbl.Rows[1][2] != "Acute health hazard" {
		t.Errorf("Rows = %v, want both hazard rows in document order", tbl.Rows)
	}
}
