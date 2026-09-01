package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/sections"
	"gopkg.in/yaml.v3"
)

func testLibrary(t *testing.T) *sections.Library {
	t.Helper()
	lib, err := sections.NewLibrary(sections.LibraryOptions{})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}
	return lib
}

// The drift guard.
//
// The scaffold is authored text rather than a marshalled struct, so nothing
// stops it naming a field that Data does not have. KnownFields(true) makes the
// decoder reject any unknown key, which turns that risk into a test failure.
func TestScaffoldDecodesIntoData(t *testing.T) {
	out, err := Scaffold(testLibrary(t), "Caustic Soda 50%")
	if err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(out))
	dec.KnownFields(true)

	var data Data
	if err := dec.Decode(&data); err != nil {
		t.Fatalf("scaffold does not decode into Data: %v\n\n%s", err, out)
	}

	if data.ProductName != "Caustic Soda 50%" {
		t.Errorf("ProductName = %q, want the name passed in", data.ProductName)
	}
	// Everything under sections: ships commented out, so a fresh document
	// resolves entirely to defaults.
	if len(data.Sections) != 0 {
		t.Errorf("Sections = %v, want empty in a fresh scaffold", data.Sections)
	}
}

// A name containing YAML metacharacters must not corrupt the file.
func TestScaffoldQuotesAwkwardNames(t *testing.T) {
	for _, name := range []string{"Acid: 10% mix", "  padded  ", `Say "hi"`, "#1 Cleaner", ""} {
		t.Run(name, func(t *testing.T) {
			out, err := Scaffold(testLibrary(t), name)
			if err != nil {
				t.Fatalf("Scaffold() error = %v", err)
			}
			dec := yaml.NewDecoder(bytes.NewReader(out))
			dec.KnownFields(true)

			var data Data
			if err := dec.Decode(&data); err != nil {
				t.Fatalf("scaffold with name %q does not parse: %v", name, err)
			}
			if data.ProductName != name {
				t.Errorf("ProductName = %q, want %q", data.ProductName, name)
			}
		})
	}
}

// The scaffold is generated from the live library, so every section must
// appear. If a section is added to the library and this fails, the generator
// stopped enumerating rather than the test being wrong.
func TestScaffoldNamesEverySection(t *testing.T) {
	lib := testLibrary(t)

	out, err := Scaffold(lib, "Test")
	if err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}
	text := string(out)

	layout, err := sections.LoadLayout(lib)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range layout.Sections {
		def, err := sections.LoadSection(lib, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(text, "#  "+def.ID+":") {
			t.Errorf("scaffold does not offer section %q", def.ID)
		}
		if !strings.Contains(text, def.Title) {
			t.Errorf("scaffold does not name section title %q", def.Title)
		}
	}

	// Presets and variants must be advertised, not just section ids.
	for _, want := range []string{
		"presets: acute_inhalation, corrosive",
		"- inhalation: acute_toxicity | corrosive | default",
		"filled from `materials` above",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("scaffold missing %q", want)
		}
	}
}

// Uncommenting an entry -- deleting the single leading "#" -- must yield valid,
// correctly indented YAML that actually resolves. This is the promise the
// file's own header makes, tested one section at a time, which is how a person
// really edits the file.
func TestScaffoldUncommentsCleanly(t *testing.T) {
	lib := testLibrary(t)

	out, err := Scaffold(lib, "Test")
	if err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}
	lines := strings.Split(string(out), "\n")

	// Split the file at "sections:"; everything after it is section entries.
	head := 0
	for i, line := range lines {
		if line == "sections:" {
			head = i
			break
		}
	}
	if head == 0 {
		t.Fatal("scaffold has no `sections:` key")
	}
	preamble := strings.Join(lines[:head+1], "\n")

	// Group the enableable lines ("#  ...") into one block per section.
	var blocks [][]string
	for _, line := range lines[head+1:] {
		if !strings.HasPrefix(line, "#  ") {
			continue
		}
		yamlLine := strings.TrimPrefix(line, "#")
		// A line indented exactly two spaces starts a new section entry.
		if !strings.HasPrefix(yamlLine, "   ") {
			blocks = append(blocks, []string{yamlLine})
			continue
		}
		if len(blocks) == 0 {
			t.Fatalf("continuation line before any section entry: %q", line)
		}
		blocks[len(blocks)-1] = append(blocks[len(blocks)-1], yamlLine)
	}

	if len(blocks) == 0 {
		t.Fatal("no enableable section entries found")
	}

	for _, block := range blocks {
		name := strings.TrimSpace(strings.TrimSuffix(block[0], ":"))
		t.Run(name, func(t *testing.T) {
			src := preamble + "\n" + strings.Join(block, "\n") + "\n"

			dec := yaml.NewDecoder(strings.NewReader(src))
			dec.KnownFields(true)

			var data Data
			if err := dec.Decode(&data); err != nil {
				t.Fatalf("enabled entry does not parse: %v\n\n%s", err, src)
			}
			if len(data.Sections) != 1 {
				t.Fatalf("expected exactly one section selection, got %v", data.Sections)
			}
			if _, ok := data.Sections[name]; !ok {
				t.Fatalf("selection is not keyed by %q: %v", name, data.Sections)
			}

			// And it must resolve against the real library.
			if _, err := sections.ResolveAll(lib, data.Sections, sections.ResolveContext{Sources: data.SourceData(nil)}); err != nil {
				t.Fatalf("enabled entry does not resolve: %v", err)
			}
		})
	}
}

// The two comment styles must stay distinguishable: anything that is not valid
// YAML may not use the enableable "#  " form.
func TestScaffoldCommentStylesAreUnambiguous(t *testing.T) {
	out, err := Scaffold(testLibrary(t), "Test")
	if err != nil {
		t.Fatalf("Scaffold() error = %v", err)
	}
	for i, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "#") || strings.HasPrefix(line, "#  ") {
			continue
		}
		// Everything else must be prose: hash, then at most one space.
		if strings.HasPrefix(line, "# ") && strings.HasPrefix(strings.TrimPrefix(line, "# "), " ") {
			t.Errorf("line %d is ambiguous between prose and enableable YAML: %q", i+1, line)
		}
	}
}
