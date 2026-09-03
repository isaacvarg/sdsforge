package document

import (
	"sort"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/sections"
)

// usStateNames maps a lowercase USPS postal code to its full name, for
// titling each state's Right-to-Know table. There is no such list anywhere
// else in the codebase.
var usStateNames = map[string]string{
	"al": "Alabama", "ak": "Alaska", "az": "Arizona", "ar": "Arkansas",
	"ca": "California", "co": "Colorado", "ct": "Connecticut",
	"de": "Delaware", "dc": "District of Columbia", "fl": "Florida",
	"ga": "Georgia", "hi": "Hawaii", "id": "Idaho", "il": "Illinois",
	"in": "Indiana", "ia": "Iowa", "ks": "Kansas", "ky": "Kentucky",
	"la": "Louisiana", "me": "Maine", "md": "Maryland",
	"ma": "Massachusetts", "mi": "Michigan", "mn": "Minnesota",
	"ms": "Mississippi", "mo": "Missouri", "mt": "Montana",
	"ne": "Nebraska", "nv": "Nevada", "nh": "New Hampshire",
	"nj": "New Jersey", "nm": "New Mexico", "ny": "New York",
	"nc": "North Carolina", "nd": "North Dakota", "oh": "Ohio",
	"ok": "Oklahoma", "or": "Oregon", "pa": "Pennsylvania",
	"ri": "Rhode Island", "sc": "South Carolina", "sd": "South Dakota",
	"tn": "Tennessee", "tx": "Texas", "ut": "Utah", "vt": "Vermont",
	"va": "Virginia", "wa": "Washington", "wv": "West Virginia",
	"wi": "Wisconsin", "wy": "Wyoming",
}

// rightToKnowBlock builds Section 15's state Right-to-Know subsection: one
// table per state with at least one chemical flagged true, titled with the
// state's full name. Returns nil when no state ends up with any
// disclosures, so SourceData omits it and the subsection's empty_text
// fallback renders -- the same rule prop65Block follows.
func rightToKnowBlock(entries []RightToKnowEntry) sections.Content {
	chemicalsByState := map[string][][]string{}

	for _, e := range entries {
		chemical := strings.TrimSpace(e.Chemical)
		if chemical == "" {
			continue
		}
		for code, triggered := range e.States {
			if !triggered {
				continue
			}
			code = strings.ToLower(strings.TrimSpace(code))
			if _, known := usStateNames[code]; !known {
				continue
			}
			chemicalsByState[code] = append(chemicalsByState[code], []string{chemical, e.CASNumber})
		}
	}

	if len(chemicalsByState) == 0 {
		return nil
	}

	codes := make([]string, 0, len(chemicalsByState))
	for code := range chemicalsByState {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool {
		return usStateNames[codes[i]] < usStateNames[codes[j]]
	})

	tables := make([]sections.NamedTable, 0, len(codes))
	for _, code := range codes {
		tables = append(tables, sections.NamedTable{
			Title:   usStateNames[code] + " Right to Know",
			Headers: []string{"Chemical name", "CAS No."},
			Rows:    chemicalsByState[code],
		})
	}

	return &sections.Tables{Tables: tables}
}
