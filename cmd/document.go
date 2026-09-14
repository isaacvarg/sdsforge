package cmd

import (
	"github.com/spf13/cobra"
)

var documentCmd = &cobra.Command{
	Use: "document",
	// "documents" is what people type.
	Aliases: []string{"documents", "doc", "docs"},
	Short:   "Create, edit and render safety data sheets",
	Long: `A document holds one product's data -- name, hazard codes, section overrides --
in a document.yaml, plus the versions issued from it.

    create    start a new document
    list      list every document, by id
    edit      open a document's document.yaml in your editor
    classify  show what its hazard codes produce
    generate  render it into a PDF
    path      print the directory holding its files
    version   record and inspect issued revisions

Most subcommands take a document id, which 'list' and 'create' report.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(documentCmd)
}
