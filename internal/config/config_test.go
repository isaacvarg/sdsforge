package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// configDir points the config lookup at a temporary directory. It sets
// XDG_CONFIG_HOME, which is what os.UserConfigDir reads on Linux -- so this
// also proves the XDG variable is honoured.
func configDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	dir := filepath.Join(root, "sdsforge")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

// write puts content at <config dir>/<name> and returns the config dir.
func write(t *testing.T, name, content string) string {
	t.Helper()
	dir := configDir(t)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPathIsXDG(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	got, err := Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if want := filepath.Join(root, "sdsforge", "config.toml"); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// No config file is the normal state for a new install, not an error.
func TestLoadMissingFile(t *testing.T) {
	configDir(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for a missing file", err)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Errorf("Load() = %+v, want the defaults %+v", cfg, Default())
	}
}

func TestLoadFull(t *testing.T) {
	write(t, "config.toml", `
[library]
jurisdiction    = "osha"
custom_variants = true
custom_dir      = "/srv/sds-content"

[company]
name    = "Acme Chemical Co."
address = "1 Industrial Way, Springfield IL"
phone   = "+1-555-0100"
email   = "sds@acme.example"

[[emergency.contacts]]
name  = "CHEMTREC (24 hr)"
phone = "1-800-424-9300"
note  = "USA"

[[emergency.contacts]]
phone = "+1-703-527-3887"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Library.CustomDir != "/srv/sds-content" || !cfg.Library.CustomVariants {
		t.Errorf("library = %+v", cfg.Library)
	}
	if cfg.Company.Name != "Acme Chemical Co." || cfg.Company.Email != "sds@acme.example" {
		t.Errorf("company = %+v", cfg.Company)
	}
	if len(cfg.Emergency.Contacts) != 2 {
		t.Fatalf("contacts = %+v, want 2", cfg.Emergency.Contacts)
	}
}

func TestLoadMalformed(t *testing.T) {
	write(t, "config.toml", "[company\nname = ")

	if _, err := Load(); err == nil {
		t.Error("Load() error = nil for a malformed file; a broken file must not read as defaults")
	}
}

// A named contact with no number is a typo. Dropping it quietly would leave a
// gap on a printed sheet.
func TestLoadRejectsContactWithoutPhone(t *testing.T) {
	write(t, "config.toml", `
[[emergency.contacts]]
name  = "CHEMTREC"
phone = "1-800-424-9300"

[[emergency.contacts]]
name = "Plant safety officer"
`)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil for a contact with no phone")
	}
	if !strings.Contains(err.Error(), "emergency.contacts[1]") {
		t.Errorf("error does not say which contact: %v", err)
	}
}

func TestLoadRejectsCustomVariantsWithoutDir(t *testing.T) {
	write(t, "config.toml", "[library]\ncustom_variants = true\n")

	if _, err := Load(); err == nil {
		t.Error("Load() error = nil when custom_variants is on with no custom_dir")
	}
}

// An omitted jurisdiction falls back rather than producing a library with no
// content root.
func TestLoadDefaultsJurisdiction(t *testing.T) {
	write(t, "config.toml", "[company]\nname = \"Acme\"\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Library.Jurisdiction != "osha" {
		t.Errorf("jurisdiction = %q, want the default", cfg.Library.Jurisdiction)
	}
}

// The config used to be YAML. Falling through to defaults would silently
// discard a working custom_dir, so the old file must be reported.
func TestLoadReportsLegacyYAML(t *testing.T) {
	write(t, "config.yaml", "jurisdiction: osha\ncustom_dir: /srv/sds-content\n")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil with only the old config.yaml present")
	}
	for _, want := range []string{"config.yaml", "config.toml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
}

func TestCompanyLines(t *testing.T) {
	c := Company{Name: "Acme Chemical Co.", Phone: "+1-555-0100"}

	want := []string{"Acme Chemical Co.", "Telephone: +1-555-0100"}
	if got := c.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
	if len((Company{}).Lines()) != 0 {
		t.Error("an unconfigured company must produce no lines, so the library placeholder survives")
	}
	if !(Company{}).IsZero() || (Company{Email: "x"}).IsZero() {
		t.Error("IsZero() is wrong")
	}
}

func TestEmergencyLines(t *testing.T) {
	e := Emergency{Contacts: []Contact{
		{Name: "CHEMTREC (24 hr)", Phone: "1-800-424-9300", Note: "USA"},
		{Phone: "+1-703-527-3887"},
		{Name: "Plant safety officer", Phone: "+1-555-0134"},
	}}

	want := []string{
		"CHEMTREC (24 hr): 1-800-424-9300 (USA)",
		"+1-703-527-3887",
		"Plant safety officer: +1-555-0134",
	}
	if got := e.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
	if len((Emergency{}).Lines()) != 0 {
		t.Error("no contacts must produce no lines, so the library default survives")
	}
}
