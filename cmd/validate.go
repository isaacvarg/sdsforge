package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/schema"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate <document-id>",
	Short: "Check a document for errors without rendering it",
	Long: `Check a document.yaml the two ways that matter, and report everything wrong
with it rather than stopping at the first problem.

First against the JSON Schema -- the same one 'sdsforge schema' prints, built
from your configured content library, custom layer included. This catches what
generate would not: a misspelled key is silently ignored when a document is
read, so its content simply never reaches the sheet.

Then by resolving and rendering the document exactly as generate does, stopping
short of printing. That catches everything else that would make generate fail,
and needs no browser.

Exits non-zero when anything is wrong, so it can gate a script or CI job.

    sdsforge document generate <document-id> --dry-run

does the same.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}
		return runValidate(cmd, id)
	},
}

// runValidate checks one document against the schema and then dry-runs its
// render. Shared by 'document validate' and 'document generate --dry-run'.
func runValidate(cmd *cobra.Command, id document.ID) error {
	dir, err := documentDir(id)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, document.DocumentFile)

	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading document %s: %w", id, err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lib, err := openLibraryWith(cfg)
	if err != nil {
		return err
	}

	var failures []string

	problems, err := schema.Validate(lib, raw)
	if err != nil {
		// The YAML does not parse, so there is nothing to render either.
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(problems) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "schema: %d problem(s)", len(problems))
		for _, p := range problems {
			fmt.Fprintf(&b, "\n  %s:%d:%d: %s: %s", path, p.Line, p.Column, p.Path, p.Message)
		}
		failures = append(failures, b.String())
	}

	// Run regardless of the schema result: a render failure is a separate
	// problem, and the user should hear about both at once.
	if err := dryRunGenerate(cmd, id, cmd.ErrOrStderr()); err != nil {
		failures = append(failures, "generate: "+indent(err.Error()))
	}

	if len(failures) > 0 {
		return fmt.Errorf("document %s has problems:\n\n%s", id, strings.Join(failures, "\n\n"))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "document %s OK (layers: %s)\n", id, strings.Join(lib.Layers(), ", "))
	return nil
}

// dryRunGenerate runs the render pipeline up to, but not including, printing,
// and throws the result away.
func dryRunGenerate(cmd *cobra.Command, id document.ID, warn io.Writer) error {
	doc, versions, err := loadForRender(id)
	if err != nil {
		return err
	}
	_, err = buildSheet(cmd.Context(), id, doc, versions, warn, false)
	return err
}

// indent continues a multi-line message under its heading.
func indent(s string) string {
	return strings.ReplaceAll(s, "\n", "\n  ")
}

func init() {
	documentCmd.AddCommand(validateCmd)
}
