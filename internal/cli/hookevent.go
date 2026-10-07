package cli

import (
	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/event"
)

// hookEventCmd is the hidden command the git hook shims run in the background.
const hookEventCmd = "hook-event"

// runHookEvent handles one call from a shim. It never fails and never
// prints: whatever goes wrong, git must not notice.
//
//	commit                        post-commit: play the commit sound
//	pre-push <pid> <remote> <url> pre-push marked push <pid> (see event.PrePush)
//	push-done <pid>               push <pid> updated a remote-tracking ref
//	push                          shims from before success detection: play now
func runHookEvent(args []string) {
	if len(args) == 0 {
		return
	}
	p, err := config.ResolvePaths()
	if err != nil {
		return
	}
	switch {
	case args[0] == "pre-push" && len(args) == 4:
		_, _ = event.PrePush(p, args[1], args[2], args[3])
	case args[0] == "push-done" && len(args) == 2:
		_, _ = event.PushDone(p, args[1])
	case len(args) == 1:
		_, _ = event.Play(p, args[0], event.Options{})
	}
}

func newHookEventCmd() *cobra.Command {
	return &cobra.Command{
		Use:                hookEventCmd + " <commit|pre-push|push-done> [args]",
		Short:              "Internal: called by jingle's git hooks",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(_ *cobra.Command, args []string) error {
			runHookEvent(args)
			return nil
		},
	}
}
