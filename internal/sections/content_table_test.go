package sections

import (
	"errors"
	"slices"
	"testing"

	"gopkg.in/yaml.v3"
)

// equalRows compares [][]string.
//
// slices.Equal only works when the elements are `comparable`, and []string is
// not (slices cannot be compared with ==). slices.EqualFunc takes a custom
// per-element comparison instead -- and for []string elements, that comparison
// is slices.Equal itself.
func equalRows(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool {
		return slices.Equal(x, y)
	})
}

func TestTableKind(t *testing.T) {
	if got := (&Table{}).Kind(); got != "table" {
		t.Errorf("Kind() = %q, want %q", got, "table")
	}
}

func TestTableIsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		rows    [][]string
		want    bool
	}{
		{"zero value", nil, nil, true},
		// The deliberate call: column titles with no data underneath should
		// render "No data available.", not an empty skeleton table.
		{"headers but no rows", []string{"Chemical", "TLV"}, nil, true},
		{"row of blank cells", []string{"Chemical"}, [][]string{{"  ", ""}}, true},
		{"real data", []string{"Chemical"}, [][]string{{"Acetone"}}, false},
		{"blank row then real row", nil, [][]string{{""}, {"Toluene"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tbl := &Table{Headers: tt.headers, Rows: tt.rows}
			if got := tbl.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTableAppend(t *testing.T) {
	base := &Table{
		Headers: []string{"Chemical", "CAS No."},
		Rows:    [][]string{{"Acetone", "67-64-1"}},
	}
	// An override supplying only rows inherits the base's headers.
	extra := &Table{Rows: [][]string{{"Toluene", "108-88-3"}}}

	result, err := base.Append(extra)
	if err != nil {
		t.Fatalf("Append() unexpected error: %v", err)
	}

	got, ok := result.(*Table)
	if !ok {
		t.Fatalf("Append() returned %T, want *Table", result)
	}

	wantRows := [][]string{
		{"Acetone", "67-64-1"},
		{"Toluene", "108-88-3"},
	}
	if !equalRows(got.Rows, wantRows) {
		t.Errorf("Rows = %q, want %q", got.Rows, wantRows)
	}
	if !slices.Equal(got.Headers, base.Headers) {
		t.Errorf("Headers = %q, want %q (inherited from receiver)", got.Headers, base.Headers)
	}
}

// Same aliasing guard as the prose version: two appends off one base must not
// share a backing array, or the second silently corrupts the first.
func TestTableAppendDoesNotAlias(t *testing.T) {
	base := &Table{
		Headers: []string{"Chemical"},
		Rows:    make([][]string, 1, 8), // len 1, cap 8 -- room for append to reuse
	}
	base.Rows[0] = []string{"Acetone"}

	first, err := base.Append(&Table{Rows: [][]string{{"Toluene"}}})
	if err != nil {
		t.Fatalf("first Append() error: %v", err)
	}
	second, err := base.Append(&Table{Rows: [][]string{{"Xylene"}}})
	if err != nil {
		t.Fatalf("second Append() error: %v", err)
	}

	if got := first.(*Table).Rows; !equalRows(got, [][]string{{"Acetone"}, {"Toluene"}}) {
		t.Errorf("first result was mutated by the second Append: got %q", got)
	}
	if got := second.(*Table).Rows; !equalRows(got, [][]string{{"Acetone"}, {"Xylene"}}) {
		t.Errorf("second result = %q, want [[Acetone] [Xylene]]", got)
	}
	if got := base.Rows; !equalRows(got, [][]string{{"Acetone"}}) {
		t.Errorf("Append mutated its receiver: base.Rows = %q", got)
	}
}

// The three failure modes, asserted with errors.Is rather than string
// matching. errors.Is looks THROUGH the %w wrapping to find the sentinel, so
// these keep passing even if the surrounding message is reworded.
func TestTableAppendErrors(t *testing.T) {
	base := &Table{
		Headers: []string{"Chemical", "CAS No."},
		Rows:    [][]string{{"Acetone", "67-64-1"}},
	}

	tests := []struct {
		name    string
		other   Content
		wantErr error
	}{
		{
			"not a table",
			&stubContent{}, // defined in content_test.go, same package
			ErrKindMismatch,
		},
		{
			"conflicting headers",
			&Table{Headers: []string{"Substance", "CAS"}, Rows: [][]string{{"a", "b"}}},
			ErrHeaderMismatch,
		},
		{
			"row too short",
			&Table{Rows: [][]string{{"Toluene"}}}, // 1 cell, table has 2 columns
			ErrRaggedRow,
		},
		{
			"row too long",
			&Table{Rows: [][]string{{"Toluene", "108-88-3", "extra"}}},
			ErrRaggedRow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := base.Append(tt.other)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Append() error = %v, want one wrapping %v", err, tt.wantErr)
			}
			t.Logf("message: %v", err) // visible under -v; judge its usefulness
		})
	}
}

// ---------------------------------------------------------------------------
// The thesis of Phase 2.
//
// content.go was never opened to add this kind, yet Block now dispatches to
// *Table purely because content_table.go's init() registered itself. If this
// passes, the registry design holds.
// ---------------------------------------------------------------------------

func TestBlockUnmarshalTable(t *testing.T) {
	src := `
kind: table
headers: ["Chemical", "CAS No.", "OSHA PEL (TWA)"]
rows:
  - ["Acetone", "67-64-1", "1000 ppm"]
  - ["Toluene", "108-88-3", "200 ppm"]
`

	var blk Block
	if err := yaml.Unmarshal([]byte(src), &blk); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	tbl, ok := blk.Body.(*Table)
	if !ok {
		t.Fatalf("Body is %T, want *Table", blk.Body)
	}
	if !slices.Equal(tbl.Headers, []string{"Chemical", "CAS No.", "OSHA PEL (TWA)"}) {
		t.Errorf("Headers = %q", tbl.Headers)
	}
	if len(tbl.Rows) != 2 {
		t.Fatalf("len(Rows) = %d, want 2", len(tbl.Rows))
	}
	if tbl.Rows[1][2] != "200 ppm" {
		t.Errorf("Rows[1][2] = %q, want %q", tbl.Rows[1][2], "200 ppm")
	}
}

// Registering a second kind should also improve the error messages from
// Phase 1 for free: "known kinds" now lists both.
func TestRegisteredKindsIncludesAll(t *testing.T) {
	want := []string{"images", "prose", "table"}
	if got := RegisteredKinds(); !slices.Equal(got, want) {
		t.Errorf("RegisteredKinds() = %v, want %v", got, want)
	}
}
