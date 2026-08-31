// Package document
package document

type Data struct {
	ProductName     string     `yaml:"product_name"`
	LastRevision    string     `yaml:"last_revision"`
	DocumentVersion string     `yaml:"document_version"`
	Revisions       []Revision `yaml:"revisions"`
	Materials       []Material `yaml:"materials"`
}

type Revision struct {
	Version      string `yaml:"version"`
	RevisionDate string `yaml:"revision_date"`
	Description  string `yaml:"description"`
}

type Material struct {
	Sequence         int    `yaml:"sequence"`
	Name             string `yaml:"name"`
	CASNumber        string `yaml:"cas_number"`
	Percentage       string `yaml:"percentage"`
	HazardsTriggered []int  `yaml:"hazards_triggered"`
}

type Index struct {
	NextID         int          `yaml:"next_id"`
	LastModifiedID int          `yaml:"last_modified_id"`
	Documents      []IndexEntry `yaml:"documents"`
}

type IndexEntry struct {
	ID   int    `yaml:"id"`
	Name string `yaml:"name"`
}
