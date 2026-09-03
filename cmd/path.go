package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// pathCmd prints where a document's files live.
//
// This is the half of 'cd' that composes. A program cannot move the shell that
// ran it, so 'cd' starts a new one; a path on stdout can be substituted into
// the caller's own cd, or into a script that wants the PDF.
var pathCmd = &cobra.Command{
	Use:     "path [document-id]",
	Aliases: []string{"dir"},
	Short:   "Print the directory holding a document's files",
	Long: `Print the path to one document's directory, or to the directory holding all of
them when no id is given.

Nothing is written; this only reports. Use it to move your current shell, which
'sdsforge cd' cannot do:

    cd "$(sdsforge document path 1)"`,
	Example: `  sdsforge document path
  sdsforge document path 1`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, _, err := targetDir(args)
		if err != nil {
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), dir)
		return nil
	},
}

func init() {
	documentCmd.AddCommand(pathCmd)
}
