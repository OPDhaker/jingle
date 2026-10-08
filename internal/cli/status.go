package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/doctor"
	"github.com/OPDhaker/jingle/internal/output"
)

func newStatusCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show install state, settings, player, and problems",
		Long: `Report everything needed to decide what to do next: whether jingle is
installed, the global/system/previous core.hooksPath, the effective config,
the detected audio player, the git version, and any problems (each with a
severity and a fix). Always exits 0 when it can report; check "problems" for
issues, or use "jingle doctor", which also checks the current repo and exits
1 when something needs fixing.`,
		Example: `  jingle status
  jingle status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hooksEnv()
			if err != nil {
				return err
			}
			r := doctor.Check(env, "")
			return printer(cmd).Result(r, func(w io.Writer) { printReport(w, r) })
		},
	}
}

func printReport(w io.Writer, r doctor.Report) {
	yn := map[bool]string{true: "yes", false: "no"}
	fmt.Fprintf(w, "installed:  %s\n", yn[r.Installed])
	fmt.Fprintf(w, "hooks dir:  %s\n", r.HooksDir)
	if r.GlobalHooksPath != nil {
		fmt.Fprintf(w, "global core.hooksPath: %s\n", *r.GlobalHooksPath)
	}
	if r.PreviousHooksPath != nil {
		fmt.Fprintf(w, "previous core.hooksPath: %s\n", *r.PreviousHooksPath)
	}
	fmt.Fprintf(w, "config:     %s\n", r.ConfigPath)
	for _, k := range config.Keys {
		if v, ok := r.Config[k]; ok {
			fmt.Fprintf(w, "  %s = %v\n", k, v)
		}
	}
	if r.Player != nil {
		fmt.Fprintf(w, "player:     %s (%s)\n", r.Player.Name, r.Player.Path)
	}
	if r.Git != nil {
		fmt.Fprintf(w, "git:        %s (%s)\n", r.Git.Version, r.Git.Path)
	}
	if r.Repo != nil {
		fmt.Fprintf(w, "repo:       %s\n", r.Repo.Path)
		if r.Repo.HooksPath != nil {
			fmt.Fprintf(w, "  core.hooksPath = %s (%s)\n", *r.Repo.HooksPath, r.Repo.HooksPathScope)
		}
	}
	if len(r.Problems) == 0 {
		fmt.Fprintln(w, "problems:   none")
		return
	}
	fmt.Fprintln(w, "problems:")
	for _, p := range r.Problems {
		fmt.Fprintf(w, "  %-7s %s: %s\n", p.Severity, p.Code, p.Message)
		if p.Fix != "" {
			fmt.Fprintf(w, "          fix: %s\n", p.Fix)
		}
	}
}
