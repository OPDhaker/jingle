package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/event"
	"github.com/OPDhaker/jingle/internal/output"
)

func newPlayCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "play <commit|push>",
		Short: "Play an event's sound now, to test setup",
		Long: `Play the sound configured for an event, without committing or pushing.
Plays even if the event (or jingle) is turned off; the result reports
"enabled" so you can tell whether it would play on a real commit or push.

By default the player starts in the background, as it does from a git hook.
Use --wait to block until playback ends and see player errors.`,
		Example: `  jingle play commit
  jingle play push --wait --json`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"commit", "push"},
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths()
			if err != nil {
				return err
			}
			res, err := event.Play(p, args[0], event.Options{IgnoreEnabled: true, Wait: wait})
			if err != nil {
				var e *event.Error
				if errors.As(err, &e) {
					exit := output.ExitError
					if e.Code == event.CodeUnknownEvent {
						exit = output.ExitUsage
					}
					return output.Errorf(e.Code, exit, "%s", e.Message)
				}
				return output.Errorf("player_failed", output.ExitError, "%v", err)
			}
			return printer(cmd).Result(res, func(w io.Writer) {
				fmt.Fprintf(w, "played %s: %s (%s)\n", res.Event, res.Sound, res.Player.Name)
				if !res.Enabled {
					fmt.Fprintf(w, "note: the %s sound is turned off, so it won't play on a real %s\n", res.Event, res.Event)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for playback to finish")
	return cmd
}
