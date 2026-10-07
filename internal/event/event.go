// Package event plays the sound for a git event. It is the single place that
// decides whether and what to play, shared by the hook entry point, `jingle
// play`, and (later) the GUI.
package event

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/player"
)

// Error codes returned by Play. They double as CLI error codes.
const (
	CodeUnknownEvent  = "unknown_event"
	CodeConfigInvalid = "config_invalid"
	CodeDisabled      = "disabled"
	CodeNoSound       = "no_sound"
	CodeSoundMissing  = "sound_missing"
	CodeNoPlayer      = "no_player"
	CodePlayerFailed  = "player_failed"
)

// Error is a Play failure with a stable code.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Options control Play.
type Options struct {
	// IgnoreEnabled plays even if jingle or the event is switched off (`jingle play`).
	IgnoreEnabled bool
	// Wait blocks until the player exits instead of starting it detached.
	Wait bool
}

// Result describes what Play did.
type Result struct {
	Event  string        `json:"event"`
	Sound  string        `json:"sound"`
	Player player.Player `json:"player"`
	// Enabled is whether this event would play on a real commit/push.
	Enabled bool `json:"enabled"`
	Waited  bool `json:"waited"`
}

// DetectPlayer finds the player for this machine.
func DetectPlayer() (player.Player, bool) {
	return player.Detect(os.Getenv, exec.LookPath, runtime.GOOS)
}

// Play plays the sound configured for event. Errors are *Error.
func Play(paths config.Paths, event string, opts Options) (Result, error) {
	res := Result{Event: event}
	if !config.IsEvent(event) {
		return res, errorf(CodeUnknownEvent, "unknown event %q (valid: commit, push)", event)
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return res, errorf(CodeConfigInvalid, "%v", err)
	}
	res.Enabled = cfg.Enabled && cfg.Event(event).Enabled
	if !res.Enabled && !opts.IgnoreEnabled {
		return res, errorf(CodeDisabled, "%s sound is turned off", event)
	}
	res.Sound = cfg.SoundPath(paths, event)
	if res.Sound == "" {
		return res, errorf(CodeNoSound, "no sound set for %s; run: jingle config set %s.sound <file>", event, event)
	}
	if fi, err := os.Stat(res.Sound); err != nil || !fi.Mode().IsRegular() {
		return res, errorf(CodeSoundMissing, "sound file for %s not found: %s", event, res.Sound)
	}
	p, ok := DetectPlayer()
	if !ok {
		return res, errorf(CodeNoPlayer, "no audio player found on this machine")
	}
	res.Player = p
	if opts.Wait {
		res.Waited = true
		err = player.Run(p, res.Sound)
	} else {
		err = player.Start(p, res.Sound)
	}
	if err != nil {
		return res, errorf(CodePlayerFailed, "%s failed: %v", p.Name, err)
	}
	return res, nil
}
