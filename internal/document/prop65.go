package document

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

//go:embed assets/prop65_warning.svg
var prop65WarningSVG []byte

// prop65WarningDataURI embeds the warning symbol once, the same "compute
// on first use" pattern as ghs.Pictogram.DataURI, so it need not be
// re-encoded on every render.
var prop65WarningDataURI = sync.OnceValue(func() string {
	return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(prop65WarningSVG)
})

// prop65Block builds Section 15's Prop 65 subsection from the document's
// warnings, or returns nil when there is nothing to disclose. That mirrors
// SourceData's "omit rather than emit empty" rule elsewhere in this file, so
// the library's "no components known" default survives when a document
// carries no warnings.
func prop65Block(warnings []Prop65Warning) sections.Content {
	statement := prop65Statement(warnings)
	if statement == "" {
		return nil
	}

	return &sections.Images{
		Images: []sections.Image{
			{
				Src:     prop65WarningDataURI(),
				Alt:     "California Proposition 65 warning symbol",
				Caption: statement,
			},
		},
	}
}

// prop65Statement composes the warning required by Cal. Code Regs. tit. 27,
// section 25603(a)(2)(A)-(E), given the document's declared exposures.
//
// This is a best-effort transcription of the regulatory text -- it carries
// legal weight and should be reviewed by a qualified person before
// production use, the same caveat internal/ghs gives its hazard tables.
func prop65Statement(warnings []Prop65Warning) string {
	var carcinogens, repro []string
	seenCarc, seenRepro := map[string]bool{}, map[string]bool{}

	add := func(list *[]string, seen map[string]bool, name string) {
		if !seen[name] {
			seen[name] = true
			*list = append(*list, name)
		}
	}

	for _, w := range warnings {
		name := strings.TrimSpace(w.Chemical)
		if name == "" {
			continue
		}
		switch normalizeExposure(w.Exposure) {
		case "carcinogen":
			add(&carcinogens, seenCarc, name)
		case "reproductive_toxicant":
			add(&repro, seenRepro, name)
		case "both":
			add(&carcinogens, seenCarc, name)
			add(&repro, seenRepro, name)
		}
	}

	const moreInfo = "For more information go to www.P65Warnings.ca.gov."

	switch {
	case len(carcinogens) == 0 && len(repro) == 0:
		return ""

	case len(carcinogens) > 0 && len(repro) > 0 && sameNames(carcinogens, repro):
		// (D): a chemical (or set of chemicals) listed as both. (E) trims
		// "chemicals including" for a single chemical, same as (A) and (B).
		return fmt.Sprintf("This product can expose you to %s, which %s known to the State of California to cause cancer and birth defects or other reproductive harm. %s",
			singularPhrase(carcinogens), agree(carcinogens), moreInfo)

	case len(carcinogens) > 0 && len(repro) > 0:
		// (C): separate chemicals for each hazard type. Regulation text keeps
		// "chemicals including" on the first clause only, and does not list
		// (C) among the clauses (E) allows trimming for a single chemical.
		return fmt.Sprintf("This product can expose you to chemicals including %s, which %s known to the State of California to cause cancer, and %s, which %s known to the State of California to cause birth defects or other reproductive harm. %s",
			joinNames(carcinogens), agree(carcinogens), joinNames(repro), agree(repro), moreInfo)

	case len(carcinogens) > 0:
		// (A)
		return fmt.Sprintf("This product can expose you to %s, which %s known to the State of California to cause cancer. %s",
			singularPhrase(carcinogens), agree(carcinogens), moreInfo)

	default:
		// (B)
		return fmt.Sprintf("This product can expose you to %s, which %s known to the State of California to cause birth defects or other reproductive harm. %s",
			singularPhrase(repro), agree(repro), moreInfo)
	}
}

// ExposureValues lists every spelling normalizeExposure accepts, in the order
// an author is most likely to want them: the three documented values first,
// then the tolerated variants. The JSON Schema generator turns this into the
// enum for `prop65[].exposure`, so an editor offers the canonical spelling
// first while still accepting what the parser does.
//
// TestExposureValuesAllNormalize keeps this honest against the switch below.
func ExposureValues() []string {
	return []string{
		"carcinogen",
		"reproductive_toxicant",
		"both",
		"carcinogens",
		"reproductive toxicant",
		"reproductive_toxicants",
		"reproductive toxicants",
	}
}

// normalizeExposure accepts the documented enum plus a couple of spellings a
// document author might reasonably type, the same leniency ghs.NormalizeCode
// gives hazard codes. An unrecognized value returns "", which drops the
// entry rather than failing the whole document.
func normalizeExposure(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "carcinogen", "carcinogens":
		return "carcinogen"
	case "reproductive_toxicant", "reproductive toxicant", "reproductive_toxicants", "reproductive toxicants":
		return "reproductive_toxicant"
	case "both":
		return "both"
	default:
		return ""
	}
}

// singularPhrase applies section 25603(a)(2)(E): a single chemical drops the
// leading "chemicals including" rather than reading "chemicals including
// Acetone, which is known...".
func singularPhrase(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return "chemicals including " + joinNames(names)
}

// agree returns the verb matching a name list's count, e.g. "which is known"
// versus "which are known".
func agree(names []string) string {
	if len(names) == 1 {
		return "is"
	}
	return "are"
}

// joinNames renders a name list the way the regulation's example text does:
// "X", "X and Y", or "X, Y, and Z".
func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	default:
		return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
	}
}

// sameNames reports whether two name lists contain exactly the same
// chemicals, regardless of order -- the condition that turns separate
// carcinogen/reproductive-toxicant disclosures into a single (D) sentence.
func sameNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, n := range a {
		set[n] = true
	}
	for _, n := range b {
		if !set[n] {
			return false
		}
	}
	return true
}
