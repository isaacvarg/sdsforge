package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Aliases: []string{"versions"},
	Short:   "Record and inspect issued revisions of a document",
	Long: `A version is one issued revision of a document, archived so it can be produced
again exactly as it went out.

Recording a version snapshots the document.yaml together with the HTML and PDF
generated from it into a timestamped directory under the document's folder, and
adds a row to that document's version history. The history is also what fills
section 16's revision table and the version number in the sheet's header, so
what a sheet claims was issued is always what was actually archived.

Editing document.yaml changes nothing that has already been issued; run
'version create' when you are ready to issue the result.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var versionListCmd = &cobra.Command{
	Use:     "list <document-id>",
	Aliases: []string{"ls"},
	Short:   "List a document's issued versions, oldest first",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}

		index, err := document.LoadVersions(id)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if len(index.Versions) == 0 {
			fmt.Fprintf(out, "document %d has no versions yet\n", id)
			return nil
		}

		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		defer w.Flush()

		fmt.Fprintln(w, "ID\tVERSION\tISSUED\tARTIFACTS\tMEMO")
		for _, ver := range index.Versions {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				ver.ID,
				ver.Label,
				ver.Timestamp.Format("2006-01-02 15:04 MST"),
				artifactSummary(ver),
				dashIfEmpty(ver.Memo),
			)
		}
		return nil
	},
}

var versionCreateCmd = &cobra.Command{
	Use:   "create <document-id>",
	Short: "Issue a new version of a document",
	Long: `Render the document as it stands and archive it as a new version.

Give the version number either explicitly with --label, or as a bump of the
latest one with --major, --minor or --patch. There is no default: what a version
number means is a judgement about the change, not something to guess at.

    --patch   a correction that does not change the hazard information
    --minor   new or reworded content
    --major   a reclassification, or a new product identity

The memo is required. It becomes the description column of section 16's revision
history table on the sheet itself, where an empty cell is not acceptable.

Printing needs a Chrome-based browser; nothing is recorded if it fails.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}

		memo, err := cmd.Flags().GetString("memo")
		if err != nil {
			return err
		}

		doc, index, err := loadForRender(id)
		if err != nil {
			return err
		}

		label, err := resolveLabel(cmd, index)
		if err != nil {
			return err
		}

		// Drafted and rendered before anything is written, so the archived
		// sheet carries its own row in the revision history and its own number
		// in the header -- and so a failed render leaves nothing to undo.
		pending := index.Draft(label, memo, time.Now())

		built, err := buildSheet(cmd.Context(), id, doc, index.WithPending(pending),
			cmd.ErrOrStderr(), true)
		if err != nil {
			return err
		}

		// The live file is archived as it is on disk rather than re-marshalled
		// from the parsed document: the scaffold's comments are most of what a
		// person wrote, and a restore has to give them back.
		dir, err := document.Dir(id)
		if err != nil {
			return err
		}
		source, err := os.ReadFile(filepath.Join(dir, document.DocumentFile))
		if err != nil {
			return fmt.Errorf("reading document %d: %w", id, err)
		}

		files := map[string][]byte{
			document.DocumentFile: source,
			built.Slug + ".html":  built.HTML,
			built.Slug + ".pdf":   built.PDF,
		}
		if err := document.CommitVersion(id, pending, index, files); err != nil {
			return err
		}

		verDir, err := document.VersionDir(id, pending)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "recorded version %s of document %d\n%s\n",
			pending.Label, id, verDir)
		return nil
	},
}

var versionShowCmd = &cobra.Command{
	Use:   "show <document-id> <version>",
	Short: "Show one version and where its files are",
	Long: `Print one version's details and the path of every file archived with it.

The version may be given as its number ("1.1.0") or its id ("2").`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}

		index, err := document.LoadVersions(id)
		if err != nil {
			return err
		}
		ver, err := index.Find(args[1])
		if err != nil {
			return fmt.Errorf("document %d: %w", id, err)
		}
		verDir, err := document.VersionDir(id, ver)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "version %s  (id %d)\n", ver.Label, ver.ID)
		fmt.Fprintf(out, "issued:  %s\n", ver.Timestamp.Format("2006-01-02 15:04 MST"))
		fmt.Fprintf(out, "memo:    %s\n", dashIfEmpty(ver.Memo))
		fmt.Fprintln(out, "files:")
		if len(ver.Artifacts) == 0 {
			fmt.Fprintln(out, "  (none recorded)")
			return nil
		}
		for _, name := range ver.Artifacts {
			fmt.Fprintf(out, "  %s\n", filepath.Join(verDir, name))
		}
		return nil
	},
}

var versionRestoreCmd = &cobra.Command{
	Use:   "restore <document-id> <version>",
	Short: "Put an archived document.yaml back as the live one",
	Long: `Copy an archived version's document.yaml over the document's live one, so you
can carry on editing from an earlier issue.

Restoring does NOT record a version of its own -- it returns the working file to
a known state, and you issue the result with 'version create' when it is ready.

It refuses to run when the live file has edits that no version holds, since
those would be lost with nothing to recover them from. Record them first, or
pass --force to discard them.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}

		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}

		index, err := document.LoadVersions(id)
		if err != nil {
			return err
		}
		ver, err := index.Find(args[1])
		if err != nil {
			return fmt.Errorf("document %d: %w", id, err)
		}

		verDir, err := document.VersionDir(id, ver)
		if err != nil {
			return err
		}
		archived, err := os.ReadFile(filepath.Join(verDir, document.DocumentFile))
		if err != nil {
			return fmt.Errorf("reading version %s of document %d: %w", ver.Label, id, err)
		}

		dir, err := document.Dir(id)
		if err != nil {
			return err
		}
		livePath := filepath.Join(dir, document.DocumentFile)

		if !force {
			if err := checkRecorded(id, livePath, index); err != nil {
				return err
			}
		}

		if err := os.WriteFile(livePath, archived, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", livePath, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(),
			"restored version %s of document %d to:\n%s\n\nIssue it with:  sdsforge document version create %d --patch -m \"...\"\n",
			ver.Label, id, livePath, id)
		return nil
	},
}

// resolveLabel works out the version number this issue should carry.
func resolveLabel(cmd *cobra.Command, index document.VersionIndex) (string, error) {
	explicit, err := cmd.Flags().GetString("label")
	if err != nil {
		return "", err
	}

	if explicit != "" {
		next, err := document.ParseSemver(explicit)
		if err != nil {
			return "", err
		}
		if index.HasLabel(next.String()) {
			return "", fmt.Errorf("version %s already exists", next)
		}
		// A version number that does not move forward makes the revision
		// history unreadable: section 16 lists versions in the order they were
		// issued, and a reader takes the last row to be the current one.
		if latest, ok := index.Latest(); ok {
			current, err := latest.Semver()
			if err != nil {
				return "", fmt.Errorf("latest version %q is not a valid version number: %w", latest.Label, err)
			}
			if !current.Less(next) {
				return "", fmt.Errorf("version %s does not come after %s, the latest", next, current)
			}
		}
		return next.String(), nil
	}

	part, err := bumpPart(cmd)
	if err != nil {
		return "", err
	}

	latest, ok := index.Latest()
	if !ok {
		// Nothing to bump from. Documents get 1.0.0 at creation, so this only
		// happens for one written before versioning existed.
		return document.InitialVersionLabel, nil
	}
	current, err := latest.Semver()
	if err != nil {
		return "", fmt.Errorf("latest version %q is not a valid version number, so it cannot be bumped; pass --label: %w",
			latest.Label, err)
	}
	return current.Bump(part).String(), nil
}

// bumpPart reads whichever of the three bump flags was given. Cobra has already
// guaranteed that exactly one of the four selectors is set.
func bumpPart(cmd *cobra.Command) (document.BumpPart, error) {
	for _, candidate := range []struct {
		flag string
		part document.BumpPart
	}{
		{"major", document.BumpMajor},
		{"minor", document.BumpMinor},
		{"patch", document.BumpPatch},
	} {
		set, err := cmd.Flags().GetBool(candidate.flag)
		if err != nil {
			return 0, err
		}
		if set {
			return candidate.part, nil
		}
	}
	return 0, fmt.Errorf("one of --label, --major, --minor or --patch is required")
}

// warnIfDraft says so when the live document.yaml no longer matches the version
// the sheet is about to be stamped with.
//
// The header and section 16 name the latest recorded version, so a sheet
// rendered from an edited document claims to be an issue it is not. That is not
// an error -- previewing an edit is exactly what 'generate' is for -- but it
// must not go out silently.
func warnIfDraft(id int, index document.VersionIndex, warn io.Writer) {
	latest, ok := index.Latest()
	if !ok {
		return
	}

	dir, err := document.Dir(id)
	if err != nil {
		return
	}
	live, err := os.ReadFile(filepath.Join(dir, document.DocumentFile))
	if err != nil {
		return
	}

	verDir, err := document.VersionDir(id, latest)
	if err != nil {
		return
	}
	archived, err := os.ReadFile(filepath.Join(verDir, document.DocumentFile))
	if err != nil {
		// Nothing to compare against, so nothing can be claimed either way.
		return
	}

	if !bytes.Equal(live, archived) {
		fmt.Fprintf(warn,
			"warning: document %d has edits that version %s does not, so this sheet is a draft\n"+
				"         issue it with:  sdsforge document version create %d --patch -m \"...\"\n",
			id, latest.Label, id)
	}
}

// checkRecorded reports an error when the live document.yaml differs from every
// archived one, which means it holds edits that no version can give back.
func checkRecorded(id int, livePath string, index document.VersionIndex) error {
	live, err := os.ReadFile(livePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", livePath, err)
	}

	for _, ver := range index.Versions {
		verDir, err := document.VersionDir(id, ver)
		if err != nil {
			return err
		}
		archived, err := os.ReadFile(filepath.Join(verDir, document.DocumentFile))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reading version %s of document %d: %w", ver.Label, id, err)
		}
		if bytes.Equal(live, archived) {
			return nil
		}
	}

	return fmt.Errorf(
		"%s has edits that no version holds; they would be lost.\n"+
			"Record them first:  sdsforge document version create %d --patch -m \"...\"\n"+
			"Or discard them:    add --force",
		livePath, id)
}

// artifactSummary names what a version archived, short enough for a table cell.
func artifactSummary(ver document.Version) string {
	var yaml, html, pdf bool
	for _, name := range ver.Artifacts {
		switch filepath.Ext(name) {
		case ".yaml", ".yml":
			yaml = true
		case ".html":
			html = true
		case ".pdf":
			pdf = true
		}
	}

	var parts []string
	if yaml {
		parts = append(parts, "yaml")
	}
	if html {
		parts = append(parts, "html")
	}
	if pdf {
		parts = append(parts, "pdf")
	}
	if len(parts) == 0 {
		return "-"
	}
	return joinPlus(parts)
}

func joinPlus(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "+" + p
	}
	return out
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func init() {
	documentCmd.AddCommand(versionCmd)
	versionCmd.AddCommand(versionListCmd)
	versionCmd.AddCommand(versionCreateCmd)
	versionCmd.AddCommand(versionShowCmd)
	versionCmd.AddCommand(versionRestoreCmd)

	versionCreateCmd.Flags().String("label", "",
		"Version number to issue, e.g. 1.2.0")
	versionCreateCmd.Flags().Bool("major", false,
		"Issue the next major version (1.4.7 -> 2.0.0)")
	versionCreateCmd.Flags().Bool("minor", false,
		"Issue the next minor version (1.4.7 -> 1.5.0)")
	versionCreateCmd.Flags().Bool("patch", false,
		"Issue the next patch version (1.4.7 -> 1.4.8)")
	versionCreateCmd.Flags().StringP("memo", "m", "",
		"What changed in this version; shown in the sheet's revision history")

	versionCreateCmd.MarkFlagsMutuallyExclusive("label", "major", "minor", "patch")
	versionCreateCmd.MarkFlagsOneRequired("label", "major", "minor", "patch")
	_ = versionCreateCmd.MarkFlagRequired("memo")

	versionRestoreCmd.Flags().Bool("force", false,
		"Overwrite the live document even when it has unrecorded edits")
}
