package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// DocumentsDir returns the directory holding saved documents, creating it if
// it does not exist.
//
// $XDG_DATA_HOME when set, ~/.local/share otherwise -- the XDG default. Go has
// no os.UserDataDir to lean on the way config does, so the lookup is spelled
// out here.
func DocumentsDir() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get home directory: %w ", err)
		}
		base = filepath.Join(home, ".local", "share")
	}

	directory := filepath.Join(base, "sdsforge", "documents")

	error := os.MkdirAll(directory, 0o700)
	if error != nil {
		return "", errors.New("directory couldn't be made")
	}

	return directory, nil
}

func Slugify(name string) string {
	s := strings.ToLower(name)

	var builder strings.Builder
	// this keeps too many dashes from being next to each other
	dash := false

	for _, rune := range s {
		if unicode.IsLetter(rune) || unicode.IsNumber(rune) {
			builder.WriteRune(rune)
			dash = false
		} else {
			if !dash && builder.Len() > 0 {

				builder.WriteRune('-')
				dash = true
			}
		}
	}

	// A name ending in punctuation ("Caustic Soda 50%") leaves a trailing
	// separator, which shows up in generated filenames.
	result := strings.Trim(builder.String(), "-")
	return result
}
