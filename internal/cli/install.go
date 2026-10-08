package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/output"
)

func newInstallCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Turn on jingle for every repo (sets global core.hooksPath)",
		Long: `Install jingle's git hooks globally by pointing the global core.hooksPath
at jingle's hooks dir. Works in every repo with no per-repo setup.

Your existing hooks keep running: each jingle hook first runs the hook git
would have run without jingle (the repo's .git/hooks, or your previous global
core.hooksPath), with the same args, stdin, and exit status. The previous
core.hooksPath is saved so "jingle uninstall" can restore it exactly.

Repos that set their own core.hooksPath (husky and similar) override the
global one, so jingle does not play in them; "jingle doctor" run inside a
repo reports this.

Requires --yes (or --dry-run to preview). Safe to re-run: reports "unchanged".`,
		Example: `  jingle install --dry-run --json
  jingle install --yes
  jingle install --yes --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				return output.Errorf("confirmation_required", output.ExitUsage,
					"install changes your global git config; pass --yes to confirm, or --dry-run to preview")
			}
			env, err := hooksEnv()
			if err != nil {
				return err
			}
			res, err := hooks.Install(env, dryRun)
			if err != nil {
				return gitError("install_failed", err)
			}
			return printer(cmd).Result(res, func(w io.Writer) { printChanges(w, res) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm changing global git config")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would change without changing anything")
	return cmd
}

// printChanges is the plain rendering of an install/uninstall result.
func printChanges(w io.Writer, res hooks.Result) {
	fmt.Fprintf(w, "%s (hooks dir: %s)\n", res.Status, res.HooksDir)
	if res.PreviousHooksPath != nil {
		fmt.Fprintf(w, "previous core.hooksPath: %s\n", *res.PreviousHooksPath)
	}
	if res.ChainDir != "" {
		fmt.Fprintf(w, "chaining to hooks in: %s\n", res.ChainDir)
	}
	hookCount := map[string]int{}
	for _, c := range res.Changes {
		if c.Action == "write_hook" || c.Action == "remove_hook" {
			hookCount[c.Action]++
		}
	}
	for _, c := range res.Changes {
		if n := hookCount[c.Action]; n > 0 {
			// One line per action, not per hook file; --json lists every file.
			fmt.Fprintf(w, "  %s: %d hook scripts\n", c.Action, n)
			hookCount[c.Action] = 0
			continue
		} else if c.Action == "write_hook" || c.Action == "remove_hook" {
			continue
		}
		line := "  " + c.Action + " " + c.Target
		if c.From != "" || c.To != "" {
			line += fmt.Sprintf(" (%q -> %q)", c.From, c.To)
		}
		if c.Note != "" {
			line += " [" + c.Note + "]"
		}
		fmt.Fprintln(w, line)
	}
	for _, n := range res.Notes {
		fmt.Fprintln(w, "note: "+n)
	}
}
