package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/builtin"
	"github.com/OPDhaker/jingle/internal/event"
	"github.com/OPDhaker/jingle/internal/output"
)

func newPreviewCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "preview <builtin:name|file>",
		Short: "Play any sound, to audition it before adding it",
		Long: `Play a built-in sound or an audio file at the configured volume,
without adding it to an event. Nothing is changed: no config, no cooldown.
Use "play" to hear the sounds an event already has.

By default the player starts in the background. Use --wait to block until
playback ends and see player errors.`,
		Example: `  jingle preview builtin:boom
  jingle preview ~/Music/tag.mp3 --wait
  jingle preview builtin:fanfare --json`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: builtin.Entries(),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := paths()
			if err != nil {
				return err
			}
			res, err := event.Preview(p, args[0], wait)
			if err != nil {
				var e *event.Error
				if errors.As(err, &e) {
					exit := output.ExitError
					if e.Code == event.CodeInvalidValue {
						exit = output.ExitUsage
					}
					return output.Errorf(e.Code, exit, "%s", e.Message)
				}
				return output.Errorf("player_failed", output.ExitError, "%v", err)
			}
			return printer(cmd).Result(res, func(w io.Writer) {
				fmt.Fprintf(w, "played %s (%s)\n", res.File, res.Player.Name)
			})
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for playback to finish")
	return cmd
}
