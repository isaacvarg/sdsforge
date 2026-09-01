package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

func Save(data Data) (string, error) {
	//safeName := Slugify(data.ProductName)

	directory, err := DocumentsDir()
	if err != nil {
		fmt.Println(err)
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return "", errors.New("creating yaml data failed")
	}

	nextID, err := NextID()
	if err != nil {
		return "", err
	}

	dirPath := filepath.Join(directory, strconv.Itoa(nextID))
	err = os.MkdirAll(dirPath, 0o700)
	if err != nil {
		return "", errors.New("directory couldn't be made")
	}

	path := filepath.Join(dirPath, "document.yaml")

	err = IndexSave(data.ProductName)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(path, yamlData, 0o644)
	if err != nil {
		fmt.Println(err)
		return "", errors.New("failed to save file")
	}

	return path, nil
}

func Read() {
	directory, err := DocumentsDir()
	if err != nil {
		fmt.Println("documents directory could not be read")
		return
	}

	data, _ := os.ReadDir(directory)
	fmt.Println(data)
}

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
	path := filepath.Join(dir, "document.yaml")

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
// Save marshals a Data value; Create takes bytes, because the scaffold is
// authored text whose comments would be lost by a marshal round-trip.
//
// The file is written BEFORE the index is updated, so a failed write does not
// burn an id or leave the index pointing at a document that does not exist.
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

	path := filepath.Join(dirPath, "document.yaml")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}

	if err := IndexSave(name); err != nil {
		return "", err
	}

	return path, nil
}
