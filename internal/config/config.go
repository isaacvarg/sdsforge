// Package config loads sdsforge's user configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the user's sdsforge settings, read from
// ~/.config/sdsforge/config.yaml.
type Config struct {
	// Jurisdiction selects the built-in content library. Only "osha" ships
	// today; the field exists so adding CLP or WHMIS is configuration rather
	// than a code change.
	Jurisdiction string `yaml:"jurisdiction"`

	// CustomVariants turns the user's own content library on. When false, the
	// custom directory is neither read nor scanned -- no cost is paid for a
	// feature that is off.
	CustomVariants bool `yaml:"custom_variants"`

	// CustomDir is the root of that library. It contains a jurisdiction
	// directory: <CustomDir>/osha/04_first_aid/inhalation/site_specific.yaml
	CustomDir string `yaml:"custom_dir"`
}

// Default returns the configuration used when no config file exists.
func Default() Config {
	return Config{
		Jurisdiction:   "osha",
		CustomVariants: false,
	}
}

// Path returns the location of the config file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not locate user config directory: %w", err)
	}
	return filepath.Join(dir, "sdsforge", "config.yaml"), nil
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
			return Default(), nil
		}
		return Default(), fmt.Errorf("reading %s: %w", path, err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Default(), fmt.Errorf("parsing %s: %w", path, err)
	}

	if cfg.CustomVariants && cfg.CustomDir == "" {
		return Default(), fmt.Errorf("%s: custom_variants is on but custom_dir is empty", path)
	}
	if cfg.Jurisdiction == "" {
		cfg.Jurisdiction = "osha"
	}

	return cfg, nil
}
