package sections

import (
	"fmt"
	"slices"
	"strings"
)

// Table is tabular content -- Section 8's exposure limits, Section 9's
// physical properties, and so on:
//
//	kind: table
//	headers: ["Chemical", "CAS No.", "OSHA PEL (TWA)"]
//	rows:
//	  - ["Acetone", "67-64-1", "1000 ppm"]
//
// Every cell is a string. Resist the urge to type them further: SDS tables are
// full of values like "1000 ppm", "N/E", and "not established" that are not
// numbers and must render exactly as authored.
type Table struct {
	Headers []string   `yaml:"headers"`
	Rows    [][]string `yaml:"rows"`
}

// Compile-time proof that *Table satisfies Content.
var _ Content = (*Table)(nil)

func (t *Table) Kind() string {
	return "table"
}

// IsEmpty reports whether this table has any actual data.
//
// Deliberate decision: a table with headers but no rows IS empty. Column
// titles with nothing under them are precisely the case that should render
// "No data available." rather than a skeleton table. Blank-only cells count as
// empty too, mirroring how Prose treats whitespace-only paragraphs.
func (t *Table) IsEmpty() bool {
	for _, row := range t.Rows {
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				return false
			}
		}
	}
	return true
}

// columns reports how many columns this table has.
//
// Headers are authoritative when present. Falling back to the first row lets a
// header-less table still be width-checked.
func (t *Table) columns() int {
	if len(t.Headers) > 0 {
		return len(t.Headers)
	}
	if len(t.Rows) > 0 {
		return len(t.Rows[0])
	}
	return 0
}

// Append adds another table's rows to this one, returning a new Table.
//
// Three failure modes, each a distinct sentinel so Phase 8 can report them
// separately:
//
//   - appending something that is not a table at all
//   - appending a table whose headers disagree with ours
//   - appending a row with the wrong number of cells
//
// That last one matters more than it looks. A short row is invisible in
// rendered HTML -- the row just comes out narrow -- so an unchecked mismatch
// becomes a silently wrong safety document.
func (t *Table) Append(other Content) (Content, error) {
	ot, isTable := other.(*Table)
	if !isTable {
		return nil, fmt.Errorf("%w: cannot append %s content to table",
			ErrKindMismatch, other.Kind())
	}

	// Header reconciliation. An override that supplies only rows inherits our
	// headers; one that supplies conflicting headers is a document bug, not
	// something to silently resolve by picking a winner.
	headers := t.Headers
	switch {
	case len(headers) == 0:
		headers = ot.Headers
	case len(ot.Headers) > 0 && !slices.Equal(headers, ot.Headers):
		return nil, fmt.Errorf("%w: cannot append table with headers %q to table with headers %q",
			ErrHeaderMismatch, ot.Headers, headers)
	}

	// Establish the expected width, preferring headers, then our own rows,
	// then the incoming ones.
	width := len(headers)
	if width == 0 {
		if width = t.columns(); width == 0 {
			width = ot.columns()
		}
	}

	// Same make-then-append pattern as Prose.Append, for the same reason: a
	// bare append(t.Rows, ...) could write into t's backing array when it has
	// spare capacity, corrupting a variant that other documents also use.
	//
	// Note this copies the OUTER slice only -- the individual row slices are
	// still shared with t and ot. That is fine here because content is treated
	// as immutable once loaded; nothing ever writes into a row in place. If
	// that ever stops being true, this needs a deep copy.
	merged := make([][]string, 0, len(t.Rows)+len(ot.Rows))
	merged = append(merged, t.Rows...)

	for i, row := range ot.Rows {
		// Only the incoming rows are checked. Validating the receiver's own
		// rows is Phase 8's job, at load time, where the error can name the
		// file and line rather than an index in an append operation.
		if len(row) != width {
			return nil, fmt.Errorf("%w: appended row %d has %d cells, want %d",
				ErrRaggedRow, i, len(row), width)
		}
		merged = append(merged, row)
	}

	return &Table{Headers: headers, Rows: merged}, nil
}

// This file never touches content.go. Adding a content kind means: one struct,
// three methods, and one init() that announces itself to the registry. That
// decoupling is the whole point of the registry design.
func init() {
	Register("table", func() Content {
		return &Table{}
	})
}
