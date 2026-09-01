package generation

import (
	"github.com/isaacvarg/sdsforge/internal/sections"
	"gopkg.in/yaml.v3"
)

// blankBlock decodes an empty prose block, used to blank a subsection.
func blankBlock(dst *sections.Block) error {
	return yaml.Unmarshal([]byte(`[]`), dst)
}
