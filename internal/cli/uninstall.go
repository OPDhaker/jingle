package cli

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/output"
)

func newUninstallCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	var yes, dryRun bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Turn off jingle everywhere and restore your git config",
		Long: `Remove jingle's global hooks and restore the global core.hooksPath that was
set before "jingle install" (or unset it if there was none). If install
created the [core] section or the global gitconfig file, those are removed
too, so the file ends up byte-for-byte as before.

If something else changed core.hooksPath after install, it is left alone.
Your config and imported sounds are kept, so a later install picks them up.

Requires --yes (or --dry-run to preview). Safe to re-run: reports "unchanged".`,
		Example: `  jingle uninstall --dry-run --json
  jingle uninstall --yes
  jingle uninstall --yes --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes && !dryRun {
				return output.Errorf("confirmation_required", output.ExitUsage,
					"uninstall changes your global git config; pass --yes to confirm, or --dry-run to preview")
			}
			env, err := hooksEnv()
			if err != nil {
				return err
			}
			res, err := hooks.Uninstall(env, dryRun)
			if err != nil {
				return gitError("uninstall_failed", err)
			}
			return printer(cmd).Result(res, func(w io.Writer) { printChanges(w, res) })
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm changing global git config")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would change without changing anything")
	return cmd
}
