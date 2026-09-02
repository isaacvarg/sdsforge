package document

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DocumentFile is the name of a document's source file, live and archived alike.
const DocumentFile = "document.yaml"

// versionsFile indexes one document's versions; versionsDir holds the snapshots.
const (
	versionsFile = "versions.yaml"
	versionsDir  = "versions"
)

// Version is one issued revision of a document.
//
// A version is immutable once written. Its directory holds the document.yaml as
// it was, plus the HTML and PDF that were generated from it, so a sheet that
// went out the door can always be produced again byte for byte rather than
// re-rendered against a content library that has moved on since.
type Version struct {
	ID    int    `yaml:"id"`
	Label string `yaml:"label"`
	// Timestamp is always UTC. Sheets are issued across time zones and the
	// ordering has to be total.
	Timestamp time.Time `yaml:"timestamp"`
	Memo      string    `yaml:"memo"`
	// Dir is the snapshot directory's name, relative to <document>/versions.
	Dir string `yaml:"dir"`
	// Artifacts names the files in that directory. The initial version recorded
	// by Create carries only the document.yaml -- see Create's comment.
	Artifacts []string `yaml:"artifacts"`
}

// Semver parses the version's label.
func (v Version) Semver() (Semver, error) {
	return ParseSemver(v.Label)
}

// VersionIndex is a document's version history, newest last.
type VersionIndex struct {
	NextID int `yaml:"next_id"`
	// Versions is in append order, which is chronological order.
	Versions []Version `yaml:"versions"`
}

// Latest returns the most recently recorded version.
func (v VersionIndex) Latest() (Version, bool) {
	if len(v.Versions) == 0 {
		return Version{}, false
	}
	return v.Versions[len(v.Versions)-1], true
}

// Find resolves a user-typed reference to one version.
//
// A bare number is an id and anything else is a label. The two can never
// collide: labels always contain dots, ids never do.
func (v VersionIndex) Find(ref string) (Version, error) {
	if id, err := strconv.Atoi(ref); err == nil {
		for _, ver := range v.Versions {
			if ver.ID == id {
				return ver, nil
			}
		}
		return Version{}, fmt.Errorf("no version with id %d", id)
	}

	for _, ver := range v.Versions {
		if ver.Label == ref {
			return ver, nil
		}
	}
	return Version{}, fmt.Errorf("no version labelled %q", ref)
}

// HasLabel reports whether the label is already taken.
func (v VersionIndex) HasLabel(label string) bool {
	for _, ver := range v.Versions {
		if ver.Label == label {
			return true
		}
	}
	return false
}

// Draft prepares the next version WITHOUT persisting anything.
//
// Drafting and committing are separate so that 'version create' can render the
// sheet against a history that already contains the version being recorded: the
// archived PDF then shows its own row in the revision history and its own number
// in the header. Nothing reaches disk until CommitVersion, so a failed render
// leaves no half-recorded version to clean up.
func (v VersionIndex) Draft(label, memo string, at time.Time) Version {
	id := v.NextID
	if id == 0 {
		id = 1
	}
	return Version{
		ID:    id,
		Label: label,
		// Truncated to the second: the snapshot directory names it to the
		// second and nothing displays finer, so sub-second digits would only
		// make the index noisier to read.
		Timestamp: at.UTC().Truncate(time.Second),
		Memo:      memo,
		Dir:       v.dirName(at.UTC(), label),
	}
}

// WithPending returns the index as it will look once ver is committed. It is the
// value to render against; see Draft.
func (v VersionIndex) WithPending(ver Version) VersionIndex {
	next := VersionIndex{
		NextID:   ver.ID + 1,
		Versions: make([]Version, 0, len(v.Versions)+1),
	}
	next.Versions = append(next.Versions, v.Versions...)
	next.Versions = append(next.Versions, ver)
	return next
}

// dirName builds a snapshot directory name: the UTC timestamp then the label, so
// the directory listing sorts chronologically and still says which version each
// entry is without opening the index.
func (v VersionIndex) dirName(at time.Time, label string) string {
	base := at.Format("20060102T150405Z") + "-" + sanitizeLabel(label)

	// Two versions in the same second is unlikely but not impossible, and a
	// silently reused directory would overwrite an issued sheet.
	taken := make(map[string]bool, len(v.Versions))
	for _, ver := range v.Versions {
		taken[ver.Dir] = true
	}
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := base + "-" + strconv.Itoa(n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// sanitizeLabel keeps a label safe as a path segment. The index stores the label
// verbatim; only the directory name is reduced.
func sanitizeLabel(label string) string {
	var b strings.Builder
	for _, r := range label {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "unlabelled"
	}
	return b.String()
}

// VersionsDir returns the directory holding one document's snapshots.
func VersionsDir(docID int) (string, error) {
	dir, err := Dir(docID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, versionsDir), nil
}

// VersionDir returns one snapshot's directory.
func VersionDir(docID int, ver Version) (string, error) {
	dir, err := VersionsDir(docID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ver.Dir), nil
}

// LoadVersions reads a document's version history. A document with no versions
// yet loads as an empty index rather than an error, the same way the document
// index does.
func LoadVersions(docID int) (VersionIndex, error) {
	path, err := versionsPath(docID)
	if err != nil {
		return VersionIndex{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return VersionIndex{NextID: 1}, nil
		}
		return VersionIndex{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var index VersionIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return VersionIndex{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	if index.NextID == 0 {
		index.NextID = 1
	}
	return index, nil
}

// CommitVersion writes a snapshot and records it.
//
// The files land first and the index is saved last, matching Create: a failed
// write must not burn an id or leave the index pointing at a snapshot that is
// not there.
func CommitVersion(docID int, ver Version, index VersionIndex, files map[string][]byte) error {
	dir, err := VersionDir(docID, ver)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	// Sorted so the recorded artifact list is stable rather than map-ordered.
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, files[name], 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	ver.Artifacts = names

	return saveVersions(docID, index.WithPending(ver))
}

func versionsPath(docID int) (string, error) {
	dir, err := Dir(docID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, versionsFile), nil
}

// saveVersions writes the index atomically, the way saveIndex does.
func saveVersions(docID int, index VersionIndex) error {
	path, err := versionsPath(docID)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}
