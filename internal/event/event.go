// Package event plays the sound for a git event. It is the single place that
// decides whether and what to play, shared by the hook entry point and
// `jingle play` / `jingle preview` (which the GUI runs).
package event

import (
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/OPDhaker/jingle/internal/builtin"
	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/gitcfg"
	"github.com/OPDhaker/jingle/internal/player"
)

// Error codes returned by Play and Preview. They double as CLI error codes.
const (
	CodeUnknownEvent  = "unknown_event"
	CodeConfigInvalid = "config_invalid"
	CodeDisabled      = "disabled"
	CodeNoSound       = "no_sound"
	CodeSoundMissing  = "sound_missing"
	CodeSoundNotFound = "sound_not_found"
	CodeInvalidValue  = "invalid_value"
	CodeNoPlayer      = "no_player"
	CodePlayerFailed  = "player_failed"
	// CodeCooldown: the event played too recently. Hooks only; `jingle play`
	// ignores the cooldown, so this never reaches the CLI.
	CodeCooldown = "cooldown"
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
	// IgnoreCooldown plays even if the event played moments ago (`jingle play`).
	IgnoreCooldown bool
	// Wait blocks until the player exits instead of starting it detached.
	Wait bool
	// Sound plays this one of the event's sounds (see config.MatchSound)
	// instead of a random one.
	Sound string
}

// Result describes what Play did.
type Result struct {
	Event string `json:"event"`
	// Sound is the file that played, picked from the event's Sounds.
	Sound  string        `json:"sound"`
	Sounds int           `json:"sounds"`
	Player player.Player `json:"player"`
	// Volume is the configured volume; the player ignores it unless
	// Player.Volume is true.
	Volume int `json:"volume"`
	// Enabled is whether this event would play on a real commit/push.
	Enabled bool `json:"enabled"`
	Waited  bool `json:"waited"`
}

// pick chooses among the sounds that exist; replaced in tests.
var pick = func(n int) int { return rand.IntN(n) }

// headIsMerge is replaced in tests.
var headIsMerge = func() bool { return gitcfg.Git{}.HeadIsMerge() }

// MergeDone runs from post-merge. A merge that fast-forwarded or squashed
// made no merge commit, so it stays silent: pulling isn't merging.
func MergeDone(paths config.Paths) (played bool, err error) {
	if !headIsMerge() {
		return false, nil
	}
	_, err = Play(paths, "merge", Options{})
	return err == nil, err
}

// DetectPlayer finds the player for this machine.
func DetectPlayer() (player.Player, bool) {
	return player.Detect(os.Getenv, exec.LookPath, runtime.GOOS)
}

// Play plays the sound configured for event. Errors are *Error.
func Play(paths config.Paths, event string, opts Options) (Result, error) {
	res := Result{Event: event}
	if !config.IsEvent(event) {
		return res, errorf(CodeUnknownEvent, "unknown event %q (valid: %s)", event, strings.Join(config.Events, ", "))
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return res, errorf(CodeConfigInvalid, "%v", err)
	}
	res.Enabled = cfg.Enabled && cfg.Event(event).Enabled
	if !res.Enabled && !opts.IgnoreEnabled {
		return res, errorf(CodeDisabled, "%s sound is turned off", event)
	}
	entries := slices.Clone(cfg.Event(event).Sounds)
	res.Sounds = len(entries)
	if len(entries) == 0 {
		return res, errorf(CodeNoSound, "no sound set for %s; run: jingle config set %s.sounds <file>", event, event)
	}
	if opts.Sound != "" {
		entry, ok := cfg.MatchSound(paths, event, opts.Sound)
		if !ok {
			return res, errorf(CodeSoundNotFound, "%q is not one of the %s sounds; see: jingle config get %s.sounds", opts.Sound, event, event)
		}
		entries = []string{entry}
	}
	// A missing file is skipped as long as another sound can play.
	var existing, sounds []string
	for _, e := range entries {
		s := soundFile(paths, e)
		sounds = append(sounds, s)
		if isFile(s) {
			existing = append(existing, s)
		}
	}
	if len(existing) == 0 {
		return res, errorf(CodeSoundMissing, "sound file for %s not found: %s", event, strings.Join(sounds, ", "))
	}
	res.Sound = existing[pick(len(existing))]
	p, ok := DetectPlayer()
	if !ok {
		return res, errorf(CodeNoPlayer, "no audio player found on this machine")
	}
	res.Player = p
	res.Volume = cfg.Volume
	// Last check, so only a sound that would really play starts the cooldown.
	cooldown := time.Duration(cfg.Cooldown) * time.Second
	if !opts.IgnoreCooldown && !takeTurn(paths, event, cooldown, time.Now) {
		return res, errorf(CodeCooldown, "%s played less than %ds ago", event, cfg.Cooldown)
	}
	res.Waited = opts.Wait
	if err := start(p, res.Sound, cfg.Volume, opts.Wait); err != nil {
		return res, err
	}
	return res, nil
}

// PreviewResult describes what Preview did.
type PreviewResult struct {
	// Sound is the sound as given: a "builtin:<name>" entry or a file path.
	Sound string `json:"sound"`
	// File is the file that played (for a built-in, its rendered WAV).
	File   string        `json:"file"`
	Player player.Player `json:"player"`
	Volume int           `json:"volume"`
	Waited bool          `json:"waited"`
}

// Preview plays a sound that need not be in any event's list, at the
// configured volume, so it can be auditioned before it is added. It changes
// nothing: no config, no cooldown. Errors are *Error.
func Preview(paths config.Paths, sound string, wait bool) (PreviewResult, error) {
	res := PreviewResult{Sound: sound}
	cfg, err := config.Load(paths)
	if err != nil {
		return res, errorf(CodeConfigInvalid, "%v", err)
	}
	if name, ok := builtin.Parse(sound); ok {
		if !builtin.Exists(name) {
			return res, errorf(CodeInvalidValue, "unknown built-in sound %q (have: %s)", sound, strings.Join(builtin.Entries(), ", "))
		}
	} else {
		abs, err := filepath.Abs(sound)
		if err != nil || !isFile(abs) {
			return res, errorf(CodeSoundNotFound, "sound file not found: %s", sound)
		}
		if ext := strings.ToLower(filepath.Ext(abs)); !slices.Contains(config.SoundExts, ext) {
			return res, errorf(CodeInvalidValue, "unsupported audio format %q (supported: %s)", filepath.Ext(abs), strings.Join(config.SoundExts, " "))
		}
		sound = abs
	}
	res.File = soundFile(paths, sound)
	if !isFile(res.File) {
		return res, errorf(CodeSoundMissing, "sound file not found: %s", res.File)
	}
	p, ok := DetectPlayer()
	if !ok {
		return res, errorf(CodeNoPlayer, "no audio player found on this machine")
	}
	res.Player, res.Volume, res.Waited = p, cfg.Volume, wait
	if err := start(p, res.File, cfg.Volume, wait); err != nil {
		return res, err
	}
	return res, nil
}

// soundFile is the file to play for a sounds list entry. A built-in is
// rendered on first use; if that fails, the returned path doesn't exist and
// counts as a missing file.
func soundFile(paths config.Paths, entry string) string {
	if name, ok := builtin.Parse(entry); ok {
		if got, err := builtin.Ensure(paths.SoundsDir(), name); err == nil {
			return got
		}
	}
	return config.SoundFile(paths, entry)
}

// start plays file, detached unless wait is set.
func start(p player.Player, file string, volume int, wait bool) error {
	var err error
	if wait {
		err = player.Run(p, file, volume)
	} else {
		err = player.Start(p, file, volume)
	}
	if err != nil {
		return errorf(CodePlayerFailed, "%s failed: %v", p.Name, err)
	}
	return nil
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
