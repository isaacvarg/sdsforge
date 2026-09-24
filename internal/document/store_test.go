package document

import (
	"os"
	"path/filepath"
	"testing"
)

// Authoring a document is itself version 1.0.0, and that version archives the
// document.yaml and nothing else: rendering needs a headless browser, and
// 'document create' must not depend on one.
func TestCreateRecordsInitialVersion(t *testing.T) {
	isolate(t)

	content := []byte("product_name: Acetone\n# an authored comment\n")
	id, path, err := Create(content)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := ParseID(string(id)); err != nil {
		t.Fatalf("Create() returned id %q: %v", id, err)
	}

	live, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the live document: %v", err)
	}
	if string(live) != string(content) {
		t.Errorf("live document = %q, want the content as given", live)
	}

	index, err := LoadVersions(id)
	if err != nil {
		t.Fatalf("LoadVersions() error = %v", err)
	}
	if len(index.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(index.Versions))
	}

	ver := index.Versions[0]
	if ver.Label != InitialVersionLabel {
		t.Errorf("label = %q, want %q", ver.Label, InitialVersionLabel)
	}
	if ver.Memo != InitialVersionMemo {
		t.Errorf("memo = %q, want %q", ver.Memo, InitialVersionMemo)
	}

	// YAML only -- no browser was launched, so there is nothing else to archive.
	if len(ver.Artifacts) != 1 || ver.Artifacts[0] != DocumentFile {
		t.Errorf("artifacts = %q, want just %q", ver.Artifacts, DocumentFile)
	}

	dir, err := VersionDir(id, ver)
	if err != nil {
		t.Fatalf("VersionDir() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the snapshot directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("snapshot holds %d files, want only the document", len(entries))
	}

	// The snapshot is the authored text, comments and all, because a restore
	// has to give them back.
	archived, err := os.ReadFile(filepath.Join(dir, DocumentFile))
	if err != nil {
		t.Fatalf("reading the snapshot: %v", err)
	}
	if string(archived) != string(content) {
		t.Errorf("snapshot = %q, want the content as given", archived)
	}
}

// Each document keeps its own version history, so ids restart at 1.0.0 per
// document rather than continuing across them.
func TestCreateVersionsArePerDocument(t *testing.T) {
	isolate(t)

	first, _, err := Create([]byte("product_name: Acetone\n"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	second, _, err := Create([]byte("product_name: Lye\n"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if first == second {
		t.Fatalf("two documents share the id %s", first)
	}

	for _, id := range []ID{first, second} {
		index, err := LoadVersions(id)
		if err != nil {
			t.Fatalf("LoadVersions(%s) error = %v", id, err)
		}
		if len(index.Versions) != 1 || index.Versions[0].ID != 1 {
			t.Errorf("document %s versions = %+v, want one version at id 1", id, index.Versions)
		}
	}
}

func TestLoadRoundTrip(t *testing.T) {
	isolate(t)

	id, _, err := Create([]byte("product_name: Acetone\nhazard_codes: [H225]\n"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	doc, err := Load(id)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if doc.ProductName != "Acetone" {
		t.Errorf("product name = %q", doc.ProductName)
	}
	if codes := doc.AllHazardCodes(); len(codes) != 1 || codes[0] != "H225" {
		t.Errorf("hazard codes = %q", codes)
	}
}
