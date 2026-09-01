package ghs

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every hazard code named by a variant's applies_when must exist in the
// reference table.
//
// This is the check that catches a typo in either direction: a misspelled code
// in the content library would silently never match, and a variant would sit
// unused while the sheet quietly showed default wording.
//
// It parses only the applies_when field, so the ghs package stays free of any
// dependency on the sections package.
func TestContentLibraryCodesExist(t *testing.T) {
	tbl := tables(t)
	root := os.DirFS("../sections/osha")

	type usage struct{ file, code string }
	var (
		unknown []usage
		used    = map[string]bool{}
	)

	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Variant files only: <section>/<subsection>/<variant>.yaml
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") || strings.Count(p, "/") != 2 {
			return nil
		}
		if path.Base(path.Dir(p)) == "presets" {
			return nil
		}

		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		var doc struct {
			AppliesWhen struct {
				AnyOf  []string `yaml:"any_of"`
				AllOf  []string `yaml:"all_of"`
				NoneOf []string `yaml:"none_of"`
			} `yaml:"applies_when"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return err
		}

		for _, list := range [][]string{doc.AppliesWhen.AnyOf, doc.AppliesWhen.AllOf, doc.AppliesWhen.NoneOf} {
			for _, code := range list {
				used[code] = true
				if _, ok := tbl.Lookup(code); !ok {
					unknown = append(unknown, usage{p, code})
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the content library: %v", err)
	}

	for _, u := range unknown {
		t.Errorf("%s: applies_when names %q, which is not in hazard_statements.yaml", u.file, u.code)
	}
	if len(used) == 0 {
		t.Fatal("no applies_when codes found at all; the walk is not finding variant files")
	}

	codes := make([]string, 0, len(used))
	for c := range used {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	t.Logf("content library predicates reference %d codes: %s", len(codes), strings.Join(codes, ", "))
}

// Every code in the table must classify on its own without error. A code that
// loads but cannot be classified is a table that passes validation and still
// fails in production.
func TestEveryCodeClassifies(t *testing.T) {
	tbl := tables(t)
	for _, code := range tbl.Codes() {
		c, err := tbl.Classify([]string{code})
		if err != nil {
			t.Errorf("Classify(%s) error = %v", code, err)
			continue
		}
		if len(c.Hazards) != 1 {
			t.Errorf("Classify(%s) produced %d hazards, want 1", code, len(c.Hazards))
		}
		if len(c.Precautions) == 0 {
			t.Errorf("Classify(%s) selected no precautionary statements", code)
		}
	}
}

// A table with a dangling reference must fail to load rather than silently
// dropping a hazard's precautionary statements.
func TestLoadTablesRejectsDanglingReference(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir+"/ghs", 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(dir+"/ghs/"+name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hazard_statements.yaml", `
pictograms:
  - {code: GHS05, name: corrosion}
statements:
  - {code: H314, class: Skin, category: "1", statement: Burns, signal_word: Danger, pictogram: GHS05}
`)
	write("precautionary.yaml", `
statements:
  - {code: P280, type: prevention, statement: Wear gloves.}
`)
	write("assignments.yaml", `
assignments:
  - {hazard: H314, precautionary: [P280, P999]}
`)

	_, err := LoadTables(os.DirFS(dir))
	if err == nil {
		t.Fatal("LoadTables() = nil error, want a dangling-reference error")
	}
	if !strings.Contains(err.Error(), "P999") {
		t.Errorf("error should name the dangling statement: %v", err)
	}
}
