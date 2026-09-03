package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/generation"
	"github.com/isaacvarg/sdsforge/internal/launch"
	"github.com/spf13/cobra"
)

// starterConfig is the file 'config init' writes.
//
// Authored text rather than a marshalled struct, for the same reason the
// document scaffold is: the comments are the feature. A marshal round-trip
// would produce a correct file that told the user nothing about what to put
// in it.
//
//go:embed templates/config.toml
var starterConfig []byte

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect and create the user configuration",
	Long: `sdsforge reads company details, emergency contacts and content library
settings from a TOML file, so they need not be repeated in every document.

The file lives at $XDG_CONFIG_HOME/sdsforge/config.toml, which is
~/.config/sdsforge/config.toml unless XDG_CONFIG_HOME says otherwise.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the location of the config file",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a commented starter config file",
	Long: `Create the config file with every setting present and explained.

An existing file is never overwritten unless --force is given: it holds
details that were typed by hand.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}

		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		if !force {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists; pass --force to overwrite it", path)
			} else if !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("checking %s: %w", path, err)
			}
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
		}
		// 0600: the file carries a company's contact details, not public data.
		if err := os.WriteFile(path, starterConfig, 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the configuration as sdsforge reads it",
	Long: `Show the settings actually in effect, including the defaults used for
anything the file leaves out.

This answers "why does my sheet still say no supplier details have been
recorded" -- if the company block below is empty, the file is not being read.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(out, "%s does not exist; showing defaults.\n"+
				"Run 'sdsforge config init' to create it.\n\n", path)
		} else {
			fmt.Fprintf(out, "%s\n\n", path)
		}

		fmt.Fprintln(out, "[library]")
		fmt.Fprintf(out, "  jurisdiction:    %s\n", cfg.Library.Jurisdiction)
		fmt.Fprintf(out, "  custom_variants: %t\n", cfg.Library.CustomVariants)
		fmt.Fprintf(out, "  custom_dir:      %s\n", orNone(cfg.Library.CustomDir))

		fmt.Fprintln(out, "\n[company]")
		printLines(out, cfg.Company.Lines(), "not configured -- section 1 will show the library's placeholder")

		fmt.Fprintln(out, "\n[emergency]")
		printLines(out, cfg.Emergency.Lines(), "no contacts -- section 1 will show the library's default number")

		fmt.Fprintln(out, "\n[logo]")
		printLogo(out, cfg.Logo, cfg.Company.Name)

		fmt.Fprintln(out, "\n[pdf]")
		printPDF(out, cfg.PDF)

		fmt.Fprintln(out, "\n[edit]")
		printEdit(out, cfg.Edit)

		fmt.Fprintln(out, "\n[cd]")
		printCD(out, cfg.CD)

		return nil
	},
}

// printLogo reports what the logo will look like on the page.
//
// This is where "why does my logo come out tiny" gets answered, and the one
// place a bad path surfaces without generating a document.
func printLogo(out io.Writer, cfg config.Logo, companyName string) {
	if cfg.IsZero() {
		fmt.Fprintln(out, "  (no logo configured)")
		return
	}

	fmt.Fprintf(out, "  path:  %s\n", cfg.Path)

	logo, err := generation.PrepareLogo(cfg, companyName)
	if err != nil {
		fmt.Fprintf(out, "  ERROR: %v\n", err)
		return
	}

	if logo.Measured {
		fmt.Fprintf(out, "  image: %d x %d px, %s encoded\n",
			logo.PixelWidth, logo.PixelHeight, formatBytes(logo.Bytes))
		fmt.Fprintf(out, "  print: %s wide x %s tall\n",
			config.FormatLength(logo.WidthMM), config.FormatLength(logo.HeightMM))
	} else {
		fmt.Fprintf(out, "  image: %s encoded; dimensions could not be read\n",
			formatBytes(logo.Bytes))
		fmt.Fprintln(out, "  print: bounded by max_width/max_height, ratio left to the renderer")
	}
	fmt.Fprintf(out, "  css:   %s\n", logo.Style)
	fmt.Fprintf(out, "  alt:   %s\n", logo.Alt)
}

// printEdit reports which editor 'document edit' will open, and what it will do
// afterwards.
//
// The editor is resolved here exactly as that command resolves it, so this is
// where "why does it keep opening vi" gets answered -- the same service printPDF
// performs for the browser.
func printEdit(out io.Writer, cfg config.Edit) {
	if editor, err := launch.Editor(cfg); err != nil {
		fmt.Fprintf(out, "  command:      ERROR: %v\n", err)
	} else {
		fmt.Fprintf(out, "  command:      %s (%s)\n", editor.Path, editor.Origin)
		if len(editor.Args) > 0 {
			fmt.Fprintf(out, "  args:         %s\n", editor.ArgsString())
		}
	}

	var after []string
	if cfg.Classify {
		after = append(after, "classify")
	}
	if cfg.Generate {
		after = append(after, "generate")
	}
	if len(after) == 0 {
		after = []string{"nothing (the file is always checked for parse errors)"}
	}
	fmt.Fprintf(out, "  after edit:   %s\n", strings.Join(after, ", "))

	switch duration, err := cfg.MinDurationValue(); {
	case err != nil:
		fmt.Fprintf(out, "  min_duration: ERROR: %v\n", err)
	case duration == 0:
		fmt.Fprintln(out, "  min_duration: 0 (no warning when the editor returns immediately)")
	default:
		fmt.Fprintf(out, "  min_duration: %s\n", duration)
	}
}

// printCD reports which shell 'sdsforge cd' will launch.
func printCD(out io.Writer, cfg config.CD) {
	shell, err := launch.Shell(cfg)
	if err != nil {
		fmt.Fprintf(out, "  command:      ERROR: %v\n", err)
		return
	}
	fmt.Fprintf(out, "  command:      %s (%s)\n", shell.Path, shell.Origin)
	if len(shell.Args) > 0 {
		fmt.Fprintf(out, "  args:         %s\n", shell.ArgsString())
	}
}

// printPDF reports what printing will do, and whether it can happen at all.
//
// This is where "why does generate fail" gets answered without producing a
// document: the browser is resolved here exactly as 'generate' resolves it.
func printPDF(out io.Writer, cfg config.PDF) {
	if browser, err := generation.FindBrowser(cfg.Browser); err != nil {
		fmt.Fprintf(out, "  browser: ERROR: %v\n", err)
	} else {
		fmt.Fprintf(out, "  browser: %s\n", browser)
		if cfg.Browser == "" {
			fmt.Fprintln(out, "           (found on PATH; set 'browser' to pin one)")
		}
	}

	width, height, margin, err := cfg.Geometry()
	if err != nil {
		fmt.Fprintf(out, "  page:    ERROR: %v\n", err)
		return
	}
	fmt.Fprintf(out, "  paper:   %s (%.2fin x %.2fin)\n", cfg.Paper, width, height)
	fmt.Fprintf(out, "  margin:  %s (%.2fin)\n", cfg.MarginCSS(), margin)
}

// printLines writes a rendered block, or a note explaining an empty one.
func printLines(out io.Writer, lines []string, empty string) {
	if len(lines) == 0 {
		fmt.Fprintf(out, "  (%s)\n", empty)
		return
	}
	for _, line := range lines {
		fmt.Fprintf(out, "  %s\n", line)
	}
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)

	configInitCmd.Flags().Bool("force", false, "Overwrite an existing config file")
}
