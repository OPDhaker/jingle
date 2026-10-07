package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
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
}

// Config is the user's preferences (config.toml). The TOML keys are exactly
// the keys accepted by `jingle config get/set`.
type Config struct {
	Version int   `toml:"version"`
	Enabled bool  `toml:"enabled"`
	Commit  Event `toml:"commit"`
	Push    Event `toml:"push"`
}

// Default returns the config used when no file exists.
func Default() Config {
	return Config{
		Version: Version,
		Enabled: true,
		Commit:  Event{Enabled: true},
		Push:    Event{Enabled: true},
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
var Keys = []string{"enabled", "commit.enabled", "commit.sound", "push.enabled", "push.sound"}

// ErrUnknownKey is returned by Get and Set for keys not in Keys.
var ErrUnknownKey = errors.New("unknown config key")

// Get returns the value of key (a bool or a string).
func (c *Config) Get(key string) (any, error) {
	if key == "enabled" {
		return c.Enabled, nil
	}
	ev, field, ok := c.split(key)
	if !ok {
		return nil, fmt.Errorf("%w %q (valid keys: %s)", ErrUnknownKey, key, strings.Join(Keys, ", "))
	}
	if field == "enabled" {
		return ev.Enabled, nil
	}
	return ev.Sound, nil
}

// Set parses raw and assigns it to key, reporting whether the value changed.
// Sound values are stored as given; the caller is responsible for copying the
// file into the sounds dir first.
func (c *Config) Set(key, raw string) (changed bool, err error) {
	if key == "enabled" {
		b, err := ParseBool(raw)
		if err != nil {
			return false, err
		}
		changed = c.Enabled != b
		c.Enabled = b
		return changed, nil
	}
	ev, field, ok := c.split(key)
	if !ok {
		return false, fmt.Errorf("%w %q (valid keys: %s)", ErrUnknownKey, key, strings.Join(Keys, ", "))
	}
	if field == "enabled" {
		b, err := ParseBool(raw)
		if err != nil {
			return false, err
		}
		changed = ev.Enabled != b
		ev.Enabled = b
		return changed, nil
	}
	changed = ev.Sound != raw
	ev.Sound = raw
	return changed, nil
}

// split resolves "<event>.<field>" to the event settings and field name.
func (c *Config) split(key string) (*Event, string, bool) {
	name, field, ok := strings.Cut(key, ".")
	if !ok || (field != "enabled" && field != "sound") {
		return nil, "", false
	}
	ev := c.Event(name)
	return ev, field, ev != nil
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
