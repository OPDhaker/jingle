// Package cli is the thin cobra layer over jingle's core packages.
//
// Convention: every RunE returns nil or an *output.Error. Any other error can
// only come from cobra's argument parsing, so it is reported as a usage error.
package cli

import (
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/output"
)

// Execute runs the CLI with the process arguments and returns the exit code.
func Execute() int {
	return Run(os.Args[1:], os.Stdout, os.Stderr)
}

// Run runs the CLI with explicit arguments and streams (used by tests).
func Run(args []string, stdout, stderr io.Writer) int {
	var jsonMode bool
	root := newRootCmd(&jsonMode)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if err := root.Execute(); err != nil {
		// --json may not have been parsed if parsing is what failed.
		p := output.New(jsonMode || slices.Contains(args, "--json"), stdout, stderr)
		return p.Error(err)
	}
	return output.ExitOK
}

func newRootCmd(jsonMode *bool) *cobra.Command {
	root := &cobra.Command{
		Use:   "jingle",
		Short: "Play a producer tag when you git commit or push",
		Long: `jingle plays a short audio clip (a producer tag) when you run git commit
or git push. Install it once and it works in every repo on this machine.

Every command supports --json for machine-readable output. Exit codes:
  0  requested state reached
  1  error
  2  bad command, flag, or argument`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(jsonMode, "json", false, "print machine-readable JSON")
	root.CompletionOptions.HiddenDefaultCmd = true

	printer := func(cmd *cobra.Command) *output.Printer {
		return output.New(*jsonMode, cmd.OutOrStdout(), cmd.ErrOrStderr())
	}
	root.AddCommand(newVersionCmd(printer))
	return root
}
