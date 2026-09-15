package sections

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
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

// KindSet is the content kinds a subsection accepts.
//
// Most subsections take exactly one shape and say so with a scalar. A few take
// more than one -- Section 12's ecological data is prose for a product with
// nothing to report and a table for one with real study results -- and say so
// with a sequence:
//
//	kind: "prose"
//	kind: ["prose", "table"]
//
// The FIRST entry is primary: it is the kind the library's own default.yaml
// must be, and the one the scaffold and `sections list` show. Everything after
// it is an alternative a document may supply through replace or append.
type KindSet []string

// UnmarshalYAML accepts either form under the one `kind:` key.
//
// A second key (`kinds:`) would mean two spellings of one idea and a rule
// about setting both; widening the existing key keeps all 47 single-kind
// manifests valid verbatim.
func (k *KindSet) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var one string
		if err := node.Decode(&one); err != nil {
			return fmt.Errorf("line %d: `kind` must be a string or a list of strings: %w",
				node.Line, err)
		}
		*k = KindSet{one}
		return nil
	case yaml.SequenceNode:
		var many []string
		if err := node.Decode(&many); err != nil {
			return fmt.Errorf("line %d: `kind` list must hold strings: %w", node.Line, err)
		}
		*k = KindSet(many)
		return nil
	default:
		return fmt.Errorf("line %d: `kind` must be a string or a list of strings", node.Line)
	}
}

// Primary is the kind the default variant must be. Empty for an empty set,
// which SectionDef.validate rejects.
func (k KindSet) Primary() string {
	if len(k) == 0 {
		return ""
	}
	return k[0]
}

// Accepts reports whether a content block of this kind may be used here.
func (k KindSet) Accepts(kind string) bool {
	return slices.Contains(k, kind)
}

// String renders the set for an error message or a listing: "prose", or
// "prose or table".
func (k KindSet) String() string {
	switch len(k) {
	case 0:
		return "(none)"
	case 1:
		return k[0]
	case 2:
		return k[0] + " or " + k[1]
	default:
		return strings.Join(k[:len(k)-1], ", ") + " or " + k[len(k)-1]
	}
}

// SubsectionDef declares one subsection within a section.
type SubsectionDef struct {
	ID        string  `yaml:"id"`
	Title     string  `yaml:"title"`
	Kind      KindSet `yaml:"kind"`
	EmptyText string  `yaml:"empty_text"`

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
		//
		// Every entry is checked, not just the first: a bad alternative is just
		// as silently broken as a bad primary, and it would only surface the
		// day someone tried to use it.
		switch {
		case len(sub.Kind) == 0:
			problems = append(problems, fmt.Errorf(
				"subsection %q: missing `kind`; known kinds: %s",
				sub.ID, strings.Join(RegisteredKinds(), ", ")))
		default:
			for _, kind := range sub.Kind {
				if _, ok := registry[kind]; !ok {
					problems = append(problems, fmt.Errorf(
						"subsection %q: unknown kind %q; known kinds: %s",
						sub.ID, kind, strings.Join(RegisteredKinds(), ", ")))
				}
			}
			if dup := firstDuplicate(sub.Kind); dup != "" {
				problems = append(problems, fmt.Errorf(
					"subsection %q: kind %q listed twice", sub.ID, dup))
			}
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

// firstDuplicate returns the first kind listed more than once, or "".
//
// A repeat is harmless at resolve time but always a mistake in the manifest,
// and it would quietly widen a `oneOf` in the generated schema.
func firstDuplicate(kinds KindSet) string {
	seen := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		if seen[kind] {
			return kind
		}
		seen[kind] = true
	}
	return ""
}
