package document

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func DocumentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w ", err)
	}

	directory := filepath.Join(home, ".local", "share", "sdsforge", "documents")

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

	result := builder.String()
	return result
}
