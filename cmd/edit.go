package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/isaacvarg/sdsforge/internal/launch"
	"github.com/spf13/cobra"
)

// editCmd opens a document's live document.yaml in the user's editor.
//
// 'document create' prints a path five segments deep in the XDG data directory
// that nobody memorises, and every edit afterwards starts with finding it again.
//
// The parse check on the way out is the reason this waits for the editor rather
// than just launching it: YAML that no longer loads is worth hearing about while
// the change is still fresh, not at the moment someone tries to issue a version.
var editCmd = &cobra.Command{
	Use:     "edit <document-id>",
	Short:   "Open a document's document.yaml in your editor",
	Aliases: []string{"e"},
	Long: `Open the live document.yaml for editing and wait for the editor to close.

The editor is $VISUAL, then $EDITOR, then a built-in default, unless 'command'
under [edit] in the config file names one. Run 'sdsforge config show' to see
which one will actually open.

Afterwards the file is re-read and any parse error is reported. Nothing else is
written: editing a document changes nothing that has already been issued, so run
'document version create' when the result is ready to go out.

What else happens after a successful edit is configurable:

    [edit]
    classify = true    show what the hazard codes now produce
    generate = true    re-render the PDF

--classify and --generate override those for a single run, in either direction,
so --classify=false suppresses a setting you have turned on.`,
	Example: `  sdsforge document edit 1
  sdsforge document edit 1 --classify`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := documentID(args[0])
		if err != nil {
			return err
		}
		dir, err := documentDir(id)
		if err != nil {
			return err
		}

		path := filepath.Join(dir, document.DocumentFile)
		// Checked BEFORE the editor runs. An editor opening an empty buffer
		// looks exactly like a document with nothing in it, and saving that
		// would write a file the rest of sdsforge cannot read.
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf(
					"document %d has no live document.yaml at %s\n"+
						"restore one:  sdsforge document version restore %d <version>", id, path, id)
			}
			return fmt.Errorf("checking %s: %w", path, err)
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		editor, err := launch.Editor(cfg.Edit)
		if err != nil {
			return err
		}
		minDuration, err := cfg.Edit.MinDurationValue()
		if err != nil {
			return err
		}
		classify, err := boolFlagOr(cmd, "classify", cfg.Edit.Classify)
		if err != nil {
			return err
		}
		generate, err := boolFlagOr(cmd, "generate", cfg.Edit.Generate)
		if err != nil {
			return err
		}

		// The editor inherits the caller's working directory rather than the
		// document's: an editor's own configuration, and its version control
		// integration, are usually anchored to where the user started.
		editor = editor.With(path)
		started := time.Now()
		if err := editor.Run(""); err != nil {
			return err
		}
		if elapsed := time.Since(started); minDuration > 0 && elapsed < minDuration {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"warning: %s returned after %s\n"+
					"  Nothing may have been saved yet. A graphical editor that hands off to an\n"+
					"  already-running window needs a flag to wait, e.g. EDITOR=\"code --wait\".\n"+
					"  Set edit.min_duration = \"0\" to stop saying this.\n",
				editor.Path, elapsed.Round(time.Millisecond))
		}

		if _, err := document.Load(id); err != nil {
			// Said explicitly because the natural fear on seeing a parse error
			// is that the work was thrown away. It was not; the file on disk is
			// whatever the editor wrote.
			return fmt.Errorf(
				"%w\n\nYour edit was saved. Fix it and run:  sdsforge document edit %d", err, id)
		}

		// Both are skipped on a parse failure above: there is nothing to
		// classify or render from a document that will not load.
		if classify {
			if err := runClassify(cmd, id); err != nil {
				return err
			}
		}
		if generate {
			if err := runGenerate(cmd, id, false, ""); err != nil {
				return err
			}
		}
		return nil
	},
}

// boolFlagOr reads a bool flag, falling back to a configured default when the
// flag was not given.
//
// The config value cannot simply BE the flag's default: cobra builds these
// commands in init(), long before any config file has been read. Consulting
// Changed instead is what keeps --flag=false meaningful for someone who turned
// the setting on in their config.
func boolFlagOr(cmd *cobra.Command, name string, fallback bool) (bool, error) {
	if !cmd.Flags().Changed(name) {
		return fallback, nil
	}
	return cmd.Flags().GetBool(name)
}

func init() {
	documentCmd.AddCommand(editCmd)

	editCmd.Flags().BoolP("classify", "c", false,
		"Show what the hazard codes produce after editing (default: edit.classify)")
	editCmd.Flags().BoolP("generate", "g", false,
		"Re-render the PDF after editing (default: edit.generate)")
}
