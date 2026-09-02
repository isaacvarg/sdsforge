package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// The version every document starts life with.
const (
	InitialVersionLabel = "1.0.0"
	InitialVersionMemo  = "Authored document"
)

// Dir returns the directory holding one document's files.
func Dir(id int) (string, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, strconv.Itoa(id)), nil
}

// Load reads one document by id.
func Load(id int) (Data, error) {
	dir, err := Dir(id)
	if err != nil {
		return Data{}, err
	}
	path := filepath.Join(dir, DocumentFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		return Data{}, fmt.Errorf("reading document %d: %w", id, err)
	}

	var data Data
	if err := yaml.Unmarshal(raw, &data); err != nil {
		return Data{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return data, nil
}

// Create allocates a new document id and writes raw file content for it.
//
// Create takes bytes rather than a Data value, because the scaffold is authored
// text whose comments would be lost by a marshal round-trip.
//
// The file is written BEFORE the index is updated, so a failed write does not
// burn an id or leave the index pointing at a document that does not exist.
//
// Authoring a document is itself version 1.0.0, recorded here rather than in the
// CLI layer so every caller gets one. That first version archives the
// document.yaml and nothing else: rendering a sheet needs a headless browser,
// and a scaffold with no hazard codes yet would only produce an empty placeholder
// PDF. Creating a document therefore stays instant and cannot fail for want of
// Chrome. The HTML and PDF appear from the first real 'version create'.
func Create(name string, content []byte) (string, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return "", err
	}

	nextID, err := NextID()
	if err != nil {
		return "", err
	}

	dirPath := filepath.Join(directory, strconv.Itoa(nextID))
	if err := os.MkdirAll(dirPath, 0o700); err != nil {
		return "", fmt.Errorf("creating %s: %w", dirPath, err)
	}

	path := filepath.Join(dirPath, DocumentFile)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	if err := IndexSave(name); err != nil {
		return "", err
	}

	initial := VersionIndex{NextID: 1}
	first := initial.Draft(InitialVersionLabel, InitialVersionMemo, time.Now())
	if err := CommitVersion(nextID, first, initial, map[string][]byte{
		DocumentFile: content,
	}); err != nil {
		return "", fmt.Errorf("recording version %s: %w", InitialVersionLabel, err)
	}

	return path, nil
}
