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
    validate  check it for errors without rendering
    generate  render it into a PDF
    path      print the directory holding its files
    version   record and inspect issued revisions

Most subcommands take a document id, which 'list' and 'create' report. An id is
26 characters, and nowhere near all of it need be typed: a leading piece of one
works, and so does the product name, as long as it picks out a single document.

    sdsforge document edit 01K6H3PZ8Q7XN4V2R9BKTC5M0E
    sdsforge document edit 01K6H3
    sdsforge document edit suspension-shower-gel`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(documentCmd)
}
