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

		lib, err := openLibraryWith(cfg)
		if err != nil {
			return err
		}

		// Supplier details used to live in the document. They are read from
		// the config now, so say the old block is being skipped rather than
		// leave the user wondering why what they typed is not on the sheet.
		if doc.HasLegacySupplier() {
			path, err := config.Path()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(),
				"document %d: the 'supplier:' block is no longer read; company and emergency details come from %s\n",
				id, path)
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
			Sources:     doc.SourceData(classification, cfg),
			HazardCodes: doc.HazardCodeSet(),
		})
		if err != nil {
			return fmt.Errorf("resolving document %d:\n%w", id, err)
		}

		// Prepared before the output file is created, so a bad logo path fails
		// without leaving a half-built sheet behind.
		logo, err := generation.PrepareLogo(cfg.Logo, cfg.Company.Name)
		if err != nil {
			return err
		}
		if logo.Oversized() {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: the logo adds %s to every sheet; a smaller file would travel better\n",
				formatBytes(logo.Bytes))
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

		if err := generation.RenderHTML(f, doc, resolved, cfg.Company, logo); err != nil {
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

// formatBytes renders a byte count for a warning message.
func formatBytes(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := int64(n) / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}
