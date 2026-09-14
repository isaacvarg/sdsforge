package document

import (
	"testing"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

func TestTscaInventoryBlockOmittedWhenEmpty(t *testing.T) {
	if block := tscaInventoryBlock(nil); block != nil {
		t.Errorf("tscaInventoryBlock(nil) = %v, want nil", block)
	}
}

func TestTscaInventoryBlockRows(t *testing.T) {
	block := tscaInventoryBlock([]TSCAInventoryEntry{
		{Material: "Toluene", CASNumber: "108-88-3", Basis: "Active"},
		{Material: "Water", CASNumber: "7732-18-5", Basis: ""},
	})
	tbl, ok := block.(*sections.Table)
	if !ok {
		t.Fatalf("tscaInventoryBlock() = %T, want *sections.Table", block)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(tbl.Rows))
	}
	if got := tbl.Rows[0]; got[0] != "Toluene" || got[1] != "108-88-3" || got[2] != "Active" {
		t.Errorf("Rows[0] = %v", got)
	}
	if got := tbl.Rows[1]; got[0] != "Water" || got[1] != "7732-18-5" || got[2] != "" {
		t.Errorf("Rows[1] = %v, want a blank Basis cell", got)
	}
}
