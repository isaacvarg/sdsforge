package document

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// The version every document starts life with.
const (
	InitialVersionLabel = "1.0.0"
	InitialVersionMemo  = "Authored document"
)

// Dir returns the directory holding one document's files.
func Dir(id ID) (string, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, string(id)), nil
}

// Load reads one document by id.
func Load(id ID) (Data, error) {
	dir, err := Dir(id)
	if err != nil {
		return Data{}, err
	}
	path := filepath.Join(dir, DocumentFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		return Data{}, fmt.Errorf("reading document %s: %w", id, err)
	}

	var data Data
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return Data{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return data, nil
}

// Create mints an id for a new document and writes raw file content for it.
//
// Create takes bytes rather than a Data value, because the scaffold is authored
// text whose comments would be lost by a marshal round-trip.
//
// The id is a ULID generated here and now -- see the ID type. Nothing is read
// to find out what the next id should be, and nothing shared is written to
// record that this one was taken, so two people creating a document against the
// same shared store cannot collide and have nothing to merge afterwards.
//
// Authoring a document is itself version 1.0.0, recorded here rather than in the
// CLI layer so every caller gets one. That first version archives the
// document.yaml and nothing else: rendering a sheet needs a headless browser,
// and a scaffold with no hazard codes yet would only produce an empty placeholder
// PDF. Creating a document therefore stays instant and cannot fail for want of
// Chrome. The HTML and PDF appear from the first real 'version create'.
func Create(content []byte) (ID, string, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return "", "", err
	}

	now := time.Now()
	id, err := NewID(now)
	if err != nil {
		return "", "", err
	}

	dirPath := filepath.Join(directory, string(id))
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", dirPath, err)
	}

	path := filepath.Join(dirPath, DocumentFile)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", path, err)
	}

	initial := VersionIndex{NextID: 1}
	first := initial.Draft(InitialVersionLabel, InitialVersionMemo, now)
	if err := CommitVersion(id, first, initial, map[string][]byte{
		DocumentFile: content,
	}); err != nil {
		return "", "", fmt.Errorf("recording version %s: %w", InitialVersionLabel, err)
	}

	return id, path, nil
}
