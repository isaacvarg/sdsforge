package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List every document, by id",
	Long: `Print every document's id and name, oldest first.

The id is what every other document command takes as its argument, but the
whole of it is rarely needed: a leading piece of an id, or the product name,
works anywhere an id does, as long as it picks out only one document. So all
of these reach the same sheet:

    sdsforge document edit 01K6H3PZ8Q7XN4V2R9BKTC5M0E
    sdsforge document edit 01K6H3
    sdsforge document edit suspension-shower-gel
    sdsforge document edit "suspension shower"

The listing is read from the documents directory itself rather than from an
index file, so it says what is really there.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := document.List()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("no documents yet -- run 'sdsforge document create'")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tCREATED\tNAME")
		for _, entry := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				entry.ID,
				entry.ID.Time().Format("2006-01-02"),
				entry.Display())
		}
		return w.Flush()
	},
}

func init() {
	documentCmd.AddCommand(listCmd)
}
