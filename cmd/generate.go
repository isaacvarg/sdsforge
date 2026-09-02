package cmd

import (
	"bytes"
	"context"
	"errors"
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
render the result as a PDF.

Printing needs a Chrome-based browser installed; sdsforge finds one on PATH, or
uses the one named by 'browser' under [pdf] in the config file. Pass --html to
write the intermediate markup instead, which is the way to see what a template
change did without printing.

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

		htmlOnly, err := cmd.Flags().GetBool("html")
		if err != nil {
			return err
		}

		dir, err := document.Dir(id)
		if err != nil {
			return err
		}
		slug := document.Slugify(doc.ProductName)

		outPath, err := cmd.Flags().GetString("out")
		if err != nil {
			return err
		}
		if outPath == "" {
			outPath = filepath.Join(dir, slug+extension(htmlOnly))
		}

		view := generation.NewView(doc, resolved, cfg, logo)
		view.ForPDF = !htmlOnly

		// Rendered to memory rather than straight to the file: on the PDF path
		// the markup is an intermediate that goes to the browser and nowhere
		// else, and on either path a half-written sheet is worse than none.
		var html bytes.Buffer
		if err := generation.RenderHTML(&html, view); err != nil {
			return err
		}

		if htmlOnly {
			if err := os.WriteFile(outPath, html.Bytes(), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", outPath, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), outPath)
			return nil
		}

		// The HTML lands beside the PDF if any of what follows fails, so a
		// missing browser or a browser that dies mid-print never costs the
		// resolve that produced it.
		keepHTML := func(cause error) error {
			// An interrupt is not a failure to recover from: the user said
			// stop, so stop rather than leaving a file they did not ask for.
			if errors.Is(cause, context.Canceled) {
				return errors.New("interrupted")
			}
			rescued := filepath.Join(dir, slug+".html")
			if err := os.WriteFile(rescued, html.Bytes(), 0o644); err != nil {
				return errors.Join(cause, fmt.Errorf("writing %s: %w", rescued, err))
			}
			return fmt.Errorf("%w\n\nThe HTML was still written to:\n  %s", cause, rescued)
		}

		// Located before anything is written, for the same reason the logo is
		// prepared before the output file is created.
		browser, err := generation.FindBrowser(cfg.PDF.Browser)
		if err != nil {
			return keepHTML(err)
		}

		width, height, margin, err := cfg.PDF.Geometry()
		if err != nil {
			return keepHTML(err)
		}
		footer, err := generation.RenderFooter(view)
		if err != nil {
			return keepHTML(err)
		}

		pdf, err := generation.RenderPDF(cmd.Context(), browser, html.Bytes(), generation.PDFOptions{
			PaperWidth:  width,
			PaperHeight: height,
			Margin:      margin,
			Footer:      footer,
		})
		if err != nil {
			return keepHTML(err)
		}

		if err := os.WriteFile(outPath, pdf, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), outPath)
		return nil
	},
}

// extension is the default suffix for the requested output form.
func extension(htmlOnly bool) string {
	if htmlOnly {
		return ".html"
	}
	return ".pdf"
}

func init() {
	documentCmd.AddCommand(generateCmd)

	generateCmd.Flags().StringP("out", "o", "",
		"Output path (default: the document's directory)")
	generateCmd.Flags().Bool("html", false,
		"Write the intermediate HTML instead of a PDF")
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
