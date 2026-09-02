// Package document
package document

import (
	"fmt"
	"sort"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/ghs"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

type Data struct {
	ProductName string     `yaml:"product_name"`
	Materials   []Material `yaml:"materials,omitempty"`

	Identification Identification `yaml:"identification,omitempty"`

	// Deprecated: unread. Supplier details now come from the user's config
	// file, since they are the same on every sheet a company issues. The field
	// is kept so documents written before that change still load.
	Supplier Supplier `yaml:"supplier,omitempty"`

	// HazardCodes are the GHS codes for the product as a whole, e.g.
	// [H315, H350]. They drive section 2 entirely and select the wording in
	// every other section, replacing what used to be five repeated preset
	// selections.
	//
	// Codes may also be declared per material; the two sets are unioned.
	// Written as strings so "H315" and a bare 315 both work -- YAML parses the
	// latter as an integer, and people write both.
	HazardCodes []string `yaml:"hazard_codes,omitempty"`

	// PrecautionaryText supplies concrete wording for precautionary statements
	// whose regulatory text is manufacturer-specified, keyed by P-code:
	//
	//	precautionary_text:
	//	  P260: "Do not breathe mist or spray."
	PrecautionaryText map[string]string `yaml:"precautionary_text,omitempty"`

	// Sections holds this document's per-section choices, keyed by SECTION ID
	// ("first_aid"), never by number or directory name. That way renumbering a
	// section in the library never invalidates a saved document.
	//
	// A section absent from this map resolves entirely to its defaults, which
	// is why a minimal document.yaml can omit the key altogether.
	Sections map[string]sections.SectionSelection `yaml:"sections,omitempty"`
}

// AllHazardCodes returns every distinct hazard code for this document, taking
// the union of the product-level list and every material's own codes, each
// normalised so "H315", "h315" and a bare 315 collapse to one entry.
//
// Sorted, because it feeds both derivation and rendered output: the same
// document must produce the same sheet on every run.
func (d Data) AllHazardCodes() []string {
	seen := make(map[string]bool)
	add := func(codes []string) {
		for _, c := range codes {
			if n := ghs.NormalizeCode(c); n != "" {
				seen[n] = true
			}
		}
	}

	add(d.HazardCodes)
	for _, m := range d.Materials {
		add(m.HazardCodes)
	}

	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// HazardCodeSet is AllHazardCodes as a set, the shape the resolver's predicates
// evaluate against.
func (d Data) HazardCodeSet() map[string]bool {
	set := make(map[string]bool)
	for _, c := range d.AllHazardCodes() {
		set[c] = true
	}
	return set
}

type Material struct {
	Sequence   int    `yaml:"sequence"`
	Name       string `yaml:"name"`
	CASNumber  string `yaml:"cas_number"`
	Percentage string `yaml:"percentage"`
	// Deprecated: unused. Kept so existing documents still load; use
	// HazardCodes below.
	HazardsTriggered []int `yaml:"hazards_triggered,omitempty"`

	// HazardCodes are GHS codes such as "H314". These feed derivation later;
	// HazardsTriggered above is kept as-is so existing documents still load.
	HazardCodes []string `yaml:"hazard_codes"`
}

type Identification struct {
	ProductCodes []string `yaml:"product_codes,omitempty"`
	Synonyms     string   `yaml:"synonyms,omitempty"`
	CASNumber    string   `yaml:"cas_number,omitempty"`

	// RecommendedUse and Restrictions are free text describing what the
	// product is for. Section 1 requires both.
	RecommendedUse string `yaml:"recommended_use,omitempty"`
	Restrictions   string `yaml:"restrictions,omitempty"`
}

// Supplier identified who is responsible for the sheet.
//
// Deprecated: superseded by the [company] and [emergency] tables in the config
// file. Nothing reads this; HasLegacySupplier reports one so 'document
// generate' can say it is being ignored.
type Supplier struct {
	Name           string `yaml:"name,omitempty"`
	Address        string `yaml:"address,omitempty"`
	Phone          string `yaml:"phone,omitempty"`
	Email          string `yaml:"email,omitempty"`
	EmergencyPhone string `yaml:"emergency_phone,omitempty"`
}

// SourceData builds the content blocks for every subsection in the library
// that declares a `source:`.
//
// A source with no data is OMITTED rather than added as an empty block, so the
// library's authored placeholder survives. That is the difference between a
// sheet that says "No supplier details have been recorded" and one that shows
// an empty heading.
//
// Section 16's revision history comes from versions rather than from the
// document, so what the sheet claims was issued is what was actually archived.
// A caller recording a new version passes an index that already includes it.
func (d Data) SourceData(cls *ghs.Classification, cfg config.Config, versions VersionIndex) sections.SourceData {
	out := sections.SourceData{}

	// Section 2 is computed, not authored. A nil or unclassified result leaves
	// every block absent so the library's "Not classified" defaults survive.
	if cls != nil && cls.IsClassified() {
		out[sections.SourceClassification] = classificationTable(cls)
		if cls.SignalWord != "" {
			out[sections.SourceSignalWord] = &sections.Prose{Text: []string{cls.SignalWord}}
		}
		if block := pictogramBlock(cls); block != nil {
			out[sections.SourcePictograms] = block
		}
		if lines := precautionaryLines(cls); len(lines) > 0 {
			out[sections.SourcePrecautionary] = &sections.Prose{Text: lines}
		}
	}

	if rows := d.identificationLines(); len(rows) > 0 {
		out[sections.SourceIdentification] = &sections.Prose{Text: rows}
	}
	if lines := d.recommendedUseLines(); len(lines) > 0 {
		out[sections.SourceRecommendedUse] = &sections.Prose{Text: lines}
	}
	// Who issues the sheet and who to call about it are the same for every
	// document, so both come from configuration rather than from this file.
	if lines := cfg.Company.Lines(); len(lines) > 0 {
		out[sections.SourceSupplier] = &sections.Prose{Text: lines}
	}
	// The emergency numbers have their own subsection in section 1; listing
	// them under Supplier details as well would print them twice.
	if lines := cfg.Emergency.Lines(); len(lines) > 0 {
		out[sections.SourceEmergencyPhone] = &sections.Prose{Text: lines}
	}
	if len(d.Materials) > 0 {
		rows := make([][]string, 0, len(d.Materials))
		for _, m := range d.Materials {
			rows = append(rows, []string{m.Name, m.CASNumber, m.Percentage})
		}
		out[sections.SourceMaterials] = &sections.Table{
			Headers: []string{"Chemical name", "CAS No.", "Concentration (% w/w)"},
			Rows:    rows,
		}
	}
	if len(versions.Versions) > 0 {
		rows := make([][]string, 0, len(versions.Versions))
		for _, v := range versions.Versions {
			rows = append(rows, []string{v.Label, v.Timestamp.Format("2006-01-02"), v.Memo})
		}
		out[sections.SourceRevisions] = &sections.Table{
			Headers: []string{"Version", "Date", "Description"},
			Rows:    rows,
		}
	}

	return out
}

func (d Data) identificationLines() []string {
	var lines []string
	if d.ProductName != "" {
		lines = append(lines, "Product name: "+d.ProductName)
	}
	if len(d.Identification.ProductCodes) > 0 {
		lines = append(lines, "Product codes: "+strings.Join(d.Identification.ProductCodes, ", "))
	}
	if d.Identification.Synonyms != "" {
		lines = append(lines, "Synonyms: "+d.Identification.Synonyms)
	}
	if d.Identification.CASNumber != "" {
		lines = append(lines, "CAS No.: "+d.Identification.CASNumber)
	}
	return lines
}

func (d Data) recommendedUseLines() []string {
	var lines []string
	if d.Identification.RecommendedUse != "" {
		lines = append(lines, "Recommended use: "+d.Identification.RecommendedUse)
	}
	if d.Identification.Restrictions != "" {
		lines = append(lines, "Restrictions on use: "+d.Identification.Restrictions)
	}
	return lines
}

// HasLegacySupplier reports whether this document still carries supplier
// details of its own. They are no longer read, so a caller should say so
// rather than let a user wonder why what they typed does not appear.
func (d Data) HasLegacySupplier() bool {
	return d.Supplier != Supplier{}
}

type Index struct {
	NextID         int          `yaml:"next_id"`
	LastModifiedID int          `yaml:"last_modified_id"`
	Documents      []IndexEntry `yaml:"documents"`
}

type IndexEntry struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
}

// classificationTable renders section 2's GHS classification table.
func classificationTable(cls *ghs.Classification) *sections.Table {
	rows := make([][]string, 0, len(cls.Hazards))
	for _, h := range cls.Hazards {
		rows = append(rows, []string{
			h.Class,
			h.Category,
			h.Code + ": " + h.Statement + ".",
		})
	}
	return &sections.Table{
		Headers: []string{"Hazard class", "Category", "Hazard statement"},
		Rows:    rows,
	}
}

// pictogramBlock renders the hazard pictograms.
//
// Artwork is embedded as a data: URI rather than linked, because a safety data
// sheet gets emailed, printed and archived: it has to stand alone. Only the
// pictograms this product actually triggers are embedded, so a typical sheet
// carries two or three, not all nine.
//
// If the library has no artwork -- a custom layer may omit it -- this falls
// back to a text list rather than rendering nothing.
func pictogramBlock(cls *ghs.Classification) sections.Content {
	if len(cls.Pictograms) == 0 {
		return nil
	}

	images := make([]sections.Image, 0, len(cls.Pictograms))
	for _, p := range cls.Pictograms {
		if !p.HasImage() {
			continue
		}
		images = append(images, sections.Image{
			Src: p.DataURI(),
			// Alt and Caption both carry the code and name: alt is what a
			// screen reader announces and what survives when images are off,
			// and neither may be reduced to decoration on this document.
			Alt:     fmt.Sprintf("%s pictogram: %s", p.Code, p.Name),
			Caption: fmt.Sprintf("%s (%s)", p.Code, p.Name),
		})
	}

	if len(images) == 0 {
		lines := make([]string, 0, len(cls.Pictograms))
		for _, p := range cls.Pictograms {
			lines = append(lines, fmt.Sprintf("%s (%s)", p.Code, p.Name))
		}
		return &sections.Prose{Text: lines}
	}

	return &sections.Images{Images: images}
}

// precautionaryLines renders the selected statements in regulatory order.
func precautionaryLines(cls *ghs.Classification) []string {
	lines := make([]string, 0, len(cls.Precautions))
	for _, p := range cls.Precautions {
		lines = append(lines, p.Code+": "+p.Statement)
	}
	return lines
}
