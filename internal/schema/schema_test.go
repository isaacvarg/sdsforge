package schema_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/schema"
	"github.com/isaacvarg/sdsforge/internal/sections"
)

const committedPath = "../../docs/document.schema.json"

// builtinLibrary opens the embedded library ONLY.
//
// Deliberately not the user's configured library: a developer with
// custom_variants turned on would otherwise generate a schema carrying their own
// variants and fail this test on a clean checkout.
func builtinLibrary(t *testing.T) *sections.Library {
	t.Helper()
	lib, err := sections.NewLibrary(sections.LibraryOptions{})
	if err != nil {
		t.Fatalf("opening built-in library: %v", err)
	}
	return lib
}

func generate(t *testing.T) []byte {
	t.Helper()
	out, err := schema.Generate(builtinLibrary(t))
	if err != nil {
		t.Fatalf("generating schema: %v", err)
	}
	return out
}

// TestCommittedSchemaIsCurrent is the drift guard. docs/document.schema.json is
// what editors actually read, so a library change that does not reach it leaves
// people completing variants that no longer exist.
func TestCommittedSchemaIsCurrent(t *testing.T) {
	got := generate(t)

	want, err := os.ReadFile(filepath.FromSlash(committedPath))
	if err != nil {
		t.Fatalf("reading committed schema: %v\n\nregenerate it with:\n\n\tgo run . schema -o docs/document.schema.json\n", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("docs/document.schema.json is out of date.\n\nregenerate it with:\n\n\tgo run . schema -o docs/document.schema.json\n\n%s",
			firstDifference(want, got))
	}
}

// TestGenerateIsDeterministic proves the golden test above means something. Map
// iteration order is random in Go, so a single unsorted range in the generator
// would make the committed file flap between runs.
func TestGenerateIsDeterministic(t *testing.T) {
	first := generate(t)
	for i := range 5 {
		if next := generate(t); !bytes.Equal(first, next) {
			t.Fatalf("run %d differs from run 0:\n%s", i+1, firstDifference(first, next))
		}
	}
}

// TestEveryRefResolves catches a mistyped definition name, which JSON Schema
// itself would report only as a silently unvalidated subtree.
func TestEveryRefResolves(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(generate(t), &doc); err != nil {
		t.Fatalf("generated schema is not valid JSON: %v", err)
	}

	defs, _ := doc["$defs"].(map[string]any)
	if len(defs) == 0 {
		t.Fatal("generated schema declares no $defs")
	}

	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch v := node.(type) {
		case map[string]any:
			if raw, ok := v["$ref"]; ok {
				target, _ := raw.(string)
				name := strings.TrimPrefix(target, "#/$defs/")
				if name == target {
					t.Errorf("%s: $ref %q is not a local definition", path, target)
				} else if _, ok := defs[name]; !ok {
					t.Errorf("%s: $ref %q points at a definition that does not exist", path, target)
				}
			}
			for key, child := range v {
				walk(child, path+"/"+key)
			}
		case []any:
			for i, child := range v {
				walk(child, fmt.Sprintf("%s/%d", path, i))
			}
		}
	}
	walk(doc, "")
}

// TestSectionsAreEnumeratedExactly is the test that would fail if the schema
// quietly regressed to a flat shape: it checks that a subsection's variant enum
// carries only the files that exist for THAT subsection, not every variant name
// in the library.
func TestSectionsAreEnumeratedExactly(t *testing.T) {
	lib := builtinLibrary(t)

	var doc struct {
		Defs map[string]struct {
			Properties struct {
				Variant struct {
					Enum []string `json:"enum"`
				} `json:"variant"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(generate(t), &doc); err != nil {
		t.Fatalf("generated schema is not valid JSON: %v", err)
	}

	layout, err := sections.LoadLayout(lib)
	if err != nil {
		t.Fatalf("loading layout: %v", err)
	}

	checked := 0
	for _, dir := range layout.Sections {
		def, err := sections.LoadSection(lib, dir)
		if err != nil {
			t.Fatalf("loading section %s: %v", dir, err)
		}
		for _, sub := range def.Subsections {
			want, err := lib.ListVariants(def.Dir, sub.ID)
			if err != nil {
				t.Fatalf("listing variants for %s/%s: %v", def.Dir, sub.ID, err)
			}
			name := "sub_" + def.ID + "_" + sub.ID
			node, ok := doc.Defs[name]
			if !ok {
				t.Errorf("no definition %q for subsection %s.%s", name, def.ID, sub.ID)
				continue
			}
			if got := node.Properties.Variant.Enum; !equalStrings(got, want) {
				t.Errorf("%s.%s variant enum = %v, want %v", def.ID, sub.ID, got, want)
			}
			checked++
		}
	}

	if checked == 0 {
		t.Fatal("checked no subsections; the walk above is not doing anything")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// firstDifference reports where two versions of the schema diverge, with a few
// lines either side. A full diff would need a dependency; the first difference
// plus the regeneration command is enough to act on.
func firstDifference(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")

	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] == gotLines[i] {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "first difference at line %d:\n", i+1)
		for j := max(0, i-3); j < i; j++ {
			fmt.Fprintf(&b, "   %s\n", wantLines[j])
		}
		fmt.Fprintf(&b, "  -%s\n", wantLines[i])
		fmt.Fprintf(&b, "  +%s\n", gotLines[i])
		return b.String()
	}

	return fmt.Sprintf("identical for %d lines, then the committed file has %d lines and the generated one has %d",
		min(len(wantLines), len(gotLines)), len(wantLines), len(gotLines))
}

// TestSchemaCoversEveryDataField is the drift guard pointing the other way.
//
// TestCommittedSchemaIsCurrent catches a stale committed file; this catches a
// stale GENERATOR -- a field added to Data with no matching schema property.
// Without it, the failure is invisible: the new key simply reads as a typo in
// every editor, because the root is closed to what it does not declare.
func TestSchemaCoversEveryDataField(t *testing.T) {
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(generate(t), &doc); err != nil {
		t.Fatalf("generated schema is not valid JSON: %v", err)
	}

	defProps := func(name string) map[string]json.RawMessage {
		def, ok := doc.Defs[name]
		if !ok {
			t.Fatalf("no definition %q in the generated schema", name)
		}
		return def.Properties
	}

	for _, tc := range []struct {
		what  string
		value any
		props map[string]json.RawMessage
	}{
		{"document.Data", document.Data{}, doc.Properties},
		{"document.Material", document.Material{}, defProps("material")},
		{"document.Identification", document.Identification{}, defProps("identification")},
		{"document.Supplier", document.Supplier{}, defProps("supplier")},
		{"document.Prop65Warning", document.Prop65Warning{}, defProps("prop65_warning")},
		{"document.SARAHazard", document.SARAHazard{}, defProps("sara_hazard")},
		{"document.RightToKnowEntry", document.RightToKnowEntry{}, defProps("right_to_know_entry")},
		// SectionSelection and SubsectionOverride are checked against a section
		// that has presets and a subsection that has variants, since the
		// generator legitimately omits `variant` where the library offers none.
		{"sections.SectionSelection", sections.SectionSelection{}, defProps("section_first_aid")},
		{"sections.SubsectionOverride", sections.SubsectionOverride{}, defProps("sub_first_aid_skin")},
	} {
		named, inline := yamlFields(tc.value)

		for _, field := range named {
			if _, ok := tc.props[field]; !ok {
				t.Errorf("%s has yaml field %q with no schema property; the schema closes out anything it does not declare, so documents using it would be flagged", tc.what, field)
			}
		}

		// An inline map absorbs arbitrary sibling keys, so the reverse check
		// only applies where there isn't one.
		if inline {
			continue
		}
		for prop := range tc.props {
			if !slices.Contains(named, prop) {
				t.Errorf("%s: schema declares property %q that no yaml field matches", tc.what, prop)
			}
		}
	}

	// RightToKnowEntry's inline map is the state flags. They are the reason
	// that node is hand-built rather than reflected, so check them by name.
	rtk := defProps("right_to_know_entry")
	for _, code := range document.StateCodes() {
		if _, ok := rtk[code]; !ok {
			t.Errorf("right_to_know_entry is missing state %q", code)
		}
	}
}

// yamlFields returns the yaml key names a struct declares, and whether one of
// them is an inline map.
func yamlFields(v any) (named []string, inline bool) {
	rt := reflect.TypeOf(v)
	for i := range rt.NumField() {
		tag := rt.Field(i).Tag.Get("yaml")
		name, opts, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if strings.Contains(opts, "inline") {
			inline = true
			continue
		}
		if name == "" {
			name = strings.ToLower(rt.Field(i).Name)
		}
		named = append(named, name)
	}
	return named, inline
}
