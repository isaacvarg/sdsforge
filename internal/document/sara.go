package document

import (
	"github.com/isaacvarg/sdsforge/internal/sections"
)

// saraHazardsBlock builds Section 15's SARA 311/312 hazard-category table,
// one row per entry in document order. A chemical with more than one
// hazard category simply appears in more than one row. Returns nil when
// there are no hazards to disclose, so SourceData omits it and the
// library's placeholder table survives.
func saraHazardsBlock(hazards []SARAHazard) sections.Content {
	if len(hazards) == 0 {
		return nil
	}

	rows := make([][]string, 0, len(hazards))
	for _, h := range hazards {
		rows = append(rows, []string{h.Chemical, h.CASNumber, h.Hazard})
	}

	return &sections.Table{
		Headers: []string{"Chemical name", "CAS No.", "Hazard category"},
		Rows:    rows,
	}
}
