package ghs

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// The reference tables live in the content library. ghs takes an fs.FS
// precisely so it does not need to know that -- os.DirFS is enough here, and
// the real caller passes the embedded library.
func tables(t *testing.T) *Tables {
	t.Helper()
	tbl, err := LoadTables(os.DirFS("../sections/osha"))
	if err != nil {
		t.Fatalf("LoadTables() error = %v", err)
	}
	return tbl
}

// LoadTables cross-references all three files; this is the guard against a
// hazard silently losing its precautionary statements.
func TestLoadTablesIsConsistent(t *testing.T) {
	tbl := tables(t)
	if got := len(tbl.Codes()); got < 70 {
		t.Errorf("loaded %d hazard codes, want the full Appendix C set", got)
	}
	for _, code := range tbl.Codes() {
		h, ok := tbl.Lookup(code)
		if !ok {
			t.Fatalf("Codes() returned %q but Lookup could not find it", code)
		}
		if h.Class == "" || h.Statement == "" {
			t.Errorf("%s is missing class or statement text: %+v", code, h)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	tests := []struct{ in, want string }{
		{"H315", "H315"},
		{"h315", "H315"},
		{"315", "H315"}, // YAML parses a bare 315 as an integer
		{" h315 ", "H315"},
		{"", ""},
		{"nonsense", "NONSENSE"}, // returned unchanged so errors name what was written
	}
	for _, tt := range tests {
		if got := NormalizeCode(tt.in); got != tt.want {
			t.Errorf("NormalizeCode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestClassifyUnknownCodeFails(t *testing.T) {
	_, err := tables(t).Classify([]string{"H314", "H999"})
	if !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("error = %v, want one wrapping ErrUnknownCode", err)
	}
	if !strings.Contains(err.Error(), "H999") {
		t.Errorf("error should name the offending code: %v", err)
	}
	if strings.Contains(err.Error(), "H314") {
		t.Errorf("error should not name the valid code: %v", err)
	}
}

func TestClassifyEmpty(t *testing.T) {
	c, err := tables(t).Classify(nil)
	if err != nil {
		t.Fatalf("Classify(nil) error = %v", err)
	}
	if c.IsClassified() {
		t.Error("IsClassified() = true for no codes")
	}
	if c.SignalWord != "" || len(c.Precautions) != 0 {
		t.Errorf("expected an empty classification, got %+v", c)
	}
}

// Danger displaces Warning regardless of the order codes are supplied in.
func TestClassifySignalWordPrecedence(t *testing.T) {
	tests := []struct {
		name  string
		codes []string
		want  string
	}{
		{"warning only", []string{"H315"}, SignalWarning},
		{"danger only", []string{"H314"}, SignalDanger},
		{"danger after warning", []string{"H315", "H350"}, SignalDanger},
		{"danger before warning", []string{"H350", "H315"}, SignalDanger},
		{"neither", []string{"H402"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := tables(t).Classify(tt.codes)
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if c.SignalWord != tt.want {
				t.Errorf("SignalWord = %q, want %q", c.SignalWord, tt.want)
			}
		})
	}
}

// Several hazards commonly share a pictogram; it must appear once.
func TestClassifyPictogramsDeduplicated(t *testing.T) {
	// H314 and H318 both carry GHS05.
	c, err := tables(t).Classify([]string{"H314", "H318"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	var codes []string
	for _, p := range c.Pictograms {
		codes = append(codes, p.Code)
	}
	if !slices.Equal(codes, []string{"GHS05"}) {
		t.Errorf("Pictograms = %v, want [GHS05] once", codes)
	}
	if c.Pictograms[0].Name != "corrosion" {
		t.Errorf("pictogram name = %q, want %q", c.Pictograms[0].Name, "corrosion")
	}
}

// The user's own example from the feature request.
func TestClassifyUserExample(t *testing.T) {
	c, err := tables(t).Classify([]string{"315", "350"}) // bare integers
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	if !slices.Equal(c.Codes, []string{"H315", "H350"}) {
		t.Errorf("Codes = %v", c.Codes)
	}
	if c.SignalWord != SignalDanger {
		t.Errorf("SignalWord = %q, want Danger (H350 carries it)", c.SignalWord)
	}

	var pics []string
	for _, p := range c.Pictograms {
		pics = append(pics, p.Code)
	}
	if !slices.Equal(pics, []string{"GHS07", "GHS08"}) {
		t.Errorf("Pictograms = %v, want [GHS07 GHS08]", pics)
	}

	statements := make(map[string]bool)
	for _, h := range c.Hazards {
		statements[h.Statement] = true
	}
	for _, want := range []string{"Causes skin irritation", "May cause cancer"} {
		if !statements[want] {
			t.Errorf("missing hazard statement %q; got %v", want, statements)
		}
	}
	if len(c.Precautions) == 0 {
		t.Fatal("no precautionary statements selected")
	}
}

// Prevention, response, storage, disposal -- the order Appendix C uses.
func TestClassifyPrecautionOrdering(t *testing.T) {
	c, err := tables(t).Classify([]string{"H314", "H350", "H225"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}

	last := -1
	for _, p := range c.Precautions {
		rank, ok := statementOrder[p.Type]
		if !ok {
			t.Fatalf("statement %s has unknown type %q", p.Code, p.Type)
		}
		if rank < last {
			t.Errorf("%s (%s) appears after a later-ranked type", p.Code, p.Type)
		}
		last = rank
	}

	// Deduplicated across hazards, and credited to every hazard that assigned it.
	seen := map[string]bool{}
	for _, p := range c.Precautions {
		if seen[p.Code] {
			t.Errorf("precautionary statement %s emitted twice", p.Code)
		}
		seen[p.Code] = true
		if len(p.TriggeredBy) == 0 {
			t.Errorf("%s is not credited to any hazard", p.Code)
		}
	}
	// P280 is assigned by all three of those hazards.
	for _, p := range c.Precautions {
		if p.Code == "P280" && len(p.TriggeredBy) < 3 {
			t.Errorf("P280 credited to %v, want all three hazards", p.TriggeredBy)
		}
	}
}

// Combination statements are single entries and must never be split.
func TestClassifyCombinationStatementsIntact(t *testing.T) {
	c, err := tables(t).Classify([]string{"H318"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	var found bool
	for _, p := range c.Precautions {
		if p.Code == "P305+P351+P338" {
			found = true
			if !strings.Contains(p.Statement, "Continue rinsing") {
				t.Errorf("combination statement text was truncated: %q", p.Statement)
			}
		}
		if p.Code == "P351" || p.Code == "P338" {
			t.Errorf("combination statement was split apart: %s", p.Code)
		}
	}
	if !found {
		t.Error("H318 did not select P305+P351+P338")
	}
}

// Exceeding the Appendix C guidance warns; it must never truncate.
func TestClassifyWarnsButDoesNotTruncate(t *testing.T) {
	c, err := tables(t).Classify([]string{"H314"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if len(c.Precautions) <= MaxPrecautionaryStatements {
		t.Fatalf("H314 selected only %d statements; this test needs more than %d",
			len(c.Precautions), MaxPrecautionaryStatements)
	}

	var warned bool
	for _, w := range c.Warnings {
		if strings.Contains(w, "no more than") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no warning about statement count; warnings = %v", c.Warnings)
	}

	// Every assigned statement is still present.
	assigned := len(tables(t).assignments["H314"])
	if len(c.Precautions) != assigned {
		t.Errorf("emitted %d statements, want all %d assigned to H314", len(c.Precautions), assigned)
	}
}

func TestApplyText(t *testing.T) {
	tbl := tables(t)

	c, err := tbl.Classify([]string{"H314"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	var genericBefore int
	for _, p := range c.Precautions {
		if p.SupplierSpecified {
			genericBefore++
		}
	}
	if genericBefore == 0 {
		t.Fatal("H314 selected no supplier-specified statements; this test needs one")
	}

	if err := c.ApplyText(map[string]string{"P260": "Do not breathe mist or spray."}); err != nil {
		t.Fatalf("ApplyText() error = %v", err)
	}
	for _, p := range c.Precautions {
		if p.Code == "P260" {
			if p.Statement != "Do not breathe mist or spray." {
				t.Errorf("P260 = %q, want the supplied text", p.Statement)
			}
			if p.SupplierSpecified {
				t.Error("P260 is still marked supplier-specified after an override")
			}
		}
	}

	// Wording for a statement this classification does not select is stale
	// text that must not reach a sheet unnoticed.
	err = c.ApplyText(map[string]string{"P501": "ok", "P999": "stale"})
	if err == nil || !strings.Contains(err.Error(), "P999") {
		t.Errorf("ApplyText() error = %v, want it to name the unselected statement", err)
	}
}

// Same input, same output -- the sheet must not churn between runs.
func TestClassifyIsDeterministic(t *testing.T) {
	tbl := tables(t)
	first, err := tbl.Classify([]string{"H350", "H314", "H225", "H318"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		next, err := tbl.Classify([]string{"H318", "H225", "H350", "H314"})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(first.Codes, next.Codes) {
			t.Fatalf("codes differ between runs: %v vs %v", first.Codes, next.Codes)
		}
		for j := range first.Precautions {
			if first.Precautions[j].Code != next.Precautions[j].Code {
				t.Fatalf("precaution order differs at %d: %s vs %s",
					j, first.Precautions[j].Code, next.Precautions[j].Code)
			}
		}
	}
}
