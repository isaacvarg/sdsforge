package generation

import (
	"time"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/sections"
	"gopkg.in/yaml.v3"
)

// fixtureVersions is a two-entry version history. It is what puts the version
// number in the header and the revision rows in section 16, so most render
// tests want it; pass a zero VersionIndex to test a document with no versions.
func fixtureVersions() document.VersionIndex {
	return document.VersionIndex{
		NextID: 3,
		Versions: []document.Version{
			{
				ID:        1,
				Label:     "1.0.0",
				Timestamp: time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC),
				Memo:      "Authored document",
			},
			{
				ID:        2,
				Label:     "1.2.0",
				Timestamp: time.Date(2026, 8, 14, 11, 30, 0, 0, time.UTC),
				Memo:      "Reclassified as corrosive",
			},
		},
	}
}

// blankBlock decodes an empty prose block, used to blank a subsection.
func blankBlock(dst *sections.Block) error {
	return yaml.Unmarshal([]byte(`[]`), dst)
}
