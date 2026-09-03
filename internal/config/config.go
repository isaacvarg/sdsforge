// Package config loads sdsforge's user configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
	Logo      Logo      `toml:"logo"`
	PDF       PDF       `toml:"pdf"`
	Edit      Edit      `toml:"edit"`
	CD        CD        `toml:"cd"`
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

// Logo is the company mark printed in the sheet header.
//
// Sizing is automatic: the artwork is measured and fitted, so a user need not
// work out print dimensions for whatever their design team exported. MaxHeight
// and MaxWidth describe a BOX the image is fitted inside, not a size it is
// forced to -- the aspect ratio always survives.
type Logo struct {
	// Path is the artwork. A relative path is taken as relative to the config
	// file itself, so a logo sitting beside config.toml needs only its name.
	// Load resolves it to an absolute path.
	Path string `toml:"path"`

	// MaxHeight and MaxWidth are CSS lengths ("16mm", "0.75in"). Empty means
	// the built-in default box; see internal/generation.
	MaxHeight string `toml:"max_height"`
	MaxWidth  string `toml:"max_width"`

	// Alt is what a screen reader announces and what a text-only rendering
	// falls back to. Defaulted from the company name when empty.
	Alt string `toml:"alt"`
}

// IsZero reports whether no logo has been configured.
func (l Logo) IsZero() bool { return l.Path == "" }

// PDF is how the finished sheet gets printed.
//
// The paper a sheet is printed on is a property of where it is issued, not of
// the product, so it belongs here beside the company details rather than in
// each document.
type PDF struct {
	// Browser is the Chrome-based browser used to print. Empty means search
	// PATH; see FindBrowser in internal/generation. A bare name is looked up on
	// PATH, anything with a separator is used as given.
	Browser string `toml:"browser"`

	// Paper names the sheet size: letter, legal, a4 or a5.
	Paper string `toml:"paper"`

	// Margin is the page margin on all four sides, as a CSS length ("0.75in").
	// The running footer is drawn inside it, so it cannot be zero.
	Margin string `toml:"margin"`
}

// Edit is how 'sdsforge document edit' opens a document for editing.
//
// Which editor to use is a property of the machine, not of the product, so it
// belongs here beside the browser rather than in each document.
type Edit struct {
	// Command is the editor to run. Empty means $VISUAL, then $EDITOR, then an
	// OS default -- see internal/launch. A bare name is looked up on PATH.
	Command string `toml:"command"`

	// Args are passed to the editor before the file, whichever source Command
	// came from. This is the place for anything needing quotes, e.g.
	// ["-c", "set ft=yaml"].
	Args []string `toml:"args"`

	// Classify and Generate are what happens after a successful edit. Both
	// default to off: the file is always checked for parse errors, and anything
	// beyond that is a choice about how you like to work. Generate needs a
	// browser and takes seconds, which is why it is not on by default even for
	// people who would want it most of the time.
	Classify bool `toml:"classify"`
	Generate bool `toml:"generate"`

	// MinDuration is how quickly an editor has to return before sdsforge says
	// something. An editor that forks and exits immediately (a GUI one invoked
	// without its wait flag) leaves sdsforge checking a file nobody has touched
	// yet, and the resulting "it says my edit is fine but I had not saved it"
	// is very hard to work out unaided. A duration string; "0" turns the
	// warning off.
	MinDuration string `toml:"min_duration"`
}

// MinDurationValue parses MinDuration.
//
// Kept beside the field rather than in the command, so a bad value is rejected
// when the config file is read instead of at the moment someone edits -- the
// same reason PDF.Geometry is checked by validate.
func (e Edit) MinDurationValue() (time.Duration, error) {
	value := strings.TrimSpace(e.MinDuration)
	if value == "" {
		value = defaultEditMinDuration
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("edit.min_duration: %q is not a duration (try \"1s\")", e.MinDuration)
	}
	if duration < 0 {
		return 0, fmt.Errorf("edit.min_duration: %q is negative", e.MinDuration)
	}
	return duration, nil
}

// CD is how 'sdsforge cd' launches a shell.
type CD struct {
	// Command is the shell to run. Empty means $SHELL, then an OS default.
	Command string `toml:"command"`

	// Args are passed to it before anything else.
	Args []string `toml:"args"`
}

// paperSizes maps a paper name to its size in millimetres, portrait.
//
// A name rather than two lengths: nobody remembers that US Letter is 215.9mm
// wide, and a typo in a raw measurement prints a subtly wrong page.
var paperSizes = map[string][2]float64{
	"letter": {215.9, 279.4},
	"legal":  {215.9, 355.6},
	"a4":     {210, 297},
	"a5":     {148, 210},
}

// PaperNames lists the accepted paper values, sorted, for error messages and
// help text.
func PaperNames() []string {
	names := make([]string, 0, len(paperSizes))
	for name := range paperSizes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Geometry returns the page size and margin in INCHES, which is the unit
// Chrome's Page.printToPDF takes. Converted here so nothing downstream has to
// know that.
func (p PDF) Geometry() (widthIn, heightIn, marginIn float64, err error) {
	size, ok := paperSizes[normalisePaper(p.Paper)]
	if !ok {
		return 0, 0, 0, fmt.Errorf("pdf.paper: %q is not a paper size; use %s",
			p.Paper, strings.Join(PaperNames(), ", "))
	}

	margin := p.Margin
	if margin == "" {
		margin = defaultMargin
	}
	marginMM, err := ParseLength(margin)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("pdf.margin: %w", err)
	}

	return size[0] / mmPerInch, size[1] / mmPerInch, marginMM / mmPerInch, nil
}

// MarginCSS renders the configured margin as a CSS length, for the footer
// template's padding -- which has to match the page margin exactly or the
// footer sits flush to the paper edge.
func (p PDF) MarginCSS() string {
	margin := p.Margin
	if margin == "" {
		margin = defaultMargin
	}
	mm, err := ParseLength(margin)
	if err != nil {
		return defaultMargin // validate rejects this; be harmless if it slips through.
	}
	return FormatLength(mm)
}

// normalisePaper accepts "Letter" and "A4" as readily as "letter" and "a4".
func normalisePaper(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// The page a sheet prints on when the config file says nothing.
//
// US Letter because OSHA's Hazard Communication Standard is the only
// jurisdiction that ships; 0.75in leaves the running footer room to sit inside
// the bottom margin without crowding the content.
const (
	defaultPaper  = "letter"
	defaultMargin = "0.75in"
)

// defaultEditMinDuration is the threshold below which an editor's return is
// treated as suspicious. One second is long enough that no real editing session
// trips it and short enough that a forking editor always does.
const defaultEditMinDuration = "1s"

// Default returns the configuration used when no config file exists.
func Default() Config {
	return Config{
		Library: Library{
			Jurisdiction:   "osha",
			CustomVariants: false,
		},
		PDF: PDF{
			Paper:  defaultPaper,
			Margin: defaultMargin,
		},
		Edit: Edit{
			MinDuration: defaultEditMinDuration,
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

	// Resolved here, while the config file's own location is in hand, so
	// nothing downstream needs to know where it lives.
	if err := cfg.resolveLogoPath(filepath.Dir(path)); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
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
	// Filled in rather than rejected: a file that omits [pdf] entirely is the
	// normal case, and toml.Unmarshal leaves the fields empty rather than at
	// the values Default() put there.
	if c.PDF.Paper == "" {
		c.PDF.Paper = defaultPaper
	}
	c.PDF.Paper = normalisePaper(c.PDF.Paper)
	if c.PDF.Margin == "" {
		c.PDF.Margin = defaultMargin
	}
	// Geometry checks both paper and margin, so a bad value is caught when the
	// file is read rather than at the moment a user prints.
	if _, _, _, err := c.PDF.Geometry(); err != nil {
		return err
	}
	if c.Edit.MinDuration == "" {
		c.Edit.MinDuration = defaultEditMinDuration
	}
	// Checked here for the same reason as the page geometry: a typo in a
	// duration should surface when the file is read, not when someone edits.
	if _, err := c.Edit.MinDurationValue(); err != nil {
		return err
	}
	for key, value := range map[string]string{
		"logo.max_height": c.Logo.MaxHeight,
		"logo.max_width":  c.Logo.MaxWidth,
	} {
		if value == "" {
			continue
		}
		if _, err := ParseLength(value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	// Whether the BROWSER exists is not checked here for the same reason the
	// logo file is not: Load runs for every command, and 'sections list' has no
	// business failing over Chrome not being installed.
	//
	// Whether the file EXISTS is deliberately not checked here: Load runs for
	// every command, and 'sections list' has no business failing over a logo.
	for i, contact := range c.Emergency.Contacts {
		// A named contact with no number is a typo, not a preference:
		// dropping it quietly would leave a gap on a printed sheet.
		if strings.TrimSpace(contact.Phone) == "" {
			return fmt.Errorf("emergency.contacts[%d] (%q) has no phone", i, contact.Name)
		}
	}
	return nil
}

// resolveLogoPath makes the configured logo path absolute, expanding a leading
// ~ and taking anything else relative to the config directory.
func (c *Config) resolveLogoPath(dir string) error {
	if c.Logo.IsZero() || filepath.IsAbs(c.Logo.Path) {
		return nil
	}

	if c.Logo.Path == "~" || strings.HasPrefix(c.Logo.Path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("logo.path: expanding ~: %w", err)
		}
		c.Logo.Path = filepath.Join(home, strings.TrimPrefix(c.Logo.Path, "~"))
		return nil
	}

	c.Logo.Path = filepath.Join(dir, c.Logo.Path)
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
