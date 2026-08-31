package document

import (
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"
)

func IndexSave(name string) error {
	index, err := loadIndex()
	if err != nil {
		return err
	}

	entry := IndexEntry{ID: index.NextID, Name: name}
	index.Documents = append(index.Documents, entry)
	index.LastModifiedID = entry.ID
	index.NextID++

	err = saveIndex(index)
	if err != nil {
		return err
	}

	return nil
}

func NextID() (int, error) {
	index, err := loadIndex()
	if err != nil {
		return 0, err
	}
	return index.NextID, nil
}

func ListIndex() {
	index, err := loadIndex()
	if err != nil {
		fmt.Println("error loading index: ", err)
		return
	}

	docs := index.Documents
	fmt.Println("ID", ": ", "Document name")

	for _, doc := range docs {
		fmt.Println(doc.ID, ": ", doc.Name)
	}
}

func saveIndex(index Index) error {
	directory, _ := DocumentsDir()
	indexPath := path.Join(directory, "index.yaml")

	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}

	tmp := indexPath + ".tmp"
	err = os.WriteFile(tmp, data, 0o0644)
	if err != nil {
		return err
	}
	return os.Rename(tmp, indexPath)
}

func loadIndex() (Index, error) {
	directory, _ := DocumentsDir()
	indexPath := path.Join(directory, "index.yaml")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Index{
				NextID: 1,
			}, nil
		}
		return Index{}, err
	}

	var index Index
	err = yaml.Unmarshal(data, &index)
	if err != nil {
		return Index{}, err
	}

	if index.NextID == 0 {
		index.NextID = 1
	}

	return index, nil
}
