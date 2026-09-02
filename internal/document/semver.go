package document

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is a document's version label: MAJOR.MINOR.PATCH and nothing else.
//
// The full semantic-versioning grammar allows prerelease and build suffixes
// ("1.0.0-rc1+build3"). Those are deliberately rejected here. A version label on
// a safety data sheet is a regulatory identifier that a reader compares by eye
// against a printed sheet, and "which of 1.0.0-rc1 and 1.0.0 was issued" is not a
// question this tool should let someone ask. Three numbers is the whole language.
type Semver struct {
	Major int
	Minor int
	Patch int
}

// BumpPart names the component a bump increments.
type BumpPart int

const (
	BumpMajor BumpPart = iota
	BumpMinor
	BumpPatch
)

// ParseSemver reads a MAJOR.MINOR.PATCH label.
func ParseSemver(s string) (Semver, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH, e.g. 1.2.0", s)
	}

	var out [3]int
	for i, p := range parts {
		// strconv.Atoi accepts a leading sign, so "1.-2.0" and "1.+2.0" would
		// otherwise parse. A version component is a plain digit string.
		if p == "" || strings.IndexFunc(p, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return Semver{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH, e.g. 1.2.0", s)
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return Semver{}, fmt.Errorf("version %q: %w", s, err)
		}
		out[i] = n
	}

	return Semver{Major: out[0], Minor: out[1], Patch: out[2]}, nil
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

// Bump returns the next version for the given part, zeroing everything below it
// -- the minor after 1.4.7 is 1.5.0, not 1.5.7.
func (s Semver) Bump(part BumpPart) Semver {
	switch part {
	case BumpMajor:
		return Semver{Major: s.Major + 1}
	case BumpMinor:
		return Semver{Major: s.Major, Minor: s.Minor + 1}
	default:
		return Semver{Major: s.Major, Minor: s.Minor, Patch: s.Patch + 1}
	}
}

// Less reports whether s sorts below o.
func (s Semver) Less(o Semver) bool {
	if s.Major != o.Major {
		return s.Major < o.Major
	}
	if s.Minor != o.Minor {
		return s.Minor < o.Minor
	}
	return s.Patch < o.Patch
}
