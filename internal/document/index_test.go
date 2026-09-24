package document

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// seed writes a document directly, at a chosen id, so a test can control the
// ordering and the names it resolves against.
func seed(t *testing.T, at time.Time, name string) ID {
	t.Helper()

	id, err := NewID(at)
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	dir, err := Dir(id)
	if err != nil {
		t.Fatalf("Dir() error = %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	body := "product_name: " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, DocumentFile), []byte(body), 0o644); err != nil {
		t.Fatalf("writing the document: %v", err)
	}
	return id
}

func TestListIsOldestFirst(t *testing.T) {
	isolate(t)

	newer := seed(t, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "Suspension Oil Base")
	older := seed(t, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), "Suspension Shower Gel")

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List() returned %d entries, want 2", len(entries))
	}
	if entries[0].ID != older || entries[1].ID != newer {
		t.Errorf("List() = %v, want the older document first", entries)
	}
	if entries[0].Name != "Suspension Shower Gel" {
		t.Errorf("name = %q", entries[0].Name)
	}
}

// The store is a git working tree. Whatever else is sitting in it is not a
// document and must not become one.
func TestListSkipsNonDocuments(t *testing.T) {
	isolate(t)

	want := seed(t, time.Now(), "Acetone")

	directory, err := DocumentsDir()
	if err != nil {
		t.Fatalf("DocumentsDir() error = %v", err)
	}
	// The index the store used to keep, a leftover numeric directory, and a
	// directory with an id but no document in it.
	if err := os.WriteFile(filepath.Join(directory, "index.yaml"), []byte("next_id: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "7"), 0o700); err != nil {
		t.Fatal(err)
	}
	empty, err := NewID(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, string(empty)), 0o700); err != nil {
		t.Fatal(err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != want {
		t.Errorf("List() = %v, want only %s", entries, want)
	}
}

func TestResolve(t *testing.T) {
	isolate(t)

	gel := seed(t, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), "Suspension Shower Gel")
	oil := seed(t, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "Suspension Oil Base")

	tests := []struct {
		name string
		ref  string
		want ID
	}{
		{"the id itself", string(gel), gel},
		{"the id lowercased", lower(string(gel)), gel},
		{"a leading piece of the id", string(oil)[:12], oil},
		{"the product name slugified", "suspension-shower-gel", gel},
		{"the product name as written", "Suspension Shower Gel", gel},
		{"the start of the product name", "suspension oil", oil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.ref)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v", tt.ref, err)
			}
			if got != tt.want {
				t.Errorf("Resolve(%q) = %s, want %s", tt.ref, got, tt.want)
			}
		})
	}
}

// Guessing between two documents could re-issue the wrong safety data sheet,
// so an ambiguous reference is refused -- and says what it was torn between.
func TestResolveRefusesAmbiguity(t *testing.T) {
	isolate(t)

	seed(t, time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), "Suspension Shower Gel")
	seed(t, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC), "Suspension Oil Base")

	_, err := Resolve("suspension")
	if err == nil {
		t.Fatal("Resolve() succeeded on an ambiguous reference")
	}
	if !strings.Contains(err.Error(), "Suspension Shower Gel") ||
		!strings.Contains(err.Error(), "Suspension Oil Base") {
		t.Errorf("error does not name both candidates:\n%s", err)
	}
}

func TestResolveNoMatch(t *testing.T) {
	isolate(t)
	seed(t, time.Now(), "Acetone")

	for _, ref := range []string{"lye", "01ZZZZ", ""} {
		if _, err := Resolve(ref); err == nil {
			t.Errorf("Resolve(%q) succeeded, want an error", ref)
		}
	}
}

// A numeric id is what every user of the old store has in their fingers. It
// must fail loudly rather than resolve to whatever happens to match.
func TestResolveRejectsLegacyNumericID(t *testing.T) {
	isolate(t)
	seed(t, time.Now(), "Acetone")

	if _, err := Resolve("1"); err == nil {
		t.Error("Resolve(\"1\") succeeded, want an error")
	}
}

// A document whose live document.yaml is gone is what 'document edit' tells the
// user to recover with 'document version restore'. They cannot, if the listing
// will not show it and the resolver will not find it.
func TestListIncludesADocumentAwaitingRestore(t *testing.T) {
	isolate(t)

	id := seed(t, time.Now(), "Acetone")
	dir, err := Dir(id)
	if err != nil {
		t.Fatal(err)
	}
	index := VersionIndex{NextID: 1}
	ver := index.Draft(InitialVersionLabel, InitialVersionMemo, time.Now())
	if err := CommitVersion(id, ver, index, map[string][]byte{
		DocumentFile: []byte("product_name: Acetone\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, DocumentFile)); err != nil {
		t.Fatal(err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("List() = %v, want %s", entries, id)
	}
	if entries[0].Display() != "(unnamed)" {
		t.Errorf("Display() = %q", entries[0].Display())
	}

	got, err := Resolve(string(id))
	if err != nil {
		t.Fatalf("Resolve(%s) error = %v", id, err)
	}
	if got != id {
		t.Errorf("Resolve() = %s, want %s", got, id)
	}
}

// A document that will not parse is still a document, and must stay reachable
// by id so it can be edited back into shape.
func TestListKeepsAnUnparseableDocument(t *testing.T) {
	isolate(t)

	id := seed(t, time.Now(), "Acetone")
	dir, err := Dir(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DocumentFile),
		[]byte("product_name: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 || entries[0].ID != id {
		t.Fatalf("List() = %v, want %s", entries, id)
	}
}
