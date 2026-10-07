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
	Enabled bool   `toml:"enabled"`
	Sound   string `toml:"sound"` // absolute, or relative to the sounds dir; "" = none
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
var Keys = []string{"enabled", "cooldown", "commit.enabled", "commit.sound", "push.enabled", "push.sound", "push.when"}

// ErrUnknownKey is returned by Get and Set for keys not in Keys.
var ErrUnknownKey = errors.New("unknown config key")

// Get returns the value of key (a bool, int, or string).
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
	return ev.Sound, nil
}

// Set parses raw and assigns it to key, reporting whether the value changed.
// Sound values are stored as given; the caller is responsible for copying the
// file into the sounds dir first.
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
	changed = ev.Sound != raw
	ev.Sound = raw
	return changed, nil
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

// SoundPath returns the absolute path of the event's sound, or "" if none is
// set. Relative paths resolve against the sounds dir.
func (c *Config) SoundPath(p Paths, event string) string {
	ev := c.Event(event)
	if ev == nil || ev.Sound == "" {
		return ""
	}
	if filepath.IsAbs(ev.Sound) {
		return ev.Sound
	}
	return filepath.Join(p.SoundsDir(), ev.Sound)
}
