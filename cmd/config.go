package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/generation"
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
