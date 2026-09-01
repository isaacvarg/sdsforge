package sections

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultEmptyText is substituted when a subsection resolves to no content.
const DefaultEmptyText = "No data available."

// SectionDef is one section's manifest, loaded from its section.yaml. It
// describes the SHAPE of a section -- which subsections exist, in what order,
// and what content kind each one holds. It carries no content itself; that
// lives in the variant files.
type SectionDef struct {
	ID          string          `yaml:"id"`
	Title       string          `yaml:"title"`
	Number      int             `yaml:"number"`
	EmptyText   string          `yaml:"empty_text"`
	Subsections []SubsectionDef `yaml:"subsections"`

	// Dir is the directory this manifest was loaded from, e.g.
	// "04_first_aid". Set by LoadSection, not read from YAML.
	Dir string `yaml:"-"`
}

// SubsectionDef declares one subsection within a section.
type SubsectionDef struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Kind      string `yaml:"kind"`
	EmptyText string `yaml:"empty_text"`

	// Source names the document data that populates this subsection, if any.
	// Empty means the content is entirely authored in the library. See
	// source.go for the valid names.
	Source string `yaml:"source"`
}

// Layout is the ordered list of section directories for a jurisdiction,
// loaded from layout.yaml at the root of the library.
type Layout struct {
	Jurisdiction string   `yaml:"jurisdiction"`
	Sections     []string `yaml:"sections"`
}

// LoadLayout reads layout.yaml from the root of fsys.
func LoadLayout(fsys fs.FS) (Layout, error) {
	data, err := fs.ReadFile(fsys, "layout.yaml")
	if err != nil {
		return Layout{}, fmt.Errorf("reading layout.yaml: %w", err)
	}

	var layout Layout
	if err := yaml.Unmarshal(data, &layout); err != nil {
		return Layout{}, fmt.Errorf("parsing layout.yaml: %w", err)
	}
	if len(layout.Sections) == 0 {
		return Layout{}, errors.New("layout.yaml lists no sections")
	}
	return layout, nil
}

// LoadSection reads and validates dir/section.yaml from fsys.
//
// Taking an fs.FS rather than a filesystem path is what lets this same
// function serve the embedded library, a user's overlay directory, and
// testdata/ in tests, with no changes and no temp files.
func LoadSection(fsys fs.FS, dir string) (SectionDef, error) {
	manifestPath := path.Join(dir, "section.yaml")

	data, err := fs.ReadFile(fsys, manifestPath)
	if err != nil {
		return SectionDef{}, fmt.Errorf("reading %s: %w", manifestPath, err)
	}

	var def SectionDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return SectionDef{}, fmt.Errorf("parsing %s: %w", manifestPath, err)
	}
	def.Dir = dir

	if err := def.validate(manifestPath); err != nil {
		return SectionDef{}, err
	}

	// Apply defaults after validation so a blank empty_text is filled rather
	// than rejected.
	if def.EmptyText == "" {
		def.EmptyText = DefaultEmptyText
	}
	for i := range def.Subsections {
		if def.Subsections[i].EmptyText == "" {
			def.Subsections[i].EmptyText = def.EmptyText
		}
	}

	return def, nil
}

// validate collects every problem rather than stopping at the first, so an
// author fixing a manifest sees the whole list in one pass.
func (s SectionDef) validate(where string) error {
	var problems []error

	if strings.TrimSpace(s.ID) == "" {
		problems = append(problems, errors.New("missing `id`"))
	}
	if strings.TrimSpace(s.Title) == "" {
		problems = append(problems, errors.New("missing `title`"))
	}
	if len(s.Subsections) == 0 {
		problems = append(problems, errors.New("declares no subsections"))
	}

	seen := make(map[string]int, len(s.Subsections))
	for i, sub := range s.Subsections {
		switch {
		case strings.TrimSpace(sub.ID) == "":
			problems = append(problems, fmt.Errorf("subsection %d: missing `id`", i))
		default:
			if first, dup := seen[sub.ID]; dup {
				problems = append(problems, fmt.Errorf(
					"subsection %d: duplicate id %q (already used by subsection %d)", i, sub.ID, first))
			}
			seen[sub.ID] = i
		}

		if strings.TrimSpace(sub.Title) == "" {
			problems = append(problems, fmt.Errorf("subsection %q: missing `title`", sub.ID))
		}

		// The registry pays off here: a typo like `kind: proze` fails at load
		// with the valid options listed, instead of silently producing an
		// empty subsection in a rendered safety document.
		if _, ok := registry[sub.Kind]; !ok {
			problems = append(problems, fmt.Errorf(
				"subsection %q: unknown kind %q; known kinds: %s",
				sub.ID, sub.Kind, strings.Join(RegisteredKinds(), ", ")))
		}

		// A misspelled source would silently mean "no data binding", so the
		// subsection would quietly render its placeholder forever.
		if sub.Source != "" && !knownSources[sub.Source] {
			problems = append(problems, fmt.Errorf(
				"subsection %q: unknown source %q; known sources: %s",
				sub.ID, sub.Source, suggestSources()))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	// errors.Join bundles them into one error whose message lists each on its
	// own line, and whose errors.Is still matches any of the constituents.
	return fmt.Errorf("%s is invalid:\n%w", where, errors.Join(problems...))
}

// Subsection looks up a subsection declaration by id.
func (s SectionDef) Subsection(id string) (SubsectionDef, bool) {
	for _, sub := range s.Subsections {
		if sub.ID == id {
			return sub, true
		}
	}
	return SubsectionDef{}, false
}
