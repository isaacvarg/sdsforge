package document

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// isolate points the data directory at a temporary one, so tests never touch
// the user's real documents.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
}

func TestLoadVersionsMissingFile(t *testing.T) {
	isolate(t)

	// A document with no versions yet is not an error, the same way a missing
	// document index is not.
	got, err := LoadVersions(7)
	if err != nil {
		t.Fatalf("LoadVersions() error = %v", err)
	}
	if got.NextID != 1 || len(got.Versions) != 0 {
		t.Errorf("LoadVersions() on a fresh document = %+v, want an empty index at id 1", got)
	}
}

func TestCommitVersionRoundTrip(t *testing.T) {
	isolate(t)

	index, err := LoadVersions(1)
	if err != nil {
		t.Fatalf("LoadVersions() error = %v", err)
	}

	at := time.Date(2026, 9, 2, 14, 30, 15, 0, time.UTC)
	ver := index.Draft("1.0.0", "Authored document", at)

	if err := CommitVersion(1, ver, index, map[string][]byte{
		DocumentFile: []byte("product_name: Acetone\n"),
	}); err != nil {
		t.Fatalf("CommitVersion() error = %v", err)
	}

	got, err := LoadVersions(1)
	if err != nil {
		t.Fatalf("LoadVersions() error = %v", err)
	}
	if len(got.Versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(got.Versions))
	}
	if got.NextID != 2 {
		t.Errorf("NextID = %d, want 2", got.NextID)
	}

	stored := got.Versions[0]
	if stored.Label != "1.0.0" || stored.Memo != "Authored document" || stored.ID != 1 {
		t.Errorf("stored version = %+v", stored)
	}
	if !stored.Timestamp.Equal(at) {
		t.Errorf("timestamp = %v, want %v", stored.Timestamp, at)
	}
	if want := []string{DocumentFile}; !reflect.DeepEqual(stored.Artifacts, want) {
		t.Errorf("artifacts = %q, want %q", stored.Artifacts, want)
	}

	// The directory sorts by time and still says which version it holds.
	if want := "20260902T143015Z-1.0.0"; stored.Dir != want {
		t.Errorf("dir = %q, want %q", stored.Dir, want)
	}

	dir, err := VersionDir(1, stored)
	if err != nil {
		t.Fatalf("VersionDir() error = %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, DocumentFile))
	if err != nil {
		t.Fatalf("reading the snapshot: %v", err)
	}
	if string(content) != "product_name: Acetone\n" {
		t.Errorf("snapshot content = %q", content)
	}
}

func TestCommitVersionRecordsEveryArtifact(t *testing.T) {
	isolate(t)

	index, _ := LoadVersions(3)
	ver := index.Draft("1.1.0", "Added H314", time.Now())

	if err := CommitVersion(3, ver, index, map[string][]byte{
		DocumentFile: []byte("product_name: Lye\n"),
		"lye.html":   []byte("<html></html>"),
		"lye.pdf":    []byte("%PDF-1.4"),
	}); err != nil {
		t.Fatalf("CommitVersion() error = %v", err)
	}

	got, _ := LoadVersions(3)
	// Sorted, so the recorded list is stable rather than map-ordered.
	want := []string{DocumentFile, "lye.html", "lye.pdf"}
	if !reflect.DeepEqual(got.Versions[0].Artifacts, want) {
		t.Errorf("artifacts = %q, want %q", got.Versions[0].Artifacts, want)
	}

	dir, _ := VersionDir(3, got.Versions[0])
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not written: %v", name, err)
		}
	}
}

func TestDraftAllocatesSequentially(t *testing.T) {
	isolate(t)

	at := time.Date(2026, 9, 2, 14, 30, 15, 0, time.UTC)
	index := VersionIndex{NextID: 1}

	first := index.Draft("1.0.0", "one", at)
	index = index.WithPending(first)
	second := index.Draft("1.1.0", "two", at.Add(time.Hour))

	if first.ID != 1 || second.ID != 2 {
		t.Errorf("ids = %d, %d; want 1, 2", first.ID, second.ID)
	}
	if index.NextID != 2 {
		t.Errorf("NextID after one pending = %d, want 2", index.NextID)
	}
}

// Two versions issued in the same second must not share a directory, or the
// second would overwrite a sheet that has already gone out.
func TestDraftAvoidsDirectoryCollision(t *testing.T) {
	at := time.Date(2026, 9, 2, 14, 30, 15, 0, time.UTC)

	index := VersionIndex{NextID: 1}
	first := index.Draft("1.0.0", "one", at)
	index = index.WithPending(first)
	second := index.Draft("1.0.0", "two", at)

	if second.Dir == first.Dir {
		t.Fatalf("both versions want %q", first.Dir)
	}
	if want := first.Dir + "-2"; second.Dir != want {
		t.Errorf("second dir = %q, want %q", second.Dir, want)
	}
}

// The timestamp is always UTC: sheets are issued across time zones and the
// ordering has to be total.
func TestDraftNormalisesToUTC(t *testing.T) {
	zone := time.FixedZone("UTC+9", 9*60*60)
	at := time.Date(2026, 9, 2, 23, 30, 0, 0, zone)

	ver := VersionIndex{NextID: 1}.Draft("1.0.0", "", at)
	if ver.Timestamp.Location() != time.UTC {
		t.Errorf("timestamp zone = %v, want UTC", ver.Timestamp.Location())
	}
	if want := "20260902T143000Z-1.0.0"; ver.Dir != want {
		t.Errorf("dir = %q, want %q", ver.Dir, want)
	}
}

func TestFind(t *testing.T) {
	index := VersionIndex{
		NextID: 3,
		Versions: []Version{
			{ID: 1, Label: "1.0.0"},
			{ID: 2, Label: "1.1.0"},
		},
	}

	// A bare number is an id and anything else is a label; the two can never
	// collide, because labels always contain dots.
	tests := []struct {
		ref     string
		want    string
		wantErr bool
	}{
		{ref: "1.1.0", want: "1.1.0"},
		{ref: "2", want: "1.1.0"},
		{ref: "1", want: "1.0.0"},
		{ref: "9", wantErr: true},
		{ref: "2.0.0", wantErr: true},
		{ref: "nonsense", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, err := index.Find(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Find(%q) = %+v, want an error", tt.ref, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Find(%q) error = %v", tt.ref, err)
			}
			if got.Label != tt.want {
				t.Errorf("Find(%q) = %s, want %s", tt.ref, got.Label, tt.want)
			}
		})
	}
}

func TestLatest(t *testing.T) {
	if _, ok := (VersionIndex{NextID: 1}).Latest(); ok {
		t.Error("Latest() found a version in an empty index")
	}

	index := VersionIndex{Versions: []Version{{Label: "1.0.0"}, {Label: "1.1.0"}}}
	got, ok := index.Latest()
	if !ok || got.Label != "1.1.0" {
		t.Errorf("Latest() = %+v, %v; want 1.1.0", got, ok)
	}
}

// A label that is not path-safe must not escape into a directory name.
func TestSanitizeLabel(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1.0.0", "1.0.0"},
		{"1.0.0-rc1", "1.0.0-rc1"},
		{"../../etc", ".._.._etc"},
		{"a/b", "a_b"},
		{"", "unlabelled"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := sanitizeLabel(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeLabel(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if strings.ContainsRune(got, filepath.Separator) {
				t.Errorf("sanitizeLabel(%q) = %q, which is not one path segment", tt.in, got)
			}
		})
	}
}
