package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/sections"
	"github.com/spf13/cobra"
)

// openLibrary builds the content library from the user's configuration.
func openLibrary() (*sections.Library, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return sections.NewLibrary(sections.LibraryOptions{
		Jurisdiction:   cfg.Jurisdiction,
		CustomVariants: cfg.CustomVariants,
		CustomDir:      cfg.CustomDir,
	})
}

var sectionsCmd = &cobra.Command{
	Use:   "sections",
	Short: "Inspect and validate the section content library",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var sectionsListCmd = &cobra.Command{
	Use:   "list [section-id]",
	Short: "List sections, or one section's subsections and variants",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lib, err := openLibrary()
		if err != nil {
			return err
		}
		layout, err := sections.LoadLayout(lib)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// No argument: an overview of the whole library.
		if len(args) == 0 {
			fmt.Fprintf(out, "jurisdiction: %s (layers: %s)\n\n",
				lib.Jurisdiction(), strings.Join(lib.Layers(), ", "))
			fmt.Fprintln(w, "NO.\tID\tTITLE\tSUBSECTIONS\tPRESETS")
			for _, dir := range layout.Sections {
				def, err := sections.LoadSection(lib, dir)
				if err != nil {
					return err
				}
				presets, err := lib.ListPresets(dir)
				if err != nil {
					return err
				}
				presetList := "-"
				if len(presets) > 0 {
					presetList = strings.Join(presets, ", ")
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%s\n",
					def.Number, def.ID, def.Title, len(def.Subsections), presetList)
			}
			return nil
		}

		// With an argument: drill into one section.
		want := args[0]
		for _, dir := range layout.Sections {
			def, err := sections.LoadSection(lib, dir)
			if err != nil {
				return err
			}
			if def.ID != want {
				continue
			}

			fmt.Fprintf(out, "%d. %s  (id: %s)\n\n", def.Number, def.Title, def.ID)
			fmt.Fprintln(w, "SUBSECTION\tKIND\tVARIANTS")
			for _, sub := range def.Subsections {
				variants, err := lib.ListVariants(def.Dir, sub.ID)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", sub.ID, sub.Kind, strings.Join(variants, ", "))
			}
			w.Flush()

			presets, err := lib.ListPresets(def.Dir)
			if err != nil {
				return err
			}
			if len(presets) > 0 {
				fmt.Fprintf(out, "\npresets: %s\n", strings.Join(presets, ", "))
			}
			return nil
		}

		return fmt.Errorf("no section with id %q; run `sdsforge sections list` to see them all", want)
	},
}

var sectionsValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check the content library for errors",
	Long: `Verify that every section manifest, variant file, and preset in the content
library is well formed -- including the custom library when it is enabled.

Reports every problem found, not just the first.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		lib, err := openLibrary()
		if err != nil {
			return err
		}
		if err := sections.ValidateLibrary(lib); err != nil {
			return fmt.Errorf("content library has problems:\n%w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "content library OK (layers: %s)\n",
			strings.Join(lib.Layers(), ", "))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sectionsCmd)
	sectionsCmd.AddCommand(sectionsListCmd)
	sectionsCmd.AddCommand(sectionsValidateCmd)
}
