package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/generation"
	"github.com/isaacvarg/sdsforge/internal/ghs"
	"github.com/isaacvarg/sdsforge/internal/sections"
	"github.com/spf13/cobra"
)

// generateCmd renders a stored document into a finished safety data sheet.
var generateCmd = &cobra.Command{
	Use:   "generate <document-id>",
	Short: "Render a document into a safety data sheet",
	Long: `Resolve every section of a stored document against the content library and
render the result as HTML.

Each section falls back to its default content unless the document selects a
preset or overrides individual subsections.`,
	Args: cobra.ExactArgs(1),
	// RunE rather than Run: returning the error lets cobra print it and set a
	// non-zero exit status, which matters for scripting.
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("document id must be a number, got %q", args[0])
		}

		doc, err := document.Load(id)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		lib, err := sections.NewLibrary(sections.LibraryOptions{
			Jurisdiction:   cfg.Jurisdiction,
			CustomVariants: cfg.CustomVariants,
			CustomDir:      cfg.CustomDir,
		})
		if err != nil {
			return err
		}

		// Section 2 is computed from the document's hazard codes, and those
		// same codes derive the wording of every other section.
		tables, err := ghs.LoadTables(lib)
		if err != nil {
			return err
		}
		classification, err := tables.Classify(doc.AllHazardCodes())
		if err != nil {
			return fmt.Errorf("document %d: %w", id, err)
		}
		if err := classification.ApplyText(doc.PrecautionaryText); err != nil {
			return fmt.Errorf("document %d: %w", id, err)
		}

		resolved, err := sections.ResolveAll(lib, doc.Sections, sections.ResolveContext{
			Sources:     doc.SourceData(classification),
			HazardCodes: doc.HazardCodeSet(),
		})
		if err != nil {
			return fmt.Errorf("resolving document %d:\n%w", id, err)
		}

		outPath, err := cmd.Flags().GetString("out")
		if err != nil {
			return err
		}
		if outPath == "" {
			dir, err := document.Dir(id)
			if err != nil {
				return err
			}
			outPath = filepath.Join(dir, document.Slugify(doc.ProductName)+".html")
		}

		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("creating %s: %w", outPath, err)
		}
		defer f.Close()

		if err := generation.RenderHTML(f, doc, resolved); err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), outPath)
		return nil
	},
}

func init() {
	documentCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringP("out", "o", "",
		"Output path (default: the document's directory)")
}
