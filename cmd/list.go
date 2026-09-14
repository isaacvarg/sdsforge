package cmd

import (
	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List every document, by id",
	Long: `Print every document's id and name, oldest first.

The id is what every other document command takes as its argument, e.g.
'sdsforge document edit 1'. Run 'sdsforge document path <id>' or
'sdsforge document version list <id>' for more about one of them.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		document.ListIndex()
	},
}

func init() {
	documentCmd.AddCommand(listCmd)
}
