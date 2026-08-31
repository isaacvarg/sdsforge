package cmd

import (
	"github.com/spf13/cobra"
)

var documentCmd = &cobra.Command{
	Use:   "document",
	Short: "A document is grouped data that gets generated into a PDF.",
	Long: `A document is grouped data that gets generated into a PDF.

`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(documentCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// itemCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// itemCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
