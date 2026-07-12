package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags
var quiet bool

var rootCmd = &cobra.Command{
	SilenceErrors: true,
	SilenceUsage:  true,
	Use:           "flipkart-cli",
	Short:         "CLI for Flipkart operations",
	Long: `flipkart-cli — Personal CLI for Flipkart account operations.

Authentication is stored in ~/.config/flipkart-cli/config.json
(cookies extracted from browser session via 'flipkart-cli auth setup').

Examples:
  flipkart-cli auth setup
  flipkart-cli auth show
  flipkart-cli giftcard list
  flipkart-cli giftcard add --card-number 6000170910944181 --card-pin 149232
  flipkart-cli giftcard add --card-number 6000170910944181 --card-pin 149232
`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false,
		"Suppress progress messages to stderr")

	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(giftcardCmd)
}

// --- helpers ---

func info(msg string) {
	if !quiet {
		fmt.Fprintln(os.Stderr, msg)
	}
}

func infof(f string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, f+"\n", args...)
	}
}

const grey = "\033[90m"
const reset = "\033[0m"
const green = "\033[32m"
const red = "\033[31m"

func infofGrey(f string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, grey+f+reset+"\n", args...)
	}
}

func infofGreen(f string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, green+f+reset+"\n", args...)
	}
}

func infofRed(f string, args ...interface{}) {
	if !quiet {
		fmt.Fprintf(os.Stderr, red+f+reset+"\n", args...)
	}
}
