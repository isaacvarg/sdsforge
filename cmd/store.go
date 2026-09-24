package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
)

var storeCmd = &cobra.Command{
	Use:   "store",
	Short: "Maintain the document store itself",
	Long: `The store is the directory holding every document, usually shared between
colleagues as a git repository.

    migrate   move a store off the old numeric document ids`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var storeMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Move a store off the old numeric document ids",
	Long: `Rename every numerically-named document directory to a ULID, and delete the
index.yaml that used to hand those numbers out.

Numbers had to be handed out by something, and that something was a file every
create rewrote. Two people creating a document against the same shared store
both took the same number: the same directory, and a conflict in the file that
gave it to them -- every time, even for unrelated documents. Ids are generated
locally now, and the listing is scanned from the directory itself, so there is
nothing left for two people to disagree about.

Each new id is stamped with the time its document was originally authored, so
the store goes on sorting into the order the documents were really written in.

This renames every document directory at once, so run it when nobody has
uncommitted work:

    1. everyone commits and pushes what they have, then stops
    2. one person runs this, commits the renames, and pushes
    3. everyone else installs this version and pulls

Run it again afterwards and it will report nothing to do. Old numeric ids stop
working: use 'sdsforge document list' to see the new ones.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		renames, err := document.PlanMigration()
		if err != nil {
			return err
		}

		stale, err := document.HasLegacyIndex()
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		if len(renames) == 0 && !stale {
			fmt.Fprintln(out, "nothing to migrate: this store is already on ULID ids")
			return nil
		}

		if len(renames) > 0 {
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			for _, r := range renames {
				fmt.Fprintf(w, "  %s\t->  %s\t%s\t(%s)\n",
					r.From, r.To, r.At.Format("2006-01-02"), displayRenameName(r))
			}
			if err := w.Flush(); err != nil {
				return err
			}
		}

		if dryRun {
			if stale {
				fmt.Fprintln(out, "  would remove documents/index.yaml")
			}
			fmt.Fprintf(out, "\n%d document(s) would be migrated; nothing was changed\n", len(renames))
			return nil
		}

		if err := document.Migrate(renames); err != nil {
			return err
		}
		if stale {
			fmt.Fprintln(out, "  removed documents/index.yaml")
		}
		fmt.Fprintf(out, "\n%d document(s) migrated\n", len(renames))
		fmt.Fprintln(out, "commit the renames and push, then have everyone else pull")
		return nil
	},
}

// displayRenameName names a document being migrated that has no product name.
func displayRenameName(r document.Rename) string {
	if r.Name == "" {
		return "unnamed"
	}
	return r.Name
}

func init() {
	storeMigrateCmd.Flags().Bool("dry-run", false, "print what would be renamed, change nothing")
	storeCmd.AddCommand(storeMigrateCmd)
	rootCmd.AddCommand(storeCmd)
}
