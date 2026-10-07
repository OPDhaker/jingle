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
	// Hooks call this on every commit/push: skip cobra, never fail, never print.
	if len(args) > 0 && args[0] == hookEventCmd {
		runHookEvent(args[1:])
		return output.ExitOK
	}
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

Typical setup:
  jingle install --yes
  jingle config set commit.sound ~/Music/tag.mp3
  jingle play commit
  jingle status

Every command supports --json for machine-readable output. Errors go to
stderr; with --json as {"error": {"code": "...", "message": "..."}}.

Exit codes:
  0  requested state reached (including "unchanged")
  1  error
  2  bad command, flag, argument, or missing --yes

Error codes: usage, confirmation_required, invalid_key, invalid_value,
unknown_event, git_not_found, install_failed, uninstall_failed,
config_invalid, sound_not_found, no_sound, sound_missing, no_player,
player_failed, paths_unavailable, io_error`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(jsonMode, "json", false, "print machine-readable JSON")
	root.CompletionOptions.HiddenDefaultCmd = true

	printer := func(cmd *cobra.Command) *output.Printer {
		return output.New(*jsonMode, cmd.OutOrStdout(), cmd.ErrOrStderr())
	}
	root.AddCommand(
		newInstallCmd(printer),
		newUninstallCmd(printer),
		newConfigCmd(printer),
		newPlayCmd(printer),
		newStatusCmd(printer),
		newVersionCmd(printer),
		newHookEventCmd(),
	)
	return root
}
