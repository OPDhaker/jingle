// Package cli is the thin cobra layer over jingle's core packages.
//
// Convention: every RunE returns nil or an *output.Error. Any other error can
// only come from cobra's argument parsing, so it is reported as a usage error.
package cli

import (
	"io"
	"os"
	"slices"
	"strings"

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
  jingle play commit --wait      # built-in sounds play with no files supplied
  jingle config set commit.sounds ~/Music/tag.mp3   # optional: your own tag
  jingle doctor

Every command supports --json for machine-readable output. Errors go to
stderr; with --json as {"error": {"code": "...", "message": "..."}}.
"jingle schema" describes every command, flag, config key, and code as JSON;
"jingle skill" prints step-by-step setup instructions for AI agents.

Exit codes:
  0  requested state reached (including "unchanged")
  1  error
  2  bad command, flag, argument, or missing --yes

` + wrap("Error codes: "+strings.Join(errorCodeNames(), ", "), 76),
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
		newPreviewCmd(printer),
		newStatusCmd(printer),
		newDoctorCmd(printer),
		newVersionCmd(printer),
		newSchemaCmd(),
		newSkillCmd(printer),
		newHookEventCmd(),
	)
	return root
}

func errorCodeNames() []string {
	names := make([]string, len(output.ErrorCodes))
	for i, c := range output.ErrorCodes {
		names[i] = c.Code
	}
	return names
}

// wrap breaks s into lines of at most width runes, at spaces.
func wrap(s string, width int) string {
	var b strings.Builder
	n := 0
	for i, w := range strings.Fields(s) {
		if i > 0 && n+1+len(w) > width {
			b.WriteByte('\n')
			n = 0
		} else if i > 0 {
			b.WriteByte(' ')
			n++
		}
		b.WriteString(w)
		n += len(w)
	}
	return b.String()
}
