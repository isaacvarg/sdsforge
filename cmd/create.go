package cmd

import (
	"fmt"

	document "github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		var providedName string
		hasArgs := len(args) != 0
		if len(args) > 1 {
			fmt.Println("too many argmuments")
			return
		}
		if hasArgs {
			providedName = args[0]
		}

		name, err := cmd.Flags().GetString("name")
		if err != nil {
			fmt.Println("error getting flags")
			return
		}
		if name == "" && !hasArgs {
			fmt.Println("a name must be provided")
			return
		}

		if name != "" {
			providedName = name
		}

		doc := document.Data{
			ProductName: providedName,
		}

		result, err := document.Save(doc)
		if err != nil {
			fmt.Println("error saving document yaml")
			fmt.Println(err)
			return
		}

		fmt.Println(result)
	},
}

func init() {
	documentCmd.AddCommand(createCmd)

	createCmd.Flags().StringP("name", "n", "", "Name of the product or material the document is for")
}
