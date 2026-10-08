package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/OPDhaker/jingle/internal/fsutil"
)

// Version is the config file schema version this build understands.
const Version = 1

// Events are the git events jingle can play a sound for.
var Events = []string{"commit", "push"}

// IsEvent reports whether name is one of Events.
func IsEvent(name string) bool { return slices.Contains(Events, name) }

// Event holds the settings for one git event.
type Event struct {
	Enabled bool `toml:"enabled"`
	// Sounds are the files to play, one picked at random each time. Each is
	// absolute or relative to the sounds dir.
	Sounds []string `toml:"sounds"`
	// Sound is the single sound of configs written before Sounds existed.
	// Load moves it into Sounds, so it is never written back.
	Sound string `toml:"sound,omitempty"`
	// When is push only: PushSuccess or PushAttempt.
	When string `toml:"when,omitempty"`
}

// Values of push.when.
const (
	PushSuccess = "success" // play once the push went through (needs git 2.28+)
	PushAttempt = "attempt" // play when pre-push passes, before anything is sent
)

// Config is the user's preferences (config.toml). The TOML keys are exactly
// the keys accepted by `jingle config get/set`.
type Config struct {
	Version int  `toml:"version"`
	Enabled bool `toml:"enabled"`
	// Cooldown is the minimum number of seconds between two sounds for the
	// same event from git hooks, so a burst of commits plays once. 0 = off.
	Cooldown int   `toml:"cooldown"`
	Commit   Event `toml:"commit"`
	Push     Event `toml:"push"`
}

// MaxCooldown caps the cooldown setting (seconds).
const MaxCooldown = 3600

// Default returns the config used when no file exists.
func Default() Config {
	return Config{
		Version:  Version,
		Enabled:  true,
		Cooldown: 3,
		Commit:   Event{Enabled: true},
		Push:     Event{Enabled: true, When: PushSuccess},
	}
}

// Event returns a pointer to the settings for the named event, or nil.
func (c *Config) Event(name string) *Event {
	switch name {
	case "commit":
		return &c.Commit
	case "push":
		return &c.Push
	}
	return nil
}

// Load reads the config file. A missing file yields Default(). Unknown keys
// and unsupported versions are errors, so typos never silently do nothing.
func Load(p Paths) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(p.ConfigFile())
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", p.ConfigFile(), err)
	}
	if undec := md.Undecoded(); len(undec) > 0 {
		keys := make([]string, len(undec))
		for i, k := range undec {
			keys[i] = k.String()
		}
		return cfg, fmt.Errorf("%s: unknown keys: %s", p.ConfigFile(), strings.Join(keys, ", "))
	}
	if cfg.Version != Version {
		return cfg, fmt.Errorf("%s: unsupported config version %d (this jingle supports %d)", p.ConfigFile(), cfg.Version, Version)
	}
	for _, name := range Events {
		ev := cfg.Event(name)
		if ev.Sound != "" {
			ev.Sounds = slices.Insert(slices.DeleteFunc(ev.Sounds, func(s string) bool { return s == ev.Sound }), 0, ev.Sound)
			ev.Sound = ""
		}
	}
	if cfg.Commit.When != "" {
		return cfg, fmt.Errorf("%s: unknown keys: commit.when", p.ConfigFile())
	}
	if err := validWhen(cfg.Push.When); err != nil {
		return cfg, fmt.Errorf("%s: push.when: %w", p.ConfigFile(), err)
	}
	if err := validCooldown(cfg.Cooldown); err != nil {
		return cfg, fmt.Errorf("%s: cooldown: %w", p.ConfigFile(), err)
	}
	return cfg, nil
}

// Save writes the config file atomically.
func Save(p Paths, cfg Config) error {
	var buf bytes.Buffer
	buf.WriteString("# jingle config. Edit with `jingle config set <key> <value>`; see `jingle config --help`.\n")
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(p.ConfigFile(), buf.Bytes(), 0o644)
}

// Keys lists every config key, in display order.
var Keys = []string{"enabled", "cooldown", "commit.enabled", "commit.sounds", "push.enabled", "push.sounds", "push.when"}

// SoundsKey reports whether key is an "<event>.sounds" list key, and for
// which event.
func SoundsKey(key string) (event string, ok bool) {
	event, ok = strings.CutSuffix(key, ".sounds")
	return event, ok && slices.Contains(Keys, key)
}

// SoundsKeys lists the "<event>.sounds" keys.
func SoundsKeys() []string {
	var keys []string
	for _, k := range Keys {
		if _, ok := SoundsKey(k); ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// ErrUnknownKey is returned by Get and Set for keys not in Keys.
var ErrUnknownKey = errors.New("unknown config key")

// Get returns the value of key (a bool, int, string, or []string).
func (c *Config) Get(key string) (any, error) {
	switch key {
	case "enabled":
		return c.Enabled, nil
	case "cooldown":
		return c.Cooldown, nil
	}
	ev, field, ok := c.split(key)
	if !ok {
		return nil, unknownKey(key)
	}
	switch field {
	case "enabled":
		return ev.Enabled, nil
	case "when":
		return ev.When, nil
	}
	return append([]string{}, ev.Sounds...), nil // never nil: JSON []
}

// Set parses raw and assigns it to key, reporting whether the value changed.
// For a sounds key, raw becomes the only sound ("" clears the list). Sound
// values are stored as given; the caller is responsible for copying the file
// into the sounds dir first.
func (c *Config) Set(key, raw string) (changed bool, err error) {
	switch key {
	case "enabled":
		b, err := ParseBool(raw)
		if err != nil {
			return false, err
		}
		changed = c.Enabled != b
		c.Enabled = b
		return changed, nil
	case "cooldown":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return false, fmt.Errorf("invalid number %q (use whole seconds, 0 to %d)", raw, MaxCooldown)
		}
		if err := validCooldown(n); err != nil {
			return false, err
		}
		changed = c.Cooldown != n
		c.Cooldown = n
		return changed, nil
	}
	ev, field, ok := c.split(key)
	if !ok {
		return false, unknownKey(key)
	}
	switch field {
	case "enabled":
		b, err := ParseBool(raw)
		if err != nil {
			return false, err
		}
		changed = ev.Enabled != b
		ev.Enabled = b
		return changed, nil
	case "when":
		v := strings.ToLower(raw)
		if err := validWhen(v); err != nil {
			return false, err
		}
		changed = ev.When != v
		ev.When = v
		return changed, nil
	}
	if raw == "" {
		return ev.SetSounds(nil), nil
	}
	return ev.SetSounds([]string{raw}), nil
}

// SetSounds replaces the sound list (empty entries and duplicates dropped)
// and reports whether it changed.
func (e *Event) SetSounds(list []string) bool {
	var clean []string
	for _, s := range list {
		if s != "" && !slices.Contains(clean, s) {
			clean = append(clean, s)
		}
	}
	if slices.Equal(e.Sounds, clean) {
		return false
	}
	e.Sounds = clean
	return true
}

// AddSound appends s unless it is already in the list.
func (e *Event) AddSound(s string) bool {
	if s == "" || slices.Contains(e.Sounds, s) {
		return false
	}
	e.Sounds = append(e.Sounds, s)
	return true
}

// RemoveSound drops s from the list, reporting whether it was there.
func (e *Event) RemoveSound(s string) bool {
	n := len(e.Sounds)
	e.Sounds = slices.DeleteFunc(e.Sounds, func(x string) bool { return x == s })
	return len(e.Sounds) != n
}

// split resolves "<event>.<field>" to the event settings and field name.
func (c *Config) split(key string) (*Event, string, bool) {
	if !slices.Contains(Keys, key) {
		return nil, "", false
	}
	name, field, ok := strings.Cut(key, ".")
	if !ok {
		return nil, "", false
	}
	ev := c.Event(name)
	return ev, field, ev != nil
}

func unknownKey(key string) error {
	return fmt.Errorf("%w %q (valid keys: %s)", ErrUnknownKey, key, strings.Join(Keys, ", "))
}

func validCooldown(n int) error {
	if n < 0 || n > MaxCooldown {
		return fmt.Errorf("invalid cooldown %d (use 0 to %d seconds)", n, MaxCooldown)
	}
	return nil
}

func validWhen(v string) error {
	if v != PushSuccess && v != PushAttempt {
		return fmt.Errorf("invalid value %q (use %s or %s)", v, PushSuccess, PushAttempt)
	}
	return nil
}

// Flatten returns every key and its value.
func (c *Config) Flatten() map[string]any {
	m := make(map[string]any, len(Keys))
	for _, k := range Keys {
		m[k], _ = c.Get(k)
	}
	return m
}

// ParseBool accepts true/false/on/off (any case).
func ParseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "true", "on":
		return true, nil
	case "false", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q (use true, false, on, or off)", s)
}

// SoundPaths returns the absolute paths of the event's sounds. Relative
// entries resolve against the sounds dir.
func (c *Config) SoundPaths(p Paths, event string) []string {
	ev := c.Event(event)
	if ev == nil {
		return nil
	}
	paths := make([]string, len(ev.Sounds))
	for i, s := range ev.Sounds {
		paths[i] = SoundFile(p, s)
	}
	return paths
}

// MatchSound finds the list entry that name refers to: the entry itself, the
// same file, or the same file name, with or without the "<event>-" prefix
// jingle adds when it copies a sound in. Exact matches win over name matches.
func (c *Config) MatchSound(p Paths, event, name string) (entry string, ok bool) {
	ev := c.Event(event)
	if ev == nil || name == "" {
		return "", false
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		abs = name
	}
	for _, s := range ev.Sounds {
		if s == name || SoundFile(p, s) == abs {
			return s, true
		}
	}
	base := filepath.Base(name)
	for _, s := range ev.Sounds {
		if b := filepath.Base(s); b == base || strings.TrimPrefix(b, event+"-") == base {
			return s, true
		}
	}
	return "", false
}

// SoundFile returns the absolute path of a sounds list entry.
func SoundFile(p Paths, s string) string {
	if filepath.IsAbs(s) {
		return s
	}
	return filepath.Join(p.SoundsDir(), s)
}
