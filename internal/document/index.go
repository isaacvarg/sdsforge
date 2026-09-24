package document

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Entry is one document as the listing sees it.
type Entry struct {
	ID   ID
	Name string
}

// Slug is the entry's product name reduced to a path-safe form. It is what a
// user is most likely to type in place of an id.
func (e Entry) Slug() string { return Slugify(e.Name) }

// Display names the document for a listing or an error. A scaffold that has
// been created but not filled in has no product name yet.
func (e Entry) Display() string {
	if e.Name == "" {
		return "(unnamed)"
	}
	return e.Name
}

// List returns every document in the store, oldest first.
//
// The listing is DERIVED, scanned from the documents directory rather than read
// from an index file. A stored index is a file that every create has to write,
// and the store is shared over git: two people creating a document between
// pulls would conflict in it every time, even for unrelated documents. It also
// drifts -- a directory removed by hand leaves an entry behind, and the listing
// then names a document that is not there. Scanning cannot be wrong about what
// exists.
//
// Anything in the directory that is not a document -- a stray file, an editor's
// backup, a directory with no document.yaml -- is skipped rather than reported.
// The store is a git working tree and is not ours alone to be strict about.
func List() ([]Entry, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return nil, err
	}

	items, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", directory, err)
	}

	entries := make([]Entry, 0, len(items))
	for _, item := range items {
		if !item.IsDir() || !IsID(item.Name()) {
			continue
		}
		id := ID(item.Name())

		name, ok, err := describe(filepath.Join(directory, item.Name()))
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		entries = append(entries, Entry{ID: id, Name: name})
	}

	// Ids are ULIDs, so sorting them as text sorts the documents by the time
	// they were created.
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

// describe reports whether a directory holds a document, and what it is called.
//
// Only the product name is decoded: the listing has no use for the rest, and a
// document whose materials or sections are mid-edit and unparseable should
// still be listable under its id.
//
// A directory with a version history but no live document.yaml counts. That is
// exactly the state 'document edit' tells a user to recover from with
// 'document version restore', and they cannot do it against a document the
// listing refuses to show and the resolver refuses to find.
func describe(dir string) (name string, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, DocumentFile))
	if err != nil {
		if !os.IsNotExist(err) {
			return "", false, fmt.Errorf("reading %s: %w", dir, err)
		}
		return "", hasVersionHistory(dir), nil
	}

	var head struct {
		ProductName string `yaml:"product_name"`
	}
	if err := yaml.Unmarshal(raw, &head); err != nil {
		// A document that does not parse is still a document. Naming it by
		// its id beats dropping it out of the listing.
		return "", true, nil
	}
	return head.ProductName, true, nil
}

func hasVersionHistory(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, versionsFile))
	return err == nil
}

// Resolve turns what a user typed into one document id.
//
// Ids are ULIDs and nobody is going to type 26 characters, so a reference may
// be any of:
//
//	01K6H3PZ8Q7XN4V2R9BKTC5M0E   the id itself
//	01k6h3                       a leading piece of it, in either case
//	suspension-shower-gel        the product name, slugified
//	"suspension shower"          the start of the product name
//
// Anything that picks out exactly one document is accepted. Anything that picks
// out several is refused with the candidates listed, because guessing between
// them could edit or re-issue the wrong safety data sheet.
func Resolve(ref string) (ID, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", fmt.Errorf("no document given")
	}

	entries, err := List()
	if err != nil {
		return "", err
	}

	// An exact id wins outright, and costs nothing to check first.
	if id, err := ParseID(trimmed); err == nil {
		for _, entry := range entries {
			if entry.ID == id {
				return id, nil
			}
		}
		return "", fmt.Errorf(
			"no document with id %s\n"+
				"run 'sdsforge document list' to see the ones that exist", id)
	}

	idPrefix := strings.ToUpper(trimmed)
	namePrefix := Slugify(trimmed)

	var matches []Entry
	for _, entry := range entries {
		byID := strings.HasPrefix(string(entry.ID), idPrefix)
		byName := namePrefix != "" && strings.HasPrefix(entry.Slug(), namePrefix)
		if byID || byName {
			matches = append(matches, entry)
		}
	}

	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", fmt.Errorf(
			"no document matches %q\n"+
				"run 'sdsforge document list' to see the ones that exist", ref)
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q matches %d documents:\n", ref, len(matches))
		for _, entry := range matches {
			fmt.Fprintf(&b, "  %s  %s\n", entry.ID, entry.Display())
		}
		b.WriteString("type more of the id or the name to pick one")
		return "", fmt.Errorf("%s", b.String())
	}
}
