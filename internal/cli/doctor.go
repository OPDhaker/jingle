package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/doctor"
	"github.com/OPDhaker/jingle/internal/output"
)

func newDoctorCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Find everything that would keep jingle silent, with fixes",
		Long: `Check the install, config, sound files, audio player, git version, and
the repo in the current directory (or --repo), and report each problem with
a severity and a fix. The report has the same fields as "jingle status",
plus "repo" when a repo was checked.

Problems with severity "error" stop sounds; "warning" means sounds play but
not as configured (e.g. push plays on attempt rather than on success).

The repo check catches a repo-level core.hooksPath (husky and similar tools
set one), which overrides jingle's global hooks in that repo. Outside a repo
it is skipped, unless --repo is given.

Exit codes: 0 when there are no errors (warnings allowed); 1 when at least
one error was found (code "problems_found"; the report is still printed on
stdout), or --repo is not a git repository (code "not_a_repo").`,
		Example: `  jingle doctor
  jingle doctor --json
  jingle doctor --repo ~/code/app --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hooksEnv()
			if err != nil {
				return err
			}
			dir, explicit := repo, cmd.Flags().Changed("repo")
			if !explicit {
				if dir, err = os.Getwd(); err != nil {
					dir = ""
				}
			}
			root := ""
			if dir != "" {
				r, ok, err := env.Git.RepoRoot(dir)
				switch {
				case err == nil && ok:
					root = r
				case explicit && err != nil:
					return gitError("git_failed", err)
				case explicit:
					return output.Errorf("not_a_repo", output.ExitError, "%s is not inside a git repository", dir)
				}
			}
			r := doctor.Check(env, root)
			if err := printer(cmd).Result(r, func(w io.Writer) { printReport(w, r) }); err != nil {
				return err
			}
			if n := r.Errors(); n > 0 {
				return output.Errorf("problems_found", output.ExitError, "%d problem(s) need fixing", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo to check (default: the current directory, if it is a repo)")
	return cmd
}
