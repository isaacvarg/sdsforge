package document

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// legacyIndexFile is the store-wide index that documents used to be listed in.
//
// It held a next_id counter, and every create rewrote it. Two people creating a
// document against the same shared store therefore conflicted in it every time,
// whether or not their documents had anything to do with each other. The
// listing is scanned now -- see List -- and the file is deleted by Migrate.
const legacyIndexFile = "index.yaml"

// Rename is one directory the migration will move.
type Rename struct {
	// From is the old directory's name, a decimal number.
	From string
	To   ID
	// Name is the product name, for the report. Migration does not depend on
	// it, so a document that will not parse still migrates.
	Name string
	// At is when the document was authored, and is what To's timestamp is
	// built from.
	At time.Time
}

// PlanMigration works out what it would take to move a store off numeric ids,
// without touching anything.
//
// Every directory whose name is not already an id is one to move. The new id is
// minted from the document's ORIGINAL authoring time -- its first recorded
// version, or failing that the directory's modification time -- so that ids,
// which sort chronologically, go on describing the order the documents were
// really written in. Stamping them with the time of the migration would sort
// the whole store by whatever order the filesystem happened to list it in.
//
// The result is ordered oldest first, which is the order the report reads best
// in and the order the ids come out in.
func PlanMigration() ([]Rename, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return nil, err
	}

	items, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", directory, err)
	}

	renames := make([]Rename, 0, len(items))
	for _, item := range items {
		if !item.IsDir() || IsID(item.Name()) {
			continue
		}
		dir := filepath.Join(directory, item.Name())

		// A directory holding neither a document nor a version history is not
		// a document. The store is a git working tree and may hold anything.
		name, ok, err := describe(dir)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		renames = append(renames, Rename{
			From: item.Name(),
			Name: name,
			At:   authoredAt(dir, item),
		})
	}

	sort.Slice(renames, func(i, j int) bool { return renames[i].At.Before(renames[j].At) })

	for i := range renames {
		id, err := NewID(renames[i].At)
		if err != nil {
			return nil, err
		}
		renames[i].To = id
	}
	return renames, nil
}

// authoredAt is the best available answer to when a document was written.
func authoredAt(dir string, item os.DirEntry) time.Time {
	// The first recorded version is the authoring of the document -- see
	// Create -- so its timestamp is the real answer whenever it is there.
	if versions, err := loadVersionsAt(dir); err == nil && len(versions.Versions) > 0 {
		return versions.Versions[0].Timestamp
	}
	if info, err := item.Info(); err == nil {
		return info.ModTime().UTC()
	}
	return time.Now().UTC()
}

// Migrate applies a plan from PlanMigration and removes the legacy index.
//
// Every target is checked before anything moves, so the store is not left half
// renamed by a collision discovered partway through.
func Migrate(renames []Rename) error {
	directory, err := DocumentsDir()
	if err != nil {
		return err
	}

	for _, r := range renames {
		to := filepath.Join(directory, string(r.To))
		if _, err := os.Stat(to); err == nil {
			return fmt.Errorf("cannot rename %s: %s already exists", r.From, r.To)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking %s: %w", to, err)
		}
	}

	for _, r := range renames {
		from := filepath.Join(directory, r.From)
		to := filepath.Join(directory, string(r.To))
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("renaming %s to %s: %w", r.From, r.To, err)
		}
	}

	index := filepath.Join(directory, legacyIndexFile)
	if err := os.Remove(index); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", index, err)
	}
	return nil
}

// HasLegacyIndex reports whether the store still carries the old index file. It
// may be there with nothing left to rename -- the file outlives the last
// numeric directory if those were removed by hand.
func HasLegacyIndex() (bool, error) {
	directory, err := DocumentsDir()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(filepath.Join(directory, legacyIndexFile))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
