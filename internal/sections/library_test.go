package sections

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLibraryBuiltinOnly(t *testing.T) {
	lib, err := NewLibrary(LibraryOptions{})
	if err != nil {
		t.Fatalf("NewLibrary() error = %v", err)
	}
	if got := lib.Jurisdiction(); got != "osha" {
		t.Errorf("Jurisdiction() = %q, want %q", got, "osha")
	}
	if got := lib.Layers(); !slices.Equal(got, []string{"built-in"}) {
		t.Errorf("Layers() = %v, want [built-in]", got)
	}
	if !lib.Exists("04_first_aid/section.yaml") {
		t.Error("embedded section.yaml not found")
	}
	if lib.Exists("99_nope/section.yaml") {
		t.Error("Exists() reported a nonexistent file")
	}
}

// The overlay must shadow a built-in file when enabled, and be completely
// ignored when disabled -- that toggle is the whole point of the config bool.
func TestLibraryCustomOverlay(t *testing.T) {
	// t.TempDir is cleaned up automatically when the test ends.
	root := t.TempDir()
	dir := filepath.Join(root, "osha", "04_first_aid", "inhalation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	custom := []byte("variant: default\npriority: 0\ncontent:\n  kind: prose\n  text:\n    - \"SITE-SPECIFIC INHALATION PROCEDURE\"\n")
	if err := os.WriteFile(filepath.Join(dir, "default.yaml"), custom, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("enabled shadows built-in", func(t *testing.T) {
		lib, err := NewLibrary(LibraryOptions{CustomVariants: true, CustomDir: root})
		if err != nil {
			t.Fatalf("NewLibrary() error = %v", err)
		}
		if got := lib.Layers(); !slices.Equal(got, []string{"custom", "built-in"}) {
			t.Fatalf("Layers() = %v, want [custom built-in]", got)
		}

		vf, err := LoadVariant(lib, "04_first_aid", "inhalation", "default")
		if err != nil {
			t.Fatalf("LoadVariant() error = %v", err)
		}
		if got := vf.Content.Body.(*Prose).Text[0]; got != "SITE-SPECIFIC INHALATION PROCEDURE" {
			t.Errorf("got built-in content %q; the custom layer should have shadowed it", got)
		}

		// A file the overlay does NOT carry still falls through to built-in.
		if _, err := LoadVariant(lib, "04_first_aid", "skin", "default"); err != nil {
			t.Errorf("fall-through to built-in failed: %v", err)
		}
	})

	t.Run("disabled ignores the overlay", func(t *testing.T) {
		lib, err := NewLibrary(LibraryOptions{CustomVariants: false, CustomDir: root})
		if err != nil {
			t.Fatalf("NewLibrary() error = %v", err)
		}
		if got := lib.Layers(); !slices.Equal(got, []string{"built-in"}) {
			t.Fatalf("Layers() = %v, want [built-in] when custom variants are off", got)
		}
		vf, err := LoadVariant(lib, "04_first_aid", "inhalation", "default")
		if err != nil {
			t.Fatalf("LoadVariant() error = %v", err)
		}
		if vf.Content.Body.(*Prose).Text[0] == "SITE-SPECIFIC INHALATION PROCEDURE" {
			t.Error("the custom layer was read even though CustomVariants was false")
		}
	})
}

func TestLibraryMissingCustomDir(t *testing.T) {
	_, err := NewLibrary(LibraryOptions{CustomVariants: true, CustomDir: "/nonexistent/path/xyz"})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error = %v, want one wrapping fs.ErrNotExist", err)
	}
}

func TestLibraryListVariants(t *testing.T) {
	lib, err := NewLibrary(LibraryOptions{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := lib.ListVariants("04_first_aid", "inhalation")
	if err != nil {
		t.Fatalf("ListVariants() error = %v", err)
	}
	want := []string{"acute_toxicity", "corrosive", "default"}
	if !slices.Equal(got, want) {
		t.Errorf("ListVariants() = %v, want %v", got, want)
	}

	presets, err := lib.ListPresets("04_first_aid")
	if err != nil {
		t.Fatalf("ListPresets() error = %v", err)
	}
	if !slices.Equal(presets, []string{"acute_inhalation", "corrosive"}) {
		t.Errorf("ListPresets() = %v", presets)
	}
}

func TestPredicateMatches(t *testing.T) {
	codes := map[string]bool{"H314": true, "H318": true}

	tests := []struct {
		name string
		pred Predicate
		want bool
	}{
		{"any_of hit", Predicate{AnyOf: []string{"H330", "H314"}}, true},
		{"any_of miss", Predicate{AnyOf: []string{"H330", "H331"}}, false},
		{"all_of hit", Predicate{AllOf: []string{"H314", "H318"}}, true},
		{"all_of miss", Predicate{AllOf: []string{"H314", "H225"}}, false},
		{"none_of blocks", Predicate{AnyOf: []string{"H314"}, NoneOf: []string{"H318"}}, false},
		{"none_of allows", Predicate{AnyOf: []string{"H314"}, NoneOf: []string{"H225"}}, true},
		{"empty never matches", Predicate{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pred.Matches(codes); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Every variant in the shipped library must load, declare content matching its
// subsection's kind, and -- where it carries a predicate -- not collide with a
// sibling on priority. This is the guard rail for authoring new content.
func TestBuiltinLibraryIsWellFormed(t *testing.T) {
	lib, err := NewLibrary(LibraryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateLibrary(lib); err != nil {
		t.Fatalf("built-in library is not well-formed:\n%v", err)
	}
}
