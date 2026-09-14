package document

import "github.com/isaacvarg/sdsforge/internal/sections"

// tscaInventoryBlock builds Section 15's optional TSCA inventory table, one
// row per entry in document order. Returns nil when there are no entries, so
// SourceData omits it and the library's placeholder row survives.
func tscaInventoryBlock(entries []TSCAInventoryEntry) sections.Content {
	if len(entries) == 0 {
		return nil
	}

	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Material, e.CASNumber, e.Basis})
	}

	return &sections.Table{
		Headers: []string{"Material", "CAS #", "Basis"},
		Rows:    rows,
	}
}
