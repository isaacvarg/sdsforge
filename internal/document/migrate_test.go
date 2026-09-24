package document

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// seedLegacy writes a document the way the old numeric store did, with a
// versions.yaml whose first entry is the authoring.
func seedLegacy(t *testing.T, number, name string, authored time.Time) {
	t.Helper()

	directory, err := DocumentsDir()
	if err != nil {
		t.Fatalf("DocumentsDir() error = %v", err)
	}
	dir := filepath.Join(directory, number)
	if err := os.MkdirAll(filepath.Join(dir, versionsDir), 0o700); err != nil {
		t.Fatalf("creating %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, DocumentFile),
		[]byte("product_name: "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	index := VersionIndex{NextID: 2}
	ver := index.Draft(InitialVersionLabel, InitialVersionMemo, authored)
	ver.Artifacts = []string{DocumentFile}
	index.Versions = append(index.Versions, ver)
	if err := saveVersions(ID(number), index); err != nil {
		t.Fatalf("saveVersions() error = %v", err)
	}
}

func legacyStore(t *testing.T) {
	t.Helper()
	isolate(t)

	seedLegacy(t, "1", "Suspension Shower Gel", time.Date(2026, 9, 2, 21, 23, 58, 0, time.UTC))
	seedLegacy(t, "2", "Hyaluronic Acid Gel Cream", time.Date(2026, 9, 14, 22, 2, 19, 0, time.UTC))
	seedLegacy(t, "10", "Suspension Oil Base", time.Date(2026, 9, 24, 18, 35, 14, 0, time.UTC))

	directory, err := DocumentsDir()
	if err != nil {
		t.Fatalf("DocumentsDir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, legacyIndexFile),
		[]byte("next_id: 11\nlast_modified_id: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The new ids carry each document's ORIGINAL authoring time, so the store goes
// on sorting into the order the documents were really written in -- which is
// not the order "1", "10", "2" that the directory lists them in.
func TestPlanMigrationKeepsChronology(t *testing.T) {
	legacyStore(t)

	renames, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if len(renames) != 3 {
		t.Fatalf("PlanMigration() returned %d renames, want 3", len(renames))
	}

	wantOrder := []string{"1", "2", "10"}
	for i, want := range wantOrder {
		if renames[i].From != want {
			t.Errorf("rename %d is %q, want %q", i, renames[i].From, want)
		}
	}
	if renames[0].Name != "Suspension Shower Gel" {
		t.Errorf("name = %q", renames[0].Name)
	}
	for i := 1; i < len(renames); i++ {
		if !(renames[i-1].To < renames[i].To) {
			t.Errorf("id %s should sort before %s", renames[i-1].To, renames[i].To)
		}
	}
	if got := renames[0].To.Time(); !got.Equal(renames[0].At) {
		t.Errorf("id carries %s, want the authoring time %s", got, renames[0].At)
	}

	// PlanMigration decides; it does not act.
	if _, err := os.Stat(filepath.Join(mustDocumentsDir(t), "1")); err != nil {
		t.Errorf("PlanMigration() disturbed the store: %v", err)
	}
}

func TestMigrate(t *testing.T) {
	legacyStore(t)

	renames, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if err := Migrate(renames); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	entries, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("List() returned %d documents, want 3", len(entries))
	}
	if entries[0].Name != "Suspension Shower Gel" || entries[2].Name != "Suspension Oil Base" {
		t.Errorf("List() = %v, want them in authoring order", entries)
	}

	// The version history moved with the document.
	versions, err := LoadVersions(entries[0].ID)
	if err != nil {
		t.Fatalf("LoadVersions() error = %v", err)
	}
	if len(versions.Versions) != 1 || versions.Versions[0].Label != InitialVersionLabel {
		t.Errorf("versions = %+v, want the authoring version", versions.Versions)
	}

	// The file that handed out the numbers, and caused the conflicts, is gone.
	if stale, err := HasLegacyIndex(); err != nil || stale {
		t.Errorf("HasLegacyIndex() = %v, %v; want false, nil", stale, err)
	}

	// The documents are reachable by name and by id prefix.
	got, err := Resolve("suspension-shower-gel")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != entries[0].ID {
		t.Errorf("Resolve() = %s, want %s", got, entries[0].ID)
	}
}

// Running it twice must be safe: people will, and one of them will be resolving
// a pull at the time.
func TestMigrateIsIdempotent(t *testing.T) {
	legacyStore(t)

	renames, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if err := Migrate(renames); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	before, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	again, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second PlanMigration() = %v, want nothing to do", again)
	}
	if err := Migrate(again); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	after, err := List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("documents = %d, want %d", len(after), len(before))
	}
	for i := range after {
		if after[i].ID != before[i].ID {
			t.Errorf("document %d changed id from %s to %s", i, before[i].ID, after[i].ID)
		}
	}
}

// A document with no versions.yaml -- created before versioning, or with the
// file lost -- still migrates rather than stopping the whole store.
func TestPlanMigrationWithoutVersionHistory(t *testing.T) {
	isolate(t)

	directory := mustDocumentsDir(t)
	dir := filepath.Join(directory, "4")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, DocumentFile),
		[]byte("product_name: Lye\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	renames, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if len(renames) != 1 || renames[0].From != "4" {
		t.Fatalf("PlanMigration() = %v, want the one document", renames)
	}
	if renames[0].At.IsZero() {
		t.Error("no authoring time was worked out")
	}
}

// Whatever else is in the store is not a document and must not be renamed.
func TestPlanMigrationSkipsNonDocuments(t *testing.T) {
	isolate(t)

	directory := mustDocumentsDir(t)
	if err := os.MkdirAll(filepath.Join(directory, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	renames, err := PlanMigration()
	if err != nil {
		t.Fatalf("PlanMigration() error = %v", err)
	}
	if len(renames) != 0 {
		t.Errorf("PlanMigration() = %v, want nothing", renames)
	}
}

func mustDocumentsDir(t *testing.T) string {
	t.Helper()
	directory, err := DocumentsDir()
	if err != nil {
		t.Fatalf("DocumentsDir() error = %v", err)
	}
	return directory
}
