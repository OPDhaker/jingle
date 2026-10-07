// Package player finds the OS audio player and starts it fully detached
// (new session, null stdio, released process) so git never waits on playback.
// macOS: afplay. Linux: pw-play, paplay, aplay, ffplay. Windows: Phase 3.
package player

import (
	"fmt"
	"os/exec"
	"strings"
)

// EnvOverride names an executable to use instead of the detected player. It
// is called as `<exe> <file>`. Tests use it; so can users with odd setups.
const EnvOverride = "JINGLE_PLAYER"

// Player is an audio player command line; the file is appended to Args.
type Player struct {
	Name string   `json:"name"`
	Path string   `json:"path"`
	Args []string `json:"args"`
}

// candidates are tried in order per OS.
var candidates = map[string][]Player{
	"darwin": {{Name: "afplay"}},
	"linux": {
		{Name: "pw-play"},
		{Name: "paplay"},
		{Name: "aplay", Args: []string{"-q"}},
		{Name: "ffplay", Args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}},
	},
}

// Detect returns the player to use on this machine.
func Detect(getenv func(string) string, lookPath func(string) (string, error), goos string) (Player, bool) {
	if exe := getenv(EnvOverride); exe != "" {
		if p, err := lookPath(exe); err == nil {
			return Player{Name: EnvOverride, Path: p, Args: []string{}}, true
		}
		return Player{}, false
	}
	for _, c := range candidates[goos] {
		if p, err := lookPath(c.Name); err == nil {
			c.Path = p
			if c.Args == nil {
				c.Args = []string{} // stable JSON: [] rather than null
			}
			return c, true
		}
	}
	return Player{}, false
}

func (p Player) command(file string) *exec.Cmd {
	args := append(append([]string{}, p.Args...), file)
	return exec.Command(p.Path, args...)
}

// Start launches the player detached and returns without waiting for it.
func Start(p Player, file string) error {
	cmd := p.command(file)
	detach(cmd)
	// nil std streams => the null device, so nothing holds git's pipes open.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Run plays file and waits for the player to finish. A failure includes the
// player's own output.
func Run(p Player, file string) error {
	out, err := p.command(file).CombinedOutput()
	if msg := strings.TrimSpace(string(out)); err != nil && msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}
