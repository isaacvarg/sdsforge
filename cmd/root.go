// Package cmd
// cli command form cobra
package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sdsforge",
	Short: "Generate and version GHS Safety Data Sheets",
	Long: `SDS Forge stores product data as YAML, resolves it against a content library of
GHS-compliant section text, and renders the result as a PDF Safety Data Sheet.

    sdsforge document create "Acme Degreaser"   start a new document
    sdsforge document edit 1                    fill it in
    sdsforge document generate 1                preview the PDF
    sdsforge document version create 1 --minor  issue it for real

Run 'sdsforge config init' first to record company and emergency contact
details, so they need not be typed into every document. 'sdsforge sections
list' shows what the content library can fill in, and 'sdsforge schema'
gives your editor completion for document.yaml.`,
	// Usage text belongs on an argument mistake, not on a runtime failure.
	// Without this, a resolve error is buried under a wall of flag help.
	SilenceUsage: true,
	// Setting this gives cobra's --version flag, and only that flag: a
	// 'version' command here would sit beside 'document version', which
	// means something else entirely.
	Version: resolveVersion(),
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	// The context reaches commands as cmd.Context(). Generating a sheet starts
	// a headless browser; without this, Ctrl-C would leave it running.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.sdsforge.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	rootCmd.SetVersionTemplate(versionLine())
}
