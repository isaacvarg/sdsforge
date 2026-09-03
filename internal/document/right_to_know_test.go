package document

import (
	"testing"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

func TestRightToKnowBlockOmittedWhenEmpty(t *testing.T) {
	for _, entries := range [][]RightToKnowEntry{
		nil,
		{{Chemical: "Toluene", States: map[string]bool{"nj": false}}},
		{{Chemical: "Toluene", States: map[string]bool{"zz": true}}}, // unrecognized code
		{{Chemical: "", States: map[string]bool{"nj": true}}},
	} {
		if block := rightToKnowBlock(entries); block != nil {
			t.Errorf("rightToKnowBlock(%v) = %v, want nil", entries, block)
		}
	}
}

func TestRightToKnowBlockSingleState(t *testing.T) {
	block := rightToKnowBlock([]RightToKnowEntry{
		{Chemical: "Toluene", CASNumber: "108-88-3", States: map[string]bool{"nj": true, "ca": false}},
	})
	tables, ok := block.(*sections.Tables)
	if !ok {
		t.Fatalf("rightToKnowBlock() = %T, want *sections.Tables", block)
	}
	if len(tables.Tables) != 1 {
		t.Fatalf("len(Tables) = %d, want 1", len(tables.Tables))
	}

	nt := tables.Tables[0]
	if nt.Title != "New Jersey Right to Know" {
		t.Errorf("Title = %q", nt.Title)
	}
	if len(nt.Rows) != 1 || nt.Rows[0][0] != "Toluene" || nt.Rows[0][1] != "108-88-3" {
		t.Errorf("Rows = %v", nt.Rows)
	}
}

func TestRightToKnowBlockChemicalInMultipleStates(t *testing.T) {
	block := rightToKnowBlock([]RightToKnowEntry{
		{Chemical: "Toluene", CASNumber: "108-88-3", States: map[string]bool{"nj": true, "pa": true}},
	})
	tables := block.(*sections.Tables)
	if len(tables.Tables) != 2 {
		t.Fatalf("len(Tables) = %d, want 2", len(tables.Tables))
	}
	// Sorted by full state name: New Jersey before Pennsylvania.
	if tables.Tables[0].Title != "New Jersey Right to Know" {
		t.Errorf("Tables[0].Title = %q", tables.Tables[0].Title)
	}
	if tables.Tables[1].Title != "Pennsylvania Right to Know" {
		t.Errorf("Tables[1].Title = %q", tables.Tables[1].Title)
	}
}

func TestRightToKnowBlockMultipleChemicalsShareState(t *testing.T) {
	block := rightToKnowBlock([]RightToKnowEntry{
		{Chemical: "Toluene", CASNumber: "108-88-3", States: map[string]bool{"nj": true}},
		{Chemical: "Dichloromethane", CASNumber: "75-09-2", States: map[string]bool{"nj": true}},
	})
	tables := block.(*sections.Tables)
	if len(tables.Tables) != 1 {
		t.Fatalf("len(Tables) = %d, want 1", len(tables.Tables))
	}
	rows := tables.Tables[0].Rows
	if len(rows) != 2 || rows[0][0] != "Toluene" || rows[1][0] != "Dichloromethane" {
		t.Errorf("Rows = %v, want both chemicals in document order", rows)
	}
}
