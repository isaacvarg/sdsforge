package cmd

import (
	"fmt"
	"os"

	"github.com/isaacvarg/sdsforge/internal/config"
	"github.com/isaacvarg/sdsforge/internal/launch"
	"github.com/spf13/cobra"
)

// cdCmd launches a shell in a document's directory.
//
// Not a command at all in the usual sense: it hands the terminal to a shell and
// waits. A process cannot change the working directory of the shell that started
// it, so rather than pretend otherwise this starts a NEW shell already in the
// right place. 'document path' is the other half, for moving the current shell.
//
// It sits on the root command rather than under 'document' because it is a
// navigation aid, not something done TO a document -- and because
// 'sdsforge cd 1' is the whole point.
var cdCmd = &cobra.Command{
	Use:   "cd [document-id]",
	Short: "Launch a shell in a document's directory",
	Long: `Start a shell with its working directory set to one document's directory, or to
the directory holding all of them when no id is given. Leave it with 'exit'.

This is a NEW shell, not a change to the one you typed in: a program cannot move
the shell that ran it. To move your current shell instead, substitute the path:

    cd "$(sdsforge document path 1)"

The shell is $SHELL, unless 'command' under [cd] in the config file names one.
It runs with SDSFORGE_SUBSHELL=1 set, plus SDSFORGE_DOCUMENT_ID when an id was
given, so a prompt can show that it is not your top-level shell.`,
	Example: `  sdsforge cd
  sdsforge cd 1`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, id, err := targetDir(args)
		if err != nil {
			return err
		}

		cfg, err := config.Load()
		if err != nil {
			return err
		}
		shell, err := launch.Shell(cfg.CD)
		if err != nil {
			return err
		}

		env := []string{"SDSFORGE_SUBSHELL=1"}
		if id != 0 {
			env = append(env, fmt.Sprintf("SDSFORGE_DOCUMENT_ID=%d", id))
		}

		if err := shell.Run(dir, env...); err != nil {
			// An interactive shell's exit status is whatever the last command
			// run inside it returned. Reporting that as an sdsforge failure --
			// which is what chezmoi's equivalent does -- means a stray 'false'
			// before 'exit' produces a spurious error. Pass the status through
			// and say nothing; a shell that could not be STARTED still returns
			// a real error below.
			//
			// os.Exit skips Execute's deferred signal cleanup, which costs
			// nothing here: the process is exiting either way.
			if code, ok := launch.ExitCode(err); ok {
				os.Exit(code)
			}
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cdCmd)
}
