package document

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// ID identifies one document, and names the directory holding it.
//
// The canonical form is a 26-character uppercase Crockford base32 ULID. Its
// leading 48 bits are the creation time in milliseconds, so a plain
// lexicographic sort of ids is a chronological sort of documents -- which is
// what lets the listing come out in creation order without a stored index.
//
// The point of a ULID here is that it needs no coordination. The store is a git
// repository shared by several people, and an incrementing counter means two of
// them who create a document between pulls both claim the same number: the same
// directory, and a conflict in the file that hands the numbers out. A ULID is
// minted locally, from the clock and 80 bits of randomness, and is unique
// whether or not the other machines have ever been heard from.
//
// The id is internal. It never appears on a rendered sheet, which is identified
// by its product name and version label, and it is not written into
// document.yaml -- the directory name is the only place it lives.
type ID string

// IDLength is the length of a ULID in its canonical text form.
const IDLength = 26

// NewID mints an id for a document created at the given time.
//
// The time is a parameter rather than time.Now() because the migration off
// numeric ids has to stamp each existing document with the time it was really
// authored, or the store would come out sorted by the order it happened to be
// migrated in.
func NewID(at time.Time) (ID, error) {
	id, err := ulid.New(ulid.Timestamp(at.UTC()), rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generating document id: %w", err)
	}
	return ID(id.String()), nil
}

// ParseID validates a string as an id and returns it in canonical form.
//
// Input is accepted in any case, since an id typed by hand or completed by a
// shell may well come back lowercased.
func ParseID(s string) (ID, error) {
	upper := strings.ToUpper(strings.TrimSpace(s))
	if len(upper) != IDLength {
		return "", fmt.Errorf("%q is not a document id: expected %d characters, got %d", s, IDLength, len(upper))
	}
	if _, err := ulid.ParseStrict(upper); err != nil {
		return "", fmt.Errorf("%q is not a document id: %w", s, err)
	}
	return ID(upper), nil
}

// IsID reports whether s is a well-formed id. It is how a directory listing
// tells documents apart from whatever else is sitting in the store.
func IsID(s string) bool {
	_, err := ParseID(s)
	return err == nil
}

// Time returns when the document was created, to the millisecond.
//
// A malformed id -- which cannot arise from ParseID -- reports the zero time
// rather than failing, because this is only ever used for display and ordering.
func (id ID) Time() time.Time {
	parsed, err := ulid.ParseStrict(string(id))
	if err != nil {
		return time.Time{}
	}
	return ulid.Time(parsed.Time()).UTC()
}

// Short returns the leading characters of the id, for messages.
//
// Every command that takes an id accepts a unique prefix, so the short form is
// usually enough to type -- but only usually. It is display, not identity:
// two documents created within about a second of each other share it, and a
// command given an ambiguous prefix says so rather than guessing.
func (id ID) Short() string {
	if len(id) <= ShortIDLength {
		return string(id)
	}
	return string(id[:ShortIDLength])
}

// ShortIDLength is how much of an id Short keeps: 40 of the 48 timestamp bits,
// which is a resolution of about a second.
const ShortIDLength = 8

func (id ID) String() string { return string(id) }
