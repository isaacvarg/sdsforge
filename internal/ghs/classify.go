package ghs

import (
	"fmt"
	"sort"
	"strings"
)

// Classification is the computed GHS result for one product.
type Classification struct {
	// Codes are the normalised hazard codes that produced this result.
	Codes []string

	Hazards     []HazardStatement
	SignalWord  string
	Pictograms  []Pictogram
	Precautions []Precaution

	// Warnings are advisory notes for the person preparing the sheet. They
	// never change the output; they ask a human to look at it.
	Warnings []string
}

// Precaution is one selected precautionary statement.
type Precaution struct {
	Code      string
	Type      string
	Statement string

	// SupplierSpecified reports that the regulatory text contains
	// manufacturer-chosen wording and this statement is still generic.
	SupplierSpecified bool

	// TriggeredBy lists the hazard codes that assigned this statement. Several
	// hazards commonly assign the same one; it is emitted once and credited to
	// all of them.
	TriggeredBy []string
}

// IsClassified reports whether any hazard was recognised.
func (c *Classification) IsClassified() bool { return len(c.Hazards) > 0 }

// Classify computes the classification for a set of hazard codes.
//
// Codes are normalised, deduplicated and sorted, so the same input always
// produces byte-identical output. An unrecognised code is an error naming it:
// silently dropping a hazard from a safety data sheet is never acceptable.
func (t *Tables) Classify(codes []string) (*Classification, error) {
	seen := make(map[string]bool, len(codes))
	var (
		normalised []string
		unknown    []string
	)
	for _, raw := range codes {
		code := NormalizeCode(raw)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		if _, ok := t.hazards[code]; !ok {
			unknown = append(unknown, code)
			continue
		}
		normalised = append(normalised, code)
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%w: %s (the reference table holds %d codes, H200 through H420)",
			ErrUnknownCode, strings.Join(unknown, ", "), len(t.hazards))
	}

	sort.Strings(normalised)

	c := &Classification{Codes: normalised}
	if len(normalised) == 0 {
		return c, nil
	}

	// Hazard statements, in code order.
	pictogramSet := map[string]bool{}
	for _, code := range normalised {
		h := t.hazards[code]
		c.Hazards = append(c.Hazards, h)

		// Danger displaces Warning; a hazard carrying neither leaves it alone.
		if h.SignalWord == SignalDanger {
			c.SignalWord = SignalDanger
		} else if h.SignalWord == SignalWarning && c.SignalWord == "" {
			c.SignalWord = SignalWarning
		}

		if h.Pictogram != "" {
			pictogramSet[h.Pictogram] = true
		}
	}

	// Pictograms, deduplicated -- several hazards commonly share one.
	for _, code := range sortedKeys(pictogramSet) {
		c.Pictograms = append(c.Pictograms, t.pictograms[code])
	}

	c.Precautions = t.selectPrecautions(normalised)
	c.Warnings = warnings(c)

	return c, nil
}

// selectPrecautions gathers the assigned statements, deduplicates them, and
// orders them prevention, response, storage, disposal as Appendix C presents
// them. Combination statements (P305+P351+P338) are single entries throughout
// and are never split apart.
func (t *Tables) selectPrecautions(codes []string) []Precaution {
	byCode := map[string]*Precaution{}
	var order []string

	for _, hazard := range codes {
		for _, p := range t.assignments[hazard] {
			if existing, ok := byCode[p]; ok {
				existing.TriggeredBy = append(existing.TriggeredBy, hazard)
				continue
			}
			stmt := t.precautions[p]
			byCode[p] = &Precaution{
				Code:              p,
				Type:              stmt.Type,
				Statement:         stmt.Statement,
				SupplierSpecified: stmt.SupplierSpecified,
				TriggeredBy:       []string{hazard},
			}
			order = append(order, p)
		}
	}

	out := make([]Precaution, 0, len(order))
	for _, code := range order {
		out = append(out, *byCode[code])
	}

	// Stable sort: statement type first, then code, so the same hazard set
	// always yields the same label.
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := statementOrder[out[i].Type], statementOrder[out[j].Type]; a != b {
			return a < b
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// warnings collects advisory notes for the person preparing the sheet.
func warnings(c *Classification) []string {
	var out []string

	if n := len(c.Precautions); n > MaxPrecautionaryStatements {
		out = append(out, fmt.Sprintf(
			"%d precautionary statements selected; 29 CFR 1910.1200 App C advises normally "+
				"no more than %d. Review and consolidate rather than relying on this tool to choose.",
			n, MaxPrecautionaryStatements))
	}

	var generic []string
	for _, p := range c.Precautions {
		if p.SupplierSpecified {
			generic = append(generic, p.Code)
		}
	}
	if len(generic) > 0 {
		out = append(out, fmt.Sprintf(
			"%s carry manufacturer-specified wording and are still generic; "+
				"supply concrete text via precautionary_text in the document.",
			strings.Join(generic, ", ")))
	}

	if c.SignalWord == "" && len(c.Hazards) > 0 {
		out = append(out, "No signal word applies to these hazards; verify that is correct.")
	}

	return out
}

// ApplyText replaces the generic wording of supplier-specified statements with
// text the document supplies, keyed by P-code.
//
// An override for a code that was not selected is an error: it means the
// document is carrying wording for a hazard it no longer declares, which is
// exactly the sort of stale text that should not reach a sheet.
func (c *Classification) ApplyText(overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}

	index := make(map[string]int, len(c.Precautions))
	for i, p := range c.Precautions {
		index[p.Code] = i
	}

	var unused []string
	for _, code := range sortedKeys(overrides) {
		i, ok := index[code]
		if !ok {
			unused = append(unused, code)
			continue
		}
		c.Precautions[i].Statement = overrides[code]
		c.Precautions[i].SupplierSpecified = false
	}

	if len(unused) > 0 {
		return fmt.Errorf("precautionary_text names statements that this classification "+
			"does not select: %s", strings.Join(unused, ", "))
	}

	// Recompute: overrides may have cleared the generic-wording warning.
	c.Warnings = warnings(c)
	return nil
}
