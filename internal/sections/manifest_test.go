package sections

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// os.DirFS turns a directory into an fs.FS. Because LoadSection takes an
// fs.FS, the tests read real fixture files with no temp-file juggling, and
// the exact same function later reads the embedded library.
func testFS(t *testing.T) fs.FS {
	t.Helper() // failures are reported at the CALLER's line, not inside here
	return os.DirFS("testdata")
}

func TestLoadSection(t *testing.T) {
	def, err := LoadSection(testFS(t), "04_first_aid")
	if err != nil {
		t.Fatalf("LoadSection() error = %v", err)
	}

	if def.ID != "first_aid" {
		t.Errorf("ID = %q, want %q", def.ID, "first_aid")
	}
	if def.Number != 4 {
		t.Errorf("Number = %d, want 4", def.Number)
	}
	if def.Dir != "04_first_aid" {
		t.Errorf("Dir = %q, want %q (set by the loader, not the YAML)", def.Dir, "04_first_aid")
	}
	if len(def.Subsections) != 3 {
		t.Fatalf("len(Subsections) = %d, want 3", len(def.Subsections))
	}
	// Order matters: it is the render order of the finished document.
	if def.Subsections[0].ID != "general" || def.Subsections[2].ID != "skin" {
		t.Errorf("subsection order not preserved: %+v", def.Subsections)
	}
	// empty_text cascades from the section to each subsection.
	if def.Subsections[1].EmptyText != "No data available." {
		t.Errorf("EmptyText = %q, want it inherited from the section", def.Subsections[1].EmptyText)
	}
}

func TestLoadSectionMissing(t *testing.T) {
	_, err := LoadSection(testFS(t), "99_does_not_exist")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want one wrapping fs.ErrNotExist", err)
	}
}

// The validator must report EVERY problem at once. An author fixing a manifest
// one error per run is an author who gives up.
func TestLoadSectionCollectsAllProblems(t *testing.T) {
	_, err := LoadSection(testFS(t), "bad_section")
	if err == nil {
		t.Fatal("LoadSection(bad_section) = nil error, want an error")
	}

	msg := err.Error()
	wantFragments := []string{
		"missing `id`",
		"missing `title`",
		"duplicate id",
		`unknown kind "proze"`,
		"known kinds: " + strings.Join(RegisteredKinds(), ", "), // free from the registry
	}
	for _, frag := range wantFragments {
		if !strings.Contains(msg, frag) {
			t.Errorf("error message missing %q\nfull message:\n%s", frag, msg)
		}
	}
	t.Logf("full message:\n%s", msg)
}

func TestSectionDefSubsectionLookup(t *testing.T) {
	def, err := LoadSection(testFS(t), "04_first_aid")
	if err != nil {
		t.Fatalf("LoadSection() error = %v", err)
	}

	sub, ok := def.Subsection("inhalation")
	if !ok {
		t.Fatal("Subsection(inhalation) not found")
	}
	if sub.Title != "Inhalation" {
		t.Errorf("Title = %q, want %q", sub.Title, "Inhalation")
	}
	if _, ok := def.Subsection("nope"); ok {
		t.Error("Subsection(nope) reported found")
	}
}

func TestLoadLayout(t *testing.T) {
	layout, err := LoadLayout(testFS(t))
	if err != nil {
		t.Fatalf("LoadLayout() error = %v", err)
	}
	if layout.Jurisdiction != "test" {
		t.Errorf("Jurisdiction = %q, want %q", layout.Jurisdiction, "test")
	}
	if len(layout.Sections) != 1 || layout.Sections[0] != "04_first_aid" {
		t.Errorf("Sections = %v", layout.Sections)
	}
}

// A scalar `kind:` must keep working exactly as it did: every shipped manifest
// but Section 12's is written that way.
func TestKindSetScalarForm(t *testing.T) {
	def, err := LoadSection(testFS(t), "04_first_aid")
	if err != nil {
		t.Fatalf("LoadSection() error = %v", err)
	}

	sub, ok := def.Subsection("general")
	if !ok {
		t.Fatal("Subsection(general) not found")
	}
	if got := sub.Kind.Primary(); got != "prose" {
		t.Errorf("Primary() = %q, want %q", got, "prose")
	}
	if !sub.Kind.Accepts("prose") {
		t.Error("Accepts(prose) = false, want true")
	}
	if sub.Kind.Accepts("table") {
		t.Error("Accepts(table) = true; a scalar kind must not widen")
	}
	if got := sub.Kind.String(); got != "prose" {
		t.Errorf("String() = %q, want %q", got, "prose")
	}
}

func TestKindSetSequenceForm(t *testing.T) {
	var set KindSet
	if err := yamlInto(`["prose", "table"]`, &set); err != nil {
		t.Fatalf("decoding kind sequence: %v", err)
	}

	if got := set.Primary(); got != "prose" {
		t.Errorf("Primary() = %q, want %q (the first entry)", got, "prose")
	}
	if !set.Accepts("prose") || !set.Accepts("table") {
		t.Errorf("Accepts() rejected a listed kind: %v", set)
	}
	if set.Accepts("images") {
		t.Error("Accepts(images) = true, want false")
	}
	if got, want := set.String(), "prose or table"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// Every entry is checked, not just the first -- a bad alternative would
// otherwise only surface the day someone tried to use it.
func TestKindSetRejectsUnknownAlternative(t *testing.T) {
	def := SectionDef{
		ID:    "ecological",
		Title: "Ecological information",
		Subsections: []SubsectionDef{
			{ID: "ecotoxicity", Title: "Ecotoxicity", Kind: KindSet{"prose", "tabel"}},
			{ID: "mobility", Title: "Mobility in soil", Kind: KindSet{"prose", "prose"}},
		},
	}

	err := def.validate("section.yaml")
	if err == nil {
		t.Fatal("validate() = nil error, want an error")
	}
	for _, frag := range []string{`unknown kind "tabel"`, `kind "prose" listed twice`} {
		if !strings.Contains(err.Error(), frag) {
			t.Errorf("error message missing %q\nfull message:\n%s", frag, err)
		}
	}
}

// A subsection with no `kind` at all names the problem rather than silently
// accepting nothing.
func TestKindSetRejectsEmpty(t *testing.T) {
	def := SectionDef{
		ID:          "ecological",
		Title:       "Ecological information",
		Subsections: []SubsectionDef{{ID: "ecotoxicity", Title: "Ecotoxicity"}},
	}

	err := def.validate("section.yaml")
	if err == nil {
		t.Fatal("validate() = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "missing `kind`") {
		t.Errorf("error message missing %q: %v", "missing `kind`", err)
	}
}
