package schema_test

import (
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/schema"
)

// validDocument is the full example from docs/document-yaml.md, plus a TSCA
// block and an unquoted date-like string.
const validDocument = `product_name: Acetone-Toluene Blend

hazard_codes: [H225, h319, 336]

identification:
  product_codes: ["ATB-100"]
  synonyms: 2024-01-01
  cas_number: ""
  recommended_use: "Industrial degreasing solvent"
  restrictions: "Not for consumer use"

precautionary_text:
  P210: "Keep away from heat, hot surfaces, sparks, open flames."

materials:
  - name: Acetone
    cas_number: "67-64-1"
    percentage: "60"
    hazard_codes: [H225, H319, H336]
  - name: Toluene
    cas_number: "108-88-3"
    percentage: "40"
    hazard_codes: [H225, H304, H315, H336, H361, H373]

prop65:
  - chemical: "Toluene"
    exposure: reproductive_toxicant

tsca_inventory:
  - material: "Toluene"
    cas_number: "108-88-3"
    basis: "Active"
  - material: "Water"
    cas_number: "7732-18-5"
    basis: ""

sections:
  exposure_controls:
    subsections:
      exposure_limits:
        replace:
          kind: table
          headers: ["Chemical name", "CAS No.", "Basis", "Exposure Limit"]
          rows:
            - ["Acetone", "67-64-1", "OSHA PEL (TWA)", "1000 ppm"]
`

func validate(t *testing.T, src string) []schema.Problem {
	t.Helper()
	problems, err := schema.Validate(builtinLibrary(t), []byte(src))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return problems
}

func TestValidateAcceptsValidDocument(t *testing.T) {
	if problems := validate(t, validDocument); len(problems) != 0 {
		t.Fatalf("expected no problems, got:\n%s", format(problems))
	}
}

// A freshly created document must not be flagged: the scaffold is the first
// thing anyone validates.
func TestValidateAcceptsScaffold(t *testing.T) {
	raw, err := document.Scaffold(builtinLibrary(t), "Test Product")
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if problems := validate(t, string(raw)); len(problems) != 0 {
		t.Fatalf("scaffold flagged:\n%s", format(problems))
	}
}

func TestValidateReportsEmptyDocument(t *testing.T) {
	problems := validate(t, "")
	if len(problems) != 1 || problems[0].Message != "document is empty" {
		t.Fatalf("expected a single empty-document problem, got:\n%s", format(problems))
	}
}

func TestValidateRejectsYAMLSyntax(t *testing.T) {
	for _, src := range []string{
		"product_name: [unclosed",
		"product_name: X\nproduct_name: Y\n",
	} {
		if _, err := schema.Validate(builtinLibrary(t), []byte(src)); err == nil {
			t.Errorf("expected a parse error for %q", src)
		}
	}
}

func TestValidateReportsProblems(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantLine int
		wantPath string
		wantMsg  string
	}{
		{
			name:     "unknown top-level key points at the key",
			src:      "product_name: X\nhazzard_codes: [H225]\n",
			wantLine: 2,
			wantPath: "hazzard_codes",
			wantMsg:  `unknown key "hazzard_codes"`,
		},
		{
			name:     "unknown section id",
			src:      "product_name: X\nsections:\n  first_aide: {}\n",
			wantLine: 3,
			wantPath: "sections.first_aide",
			wantMsg:  "unknown key",
		},
		{
			name:     "hazard code inside a sequence",
			src:      "product_name: X\nhazard_codes:\n  - H225\n  - H999\n",
			wantLine: 4,
			wantPath: "hazard_codes[1]",
			wantMsg:  `"H999"`,
		},
		{
			name: "unknown variant",
			src: "product_name: X\nsections:\n  regulatory:\n    subsections:\n" +
				"      tsca_inventory:\n        variant: nope\n",
			wantLine: 6,
			wantPath: "sections.regulatory.subsections.tsca_inventory.variant",
		},
		{
			name: "extra key on a tsca entry",
			src: "product_name: X\ntsca_inventory:\n  - material: Water\n" +
				"    cas_number: 7732-18-5\n    listed: true\n",
			wantLine: 5,
			wantPath: "tsca_inventory[0].listed",
			wantMsg:  "unknown key",
		},
		{
			name:     "wrong type",
			src:      "product_name: [a, b]\n",
			wantLine: 1,
			wantPath: "product_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := validate(t, tt.src)
			if len(problems) != 1 {
				t.Fatalf("expected 1 problem, got %d:\n%s", len(problems), format(problems))
			}
			p := problems[0]
			if p.Line != tt.wantLine || p.Path != tt.wantPath || !strings.Contains(p.Message, tt.wantMsg) {
				t.Errorf("got %s\nwant line %d, path %s, message containing %q",
					p, tt.wantLine, tt.wantPath, tt.wantMsg)
			}
		})
	}
}

func TestValidateReportsEveryProblemInOrder(t *testing.T) {
	problems := validate(t, "bogus: 1\nproduct_name: X\nalso_bogus: 2\n")
	if len(problems) != 2 || problems[0].Line != 1 || problems[1].Line != 3 {
		t.Fatalf("expected problems on lines 1 and 3, got:\n%s", format(problems))
	}
}

func format(problems []schema.Problem) string {
	var b strings.Builder
	for _, p := range problems {
		b.WriteString("  " + p.String() + "\n")
	}
	return b.String()
}
