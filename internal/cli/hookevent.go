package cli

import (
	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/event"
)

// hookEventCmd is the hidden command the git hook shims run in the background.
const hookEventCmd = "hook-event"

// runHookEvent plays the event's sound if configured. It never fails and
// never prints: whatever goes wrong, git must not notice.
func runHookEvent(args []string) {
	if len(args) != 1 {
		return
	}
	p, err := config.ResolvePaths()
	if err != nil {
		return
	}
	_, _ = event.Play(p, args[0], event.Options{})
}

func newHookEventCmd() *cobra.Command {
	return &cobra.Command{
		Use:                hookEventCmd + " <commit|push>",
		Short:              "Internal: called by jingle's git hooks",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			runHookEvent(args)
			return nil
		},
	}
}
