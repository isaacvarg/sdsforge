package cmd

import (
	"fmt"

	document "github.com/isaacvarg/sdsforge/internal/document"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Start a new document",
	Long: `Create a new document and write its document.yaml, either as the name argument
or --name.

By default the file is scaffolded from the live content library: every
section it advertises is annotated with the presets and variants actually
available, which is the easiest way to see what can be filled in. Pass
--minimal for a bare file with just the product name, if you would rather
start from nothing.

Creating a document also issues it as version 1.0.0, archiving the
document.yaml alone -- rendering a sheet needs a browser, and a scaffold with
no hazard codes yet would only produce an empty PDF. The HTML and PDF appear
from the first real 'document version create'.

Prints the path to the new document.yaml. Every other document command
addresses the document by the id in that path, e.g. 'sdsforge document edit 1'.`,
	Example: `  sdsforge document create "Acme Degreaser"
  sdsforge document create --name "Acme Degreaser" --minimal`,
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

		minimal, err := cmd.Flags().GetBool("minimal")
		if err != nil {
			fmt.Println("error getting flags")
			return
		}

		var content []byte
		if minimal {
			doc := document.Data{ProductName: providedName}
			content, err = yaml.Marshal(doc)
			if err != nil {
				fmt.Println("error creating document yaml:", err)
				return
			}
		} else {
			// The scaffold is generated from the live content library, so the
			// sections it advertises are always the ones actually available.
			lib, err := openLibrary()
			if err != nil {
				fmt.Println("error opening content library:", err)
				return
			}
			content, err = document.Scaffold(lib, providedName)
			if err != nil {
				fmt.Println("error building document scaffold:", err)
				return
			}
		}

		id, path, err := document.Create(content)
		if err != nil {
			fmt.Println("error saving document yaml")
			fmt.Println(err)
			return
		}

		// The id is printed because it can no longer be guessed: it is minted
		// here rather than counted up, so this is the user's only sight of it
		// before 'document list'. Every command takes a unique prefix, so the
		// short form is what they will actually type.
		fmt.Println(path)
		fmt.Printf("document %s (%s)\n", id, id.Short())
	},
}

func init() {
	documentCmd.AddCommand(createCmd)

	createCmd.Flags().StringP("name", "n", "", "Name of the product or material the document is for")
	createCmd.Flags().Bool("minimal", false, "Write a bare document.yaml instead of the annotated template")
}
