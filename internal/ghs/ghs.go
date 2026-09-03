// Package ghs computes a GHS classification from a set of hazard codes.
//
// It is deliberately self-contained: it imports neither the sections nor the
// document package, takes an fs.FS, and returns plain structs. That keeps it
// testable on its own and lets callers decide how to render the result.
//
// The reference tables it loads are transcribed from 29 CFR 1910.1200
// Appendices B and C. They carry legal and safety weight and must be reviewed
// by a qualified person before production use. Every lookup failure is
// reported rather than silently skipped, because a hazard quietly dropped from
// a safety data sheet is the worst outcome this package can produce.
package ghs

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Signal words, in increasing severity. Danger displaces Warning.
const (
	SignalWarning = "Warning"
	SignalDanger  = "Danger"
)

// Statement types, in the order Appendix C presents them on a label.
var statementOrder = map[string]int{
	"prevention": 0,
	"response":   1,
	"storage":    2,
	"disposal":   3,
}

// MaxPrecautionaryStatements is the guidance figure from Appendix C: normally
// no more than six precautionary statements should appear. Exceeding it
// produces a warning, never a truncation -- silently dropping safety
// statements would be the wrong failure.
const MaxPrecautionaryStatements = 6

// HazardStatement is one row of the H-code table.
type HazardStatement struct {
	Code       string `yaml:"code"`
	Class      string `yaml:"class"`
	Category   string `yaml:"category"`
	Statement  string `yaml:"statement"`
	SignalWord string `yaml:"signal_word"`
	Pictogram  string `yaml:"pictogram"`
}

// PrecautionaryStatement is one row of the P-code table.
type PrecautionaryStatement struct {
	Code      string `yaml:"code"`
	Type      string `yaml:"type"`
	Statement string `yaml:"statement"`

	// SupplierSpecified marks a statement whose regulatory text contains
	// manufacturer-chosen wording. The generic text is emitted unless the
	// document supplies a replacement.
	SupplierSpecified bool `yaml:"supplier_specified"`
}

// Pictogram is a GHS pictogram code, its name, and its artwork.
type Pictogram struct {
	Code string `yaml:"code"`
	Name string `yaml:"name"`

	// Image names the artwork file relative to the ghs/ directory. Optional:
	// without it the pictogram still renders as text.
	Image string `yaml:"image"`

	// Data is the artwork itself, read at load time so callers can embed it
	// without needing the filesystem again. A generated sheet must stand alone
	// once written.
	Data []byte `yaml:"-"`

	// MediaType is the MIME type for Data, e.g. "image/svg+xml".
	MediaType string `yaml:"-"`
}

// HasImage reports whether artwork was loaded for this pictogram.
func (p Pictogram) HasImage() bool { return len(p.Data) > 0 }

// DataURI returns the artwork as a data: URI for embedding in a document.
func (p Pictogram) DataURI() string {
	if !p.HasImage() {
		return ""
	}
	return "data:" + p.MediaType + ";base64," + base64.StdEncoding.EncodeToString(p.Data)
}

// Tables is the loaded GHS reference data.
type Tables struct {
	hazards     map[string]HazardStatement
	precautions map[string]PrecautionaryStatement
	pictograms  map[string]Pictogram
	assignments map[string][]string
}

// LoadTables reads the reference data from a content library.
//
// Because it takes an fs.FS, a user's custom library layer can shadow any of
// the three files without recompiling -- the same mechanism the section
// content uses.
func LoadTables(fsys fs.FS) (*Tables, error) {
	var hazardDoc struct {
		Pictograms []Pictogram       `yaml:"pictograms"`
		Statements []HazardStatement `yaml:"statements"`
	}
	if err := readYAML(fsys, "ghs/hazard_statements.yaml", &hazardDoc); err != nil {
		return nil, err
	}

	var precautionaryDoc struct {
		Statements []PrecautionaryStatement `yaml:"statements"`
	}
	if err := readYAML(fsys, "ghs/precautionary.yaml", &precautionaryDoc); err != nil {
		return nil, err
	}

	var assignmentDoc struct {
		Assignments []struct {
			Hazard        string   `yaml:"hazard"`
			Precautionary []string `yaml:"precautionary"`
		} `yaml:"assignments"`
	}
	if err := readYAML(fsys, "ghs/assignments.yaml", &assignmentDoc); err != nil {
		return nil, err
	}

	t := &Tables{
		hazards:     make(map[string]HazardStatement, len(hazardDoc.Statements)),
		precautions: make(map[string]PrecautionaryStatement, len(precautionaryDoc.Statements)),
		pictograms:  make(map[string]Pictogram, len(hazardDoc.Pictograms)),
		assignments: make(map[string][]string, len(assignmentDoc.Assignments)),
	}
	for _, p := range hazardDoc.Pictograms {
		if p.Image != "" {
			data, mediaType, err := readImage(fsys, p.Image)
			if err != nil {
				return nil, fmt.Errorf("pictogram %s: %w", p.Code, err)
			}
			p.Data, p.MediaType = data, mediaType
		}
		t.pictograms[p.Code] = p
	}
	for _, h := range hazardDoc.Statements {
		t.hazards[h.Code] = h
	}
	for _, p := range precautionaryDoc.Statements {
		t.precautions[p.Code] = p
	}
	for _, a := range assignmentDoc.Assignments {
		t.assignments[a.Hazard] = a.Precautionary
	}

	if err := t.validate(); err != nil {
		return nil, err
	}
	return t, nil
}

// readImage loads pictogram artwork and infers its media type from the
// extension. An unreadable file named by the table is an error, not a silent
// fallback to text: a pictogram that vanishes from a sheet without warning is
// exactly the failure this package exists to prevent.
func readImage(fsys fs.FS, name string) ([]byte, string, error) {
	p := path.Join("ghs", name)
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, "", fmt.Errorf("reading artwork %s: %w", p, err)
	}

	mediaType, ok := mediaTypes[strings.ToLower(path.Ext(name))]
	if !ok {
		return nil, "", fmt.Errorf("artwork %s has unsupported extension %q", p, path.Ext(name))
	}
	return data, mediaType, nil
}

var mediaTypes = map[string]string{
	".svg":  "image/svg+xml",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
}

func readYAML(fsys fs.FS, name string, dst any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("reading GHS table %s: %w", name, err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("parsing GHS table %s: %w", name, err)
	}
	return nil
}

// validate cross-references the three tables. A dangling reference means a
// hazard would silently lose its precautionary statements.
func (t *Tables) validate() error {
	var problems []error

	for code := range t.hazards {
		if _, ok := t.assignments[code]; !ok {
			problems = append(problems, fmt.Errorf(
				"hazard %s has no precautionary assignment", code))
		}
	}
	for _, code := range sortedKeys(t.assignments) {
		if _, ok := t.hazards[code]; !ok {
			problems = append(problems, fmt.Errorf(
				"assignments name unknown hazard %s", code))
		}
		for _, p := range t.assignments[code] {
			if _, ok := t.precautions[p]; !ok {
				problems = append(problems, fmt.Errorf(
					"hazard %s assigns unknown precautionary statement %s", code, p))
			}
		}
	}
	for _, code := range sortedKeys(t.hazards) {
		if pic := t.hazards[code].Pictogram; pic != "" {
			if _, ok := t.pictograms[pic]; !ok {
				problems = append(problems, fmt.Errorf(
					"hazard %s names unknown pictogram %s", code, pic))
			}
		}
		switch t.hazards[code].SignalWord {
		case "", SignalWarning, SignalDanger:
		default:
			problems = append(problems, fmt.Errorf(
				"hazard %s has invalid signal word %q", code, t.hazards[code].SignalWord))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("GHS reference tables are inconsistent:\n%w", errors.Join(problems...))
	}
	return nil
}

// Codes returns every hazard code in the table, sorted.
func (t *Tables) Codes() []string { return sortedKeys(t.hazards) }

// Lookup returns one hazard statement.
func (t *Tables) Lookup(code string) (HazardStatement, bool) {
	h, ok := t.hazards[NormalizeCode(code)]
	return h, ok
}

// PrecautionaryCodes returns every precautionary code in the table, sorted.
func (t *Tables) PrecautionaryCodes() []string { return sortedKeys(t.precautions) }

// LookupPrecautionary returns one precautionary statement.
//
// Unlike Lookup, the code is taken as written: P-codes carry combined forms
// such as "P301+P312" that NormalizeCode's H-code rules would mangle.
func (t *Tables) LookupPrecautionary(code string) (PrecautionaryStatement, bool) {
	p, ok := t.precautions[strings.ToUpper(strings.TrimSpace(code))]
	return p, ok
}

// ErrUnknownCode is returned when a document names a hazard code that is not in
// the reference table.
var ErrUnknownCode = errors.New("unknown hazard code")

// NormalizeCode canonicalises a hazard code.
//
// YAML parses a bare 315 as an integer and people write h315 as often as H315,
// so all three forms are accepted and unified. An unrecognisable value is
// returned unchanged, so Classify can report it by the name the user wrote.
func NormalizeCode(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if _, err := strconv.Atoi(s); err == nil {
		return "H" + s
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // map order is random; sort for stable output
	return keys
}
