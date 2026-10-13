// Package player finds the OS audio player and starts it fully detached
// (new session, null stdio, released process) so git never waits on playback.
// macOS: afplay. Linux: pw-play, paplay, aplay, ffplay. Windows: not supported yet.
package player

import (
	"fmt"
	"os/exec"
	"strconv"
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
	// Volume is whether this player can play below full volume.
	Volume bool `json:"volume"`
	// volume returns the flags for a volume of 0-99 (100 adds none).
	volume func(v int) []string
}

// candidates are tried in order per OS. Volume flags: afplay and pw-play
// take a factor (1 = full), paplay 0-65536, ffplay 0-100; aplay has none.
var candidates = map[string][]Player{
	"darwin": {{Name: "afplay", volume: func(v int) []string { return []string{"-v", factor(v)} }}},
	"linux": {
		{Name: "pw-play", volume: func(v int) []string { return []string{"--volume", factor(v)} }},
		{Name: "paplay", volume: func(v int) []string { return []string{"--volume", strconv.Itoa(v * 65536 / 100)} }},
		{Name: "aplay", Args: []string{"-q"}},
		{Name: "ffplay", Args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}, volume: func(v int) []string { return []string{"-volume", strconv.Itoa(v)} }},
	},
}

// factor renders a 0-100 volume as a 0-1 factor, e.g. "0.35".
func factor(v int) string { return strconv.FormatFloat(float64(v)/100, 'f', -1, 64) }

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
			c.Volume = c.volume != nil
			if c.Args == nil {
				c.Args = []string{} // stable JSON: [] rather than null
			}
			return c, true
		}
	}
	return Player{}, false
}

// CommandLine returns the player's arguments for file at volume (0-100). A
// player that can't set the volume plays at full volume.
func (p Player) CommandLine(file string, volume int) []string {
	args := append([]string{}, p.Args...)
	if volume < 100 && p.volume != nil {
		args = append(args, p.volume(max(volume, 0))...)
	}
	return append(args, file)
}

func (p Player) command(file string, volume int) *exec.Cmd {
	return exec.Command(p.Path, p.CommandLine(file, volume)...)
}

// Start launches the player detached and returns without waiting for it.
func Start(p Player, file string, volume int) error {
	cmd := p.command(file, volume)
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
func Run(p Player, file string, volume int) error {
	out, err := p.command(file, volume).CombinedOutput()
	if msg := strings.TrimSpace(string(out)); err != nil && msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}
