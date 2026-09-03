package sections

import (
	"fmt"
	"strings"
)

// Tables is a named sequence of independent tables -- Section 15's state
// Right-to-Know disclosures, one table per state a document triggers:
//
//	kind: tables
//	tables:
//	  - title: "New Jersey Right to Know"
//	    headers: ["Chemical name", "CAS No."]
//	    rows:
//	      - ["Toluene", "108-88-3"]
//
// Table.Append merges rows into a single header set, which is the wrong
// operation here: each named table keeps its own headers, so this is its
// own content kind rather than a variant of Table.
type Tables struct {
	Tables []NamedTable `yaml:"tables"`
}

// NamedTable is one table with a heading, printed above it.
type NamedTable struct {
	Title   string     `yaml:"title"`
	Headers []string   `yaml:"headers"`
	Rows    [][]string `yaml:"rows"`
}

var _ Content = (*Tables)(nil)

func (t *Tables) Kind() string {
	return "tables"
}

// IsEmpty reports whether any table in the sequence has actual data, using
// the same rule Table.IsEmpty applies to a single table: headers with no
// non-blank rows count as empty.
func (t *Tables) IsEmpty() bool {
	for _, nt := range t.Tables {
		for _, row := range nt.Rows {
			for _, cell := range row {
				if strings.TrimSpace(cell) != "" {
					return false
				}
			}
		}
	}
	return true
}

// Append adds another sequence's tables to this one. Unlike Table.Append,
// there is no header reconciliation: each named table is independent, so
// appending just concatenates the two lists.
func (t *Tables) Append(other Content) (Content, error) {
	ot, isTables := other.(*Tables)
	if !isTables {
		return nil, fmt.Errorf("%w: cannot append %s content to tables",
			ErrKindMismatch, other.Kind())
	}

	merged := make([]NamedTable, 0, len(t.Tables)+len(ot.Tables))
	merged = append(merged, t.Tables...)
	merged = append(merged, ot.Tables...)

	return &Tables{Tables: merged}, nil
}

func init() {
	Register("tables", func() Content {
		return &Tables{}
	})
}
