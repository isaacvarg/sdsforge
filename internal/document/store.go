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
