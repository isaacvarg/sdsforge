package sections

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// VariantFile is one authored variant of one subsection.
type VariantFile struct {
	// Variant is the name, which must match the filename stem.
	Variant string `yaml:"variant"`

	// AppliesWhen is the derivation predicate. It is PARSED AND VALIDATED
	// today but never evaluated -- selection is manual until the H-code work
	// lands. Writing the field now means the whole content library is already
	// annotated when derivation arrives, instead of needing a second pass over
	// 77 files.
	AppliesWhen *Predicate `yaml:"applies_when"`

	// Priority breaks ties during derivation, where several predicates can
	// match at once (a substance that is both corrosive and acutely toxic by
	// inhalation matches both on 4.1). Highest wins; an exact tie is a
	// validation error, never an arbitrary pick.
	Priority int `yaml:"priority"`

	// Content is the block this variant contributes.
	Content Block `yaml:"content"`
}

// Predicate matches a document's set of GHS hazard codes.
type Predicate struct {
	AnyOf  []string `yaml:"any_of"`
	AllOf  []string `yaml:"all_of"`
	NoneOf []string `yaml:"none_of"`
}

// IsZero reports whether the predicate constrains anything at all.
func (p *Predicate) IsZero() bool {
	return p == nil || (len(p.AnyOf) == 0 && len(p.AllOf) == 0 && len(p.NoneOf) == 0)
}

// Matches evaluates the predicate against a set of hazard codes.
//
// Unused by the resolver today; it is the seam derivation plugs into. Tested
// now so it is known-good when that day comes.
func (p *Predicate) Matches(codes map[string]bool) bool {
	if p.IsZero() {
		return false // an unconstrained variant is never auto-selected
	}
	for _, c := range p.NoneOf {
		if codes[c] {
			return false
		}
	}
	for _, c := range p.AllOf {
		if !codes[c] {
			return false
		}
	}
	if len(p.AnyOf) > 0 {
		for _, c := range p.AnyOf {
			if codes[c] {
				return true
			}
		}
		return false
	}
	return true
}

// Preset is a named bundle of per-subsection variant choices -- what a user
// means when they ask for "the corrosive version of section 4".
//
// This is the piece that stops the combinatorial blowup: a section with 7
// subsections and 3 variants each needs 21 small variant files plus a handful
// of these, not 3^7 hand-written whole-section files.
type Preset struct {
	Preset string            `yaml:"preset"`
	Picks  map[string]string `yaml:"picks"`
}

// DefaultVariant is the variant name used when nothing selects otherwise.
const DefaultVariant = "default"

// LoadVariant reads one variant file for one subsection.
func LoadVariant(fsys fs.FS, sectionDir, subsectionID, variant string) (VariantFile, error) {
	if variant == "" {
		variant = DefaultVariant
	}
	p := path.Join(sectionDir, subsectionID, variant+".yaml")

	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return VariantFile{}, fmt.Errorf("reading variant %q for %s/%s: %w",
			variant, sectionDir, subsectionID, err)
	}

	var vf VariantFile
	if err := yaml.Unmarshal(data, &vf); err != nil {
		return VariantFile{}, fmt.Errorf("parsing %s: %w", p, err)
	}

	if vf.Content.Body == nil {
		return VariantFile{}, fmt.Errorf("%s: has no `content` block", p)
	}
	// The declared name and the filename must agree, or error messages and
	// preset lookups start disagreeing with each other.
	if vf.Variant != "" && vf.Variant != variant {
		return VariantFile{}, fmt.Errorf("%s: declares variant %q but is named %q",
			p, vf.Variant, variant)
	}
	vf.Variant = variant

	return vf, nil
}

// LoadPreset reads one preset file for a section.
func LoadPreset(fsys fs.FS, sectionDir, preset string) (Preset, error) {
	p := path.Join(sectionDir, "presets", preset+".yaml")

	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return Preset{}, fmt.Errorf("reading preset %q for %s: %w", preset, sectionDir, err)
	}

	var ps Preset
	if err := yaml.Unmarshal(data, &ps); err != nil {
		return Preset{}, fmt.Errorf("parsing %s: %w", p, err)
	}
	if len(ps.Picks) == 0 {
		return Preset{}, fmt.Errorf("%s: preset selects no subsections", p)
	}
	if ps.Preset != "" && ps.Preset != preset {
		return Preset{}, fmt.Errorf("%s: declares preset %q but is named %q", p, ps.Preset, preset)
	}
	ps.Preset = preset

	return ps, nil
}

// ErrUnknownPreset is returned when a document names a preset that does not exist.
var ErrUnknownPreset = errors.New("unknown preset")

// ErrUnknownVariant is returned when a document names a variant that does not exist.
var ErrUnknownVariant = errors.New("unknown variant")

// suggest formats an "available options" clause for an error message.
func suggest(available []string) string {
	if len(available) == 0 {
		return "none are defined"
	}
	return "available: " + strings.Join(available, ", ")
}
