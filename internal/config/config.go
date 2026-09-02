// Package config loads sdsforge's user configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the user's sdsforge settings, read from
// ~/.config/sdsforge/config.toml.
//
// It is split into tables because the three groups answer different questions:
// which content library to draw from, who issues the sheet, and who to call in
// an emergency. The last two are the same on every sheet a company produces,
// which is exactly why they live here rather than in each document.
type Config struct {
	Library   Library   `toml:"library"`
	Company   Company   `toml:"company"`
	Emergency Emergency `toml:"emergency"`
}

// Library selects where section content comes from.
type Library struct {
	// Jurisdiction selects the built-in content library. Only "osha" ships
	// today; the field exists so adding CLP or WHMIS is configuration rather
	// than a code change.
	Jurisdiction string `toml:"jurisdiction"`

	// CustomVariants turns the user's own content library on. When false, the
	// custom directory is neither read nor scanned -- no cost is paid for a
	// feature that is off.
	CustomVariants bool `toml:"custom_variants"`

	// CustomDir is the root of that library. It contains a jurisdiction
	// directory: <CustomDir>/osha/04_first_aid/inhalation/site_specific.yaml
	CustomDir string `toml:"custom_dir"`
}

// Company is the legal entity responsible for the sheet. Section 1 requires a
// name, address and telephone number.
type Company struct {
	Name    string `toml:"name"`
	Address string `toml:"address"`
	Phone   string `toml:"phone"`
	Email   string `toml:"email"`
}

// Emergency holds the numbers to call about an incident with the product.
//
// A list rather than a single number, because a sheet routinely carries
// several: a 24-hour response service, its international line, and the site's
// own safety officer.
type Emergency struct {
	Contacts []Contact `toml:"contacts"`
}

// Contact is one emergency telephone number. Name and Note are optional; a
// bare number still renders.
type Contact struct {
	Name  string `toml:"name"`
	Phone string `toml:"phone"`
	Note  string `toml:"note"`
}

// Default returns the configuration used when no config file exists.
func Default() Config {
	return Config{
		Library: Library{
			Jurisdiction:   "osha",
			CustomVariants: false,
		},
	}
}

// Dir returns the directory holding the config file.
//
// os.UserConfigDir honours $XDG_CONFIG_HOME and falls back to ~/.config, so
// this is the XDG location on Linux without special-casing it here.
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not locate user config directory: %w", err)
	}
	return filepath.Join(dir, "sdsforge"), nil
}

// Path returns the location of the config file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config file, falling back to defaults when it does not exist.
//
// A missing file is normal and must not be an error; a malformed one is an
// error, because silently ignoring it would hide a user's intent.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Config used to be YAML. Falling through to defaults would
			// silently discard a working custom_dir, so say what happened.
			if legacy := legacyPath(path); legacy != "" {
				return Default(), fmt.Errorf(
					"%s holds the old YAML configuration; sdsforge now reads %s\n"+
						"run 'sdsforge config init' to write a starter file, then copy your settings across and delete the old one",
					legacy, path)
			}
			return Default(), nil
		}
		return Default(), fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := cfg.validate(); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// validate rejects a file that parsed but does not say what it means.
func (c *Config) validate() error {
	if c.Library.CustomVariants && c.Library.CustomDir == "" {
		return errors.New("library.custom_variants is on but library.custom_dir is empty")
	}
	if c.Library.Jurisdiction == "" {
		c.Library.Jurisdiction = "osha"
	}
	for i, contact := range c.Emergency.Contacts {
		// A named contact with no number is a typo, not a preference:
		// dropping it quietly would leave a gap on a printed sheet.
		if strings.TrimSpace(contact.Phone) == "" {
			return fmt.Errorf("emergency.contacts[%d] (%q) has no phone", i, contact.Name)
		}
	}
	return nil
}

// legacyPath returns the pre-TOML config file beside path, if it still exists.
func legacyPath(path string) string {
	legacy := filepath.Join(filepath.Dir(path), "config.yaml")
	if _, err := os.Stat(legacy); err != nil {
		return ""
	}
	return legacy
}

// IsZero reports whether any company detail has been configured.
func (c Company) IsZero() bool {
	return c.Name == "" && c.Address == "" && c.Phone == "" && c.Email == ""
}

// Lines renders the company as the section 1 "Supplier details" block, one
// entry per line. Empty fields are skipped rather than printed blank.
func (c Company) Lines() []string {
	var lines []string
	if c.Name != "" {
		lines = append(lines, c.Name)
	}
	if c.Address != "" {
		lines = append(lines, c.Address)
	}
	if c.Phone != "" {
		lines = append(lines, "Telephone: "+c.Phone)
	}
	if c.Email != "" {
		lines = append(lines, "Email: "+c.Email)
	}
	return lines
}

// Lines renders the emergency contacts, one per line:
//
//	CHEMTREC (24 hr): 1-800-424-9300 (USA)
//
// Name and note are optional, so a bare number renders as just the number.
func (e Emergency) Lines() []string {
	lines := make([]string, 0, len(e.Contacts))
	for _, c := range e.Contacts {
		if c.Phone == "" {
			continue // Load rejects these; skip defensively for hand-built values.
		}
		line := c.Phone
		if c.Name != "" {
			line = c.Name + ": " + line
		}
		if c.Note != "" {
			line += " (" + c.Note + ")"
		}
		lines = append(lines, line)
	}
	return lines
}
