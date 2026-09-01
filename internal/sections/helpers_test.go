package sections

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// yamlInto is a tiny test helper for decoding a YAML fragment.
func yamlInto(src string, dst any) error {
	return yaml.Unmarshal([]byte(src), dst)
}

// writeVariant creates a variant file inside a custom-library directory.
func writeVariant(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, "osha", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
