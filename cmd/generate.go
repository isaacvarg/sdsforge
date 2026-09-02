package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isaacvarg/sdsforge/internal/document"
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
preset or overrides individual subsections.

The sheet is written into the document's directory and overwritten on every run.
To keep an issue of it, record a version instead:

    sdsforge document version create <document-id> --minor -m "what changed"`,
	Args: cobra.ExactArgs(1),
	// RunE rather than Run: returning the error lets cobra print it and set a
	// non-zero exit status, which matters for scripting.
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}

		doc, versions, err := loadForRender(id)
		if err != nil {
			return err
		}

		htmlOnly, err := cmd.Flags().GetBool("html")
		if err != nil {
			return err
		}

		dir, err := document.Dir(id)
		if err != nil {
			return err
		}

		warnIfDraft(id, versions, cmd.ErrOrStderr())

		built, err := buildSheet(cmd.Context(), id, doc, versions, cmd.ErrOrStderr(), !htmlOnly)

		outPath, flagErr := cmd.Flags().GetString("out")
		if flagErr != nil {
			return flagErr
		}
		if outPath == "" {
			outPath = filepath.Join(dir, built.Slug+extension(htmlOnly))
		}

		if err != nil {
			// The HTML lands beside the PDF if the print stage failed, so a
			// missing browser or a browser that dies mid-print never costs the
			// resolve that produced it.
			//
			// An interrupt is not a failure to recover from: the user said
			// stop, so stop rather than leaving a file they did not ask for.
			if built.HTML == nil || errors.Is(err, context.Canceled) {
				if errors.Is(err, context.Canceled) {
					return errors.New("interrupted")
				}
				return err
			}
			rescued := filepath.Join(dir, built.Slug+".html")
			if writeErr := os.WriteFile(rescued, built.HTML, 0o644); writeErr != nil {
				return errors.Join(err, fmt.Errorf("writing %s: %w", rescued, writeErr))
			}
			return fmt.Errorf("%w\n\nThe HTML was still written to:\n  %s", err, rescued)
		}

		content := built.PDF
		if htmlOnly {
			content = built.HTML
		}
		if err := os.WriteFile(outPath, content, 0o644); err != nil {
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
