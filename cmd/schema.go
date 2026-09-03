package cmd

import (
	"fmt"
	"os"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/schema"
	"github.com/isaacvarg/sdsforge/internal/sections"
	"github.com/spf13/cobra"
)

var (
	schemaOutput string
	schemaCustom bool
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the JSON Schema for document.yaml",
	Long: `Print a JSON Schema describing document.yaml, generated from the content
library: every section id, every preset, and every per-subsection variant is
enumerated from the files that actually exist.

Point an editor at it for completion and validation while you write a document.
docs/editor-setup.md has the wiring for Neovim, VS Code and others; the schema
this command emits is also committed to docs/document.schema.json.

By default the schema describes the BUILT-IN library only, and no config file is
read -- so the output is the same on every machine. Pass --custom to fold in your
own content layer, which is what you want for a schema your editor will use.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := sections.LibraryOptions{}
		if schemaCustom {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			opts = sections.LibraryOptions{
				Jurisdiction:   cfg.Library.Jurisdiction,
				CustomVariants: cfg.Library.CustomVariants,
				CustomDir:      cfg.Library.CustomDir,
			}
		}

		lib, err := sections.NewLibrary(opts)
		if err != nil {
			return err
		}

		out, err := schema.Generate(lib)
		if err != nil {
			return err
		}

		if schemaOutput == "" {
			_, err := cmd.OutOrStdout().Write(out)
			return err
		}
		if err := os.WriteFile(schemaOutput, out, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", schemaOutput, err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "wrote %s (%d bytes)\n", schemaOutput, len(out))
		return nil
	},
}

func init() {
	schemaCmd.Flags().StringVarP(&schemaOutput, "output", "o", "", "write to a file instead of stdout")
	schemaCmd.Flags().BoolVar(&schemaCustom, "custom", false, "include your custom content library layer")
	rootCmd.AddCommand(schemaCmd)
}
