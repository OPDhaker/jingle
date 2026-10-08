package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/output"
)

const configKeysHelp = `Keys:
  enabled          true|false  master switch
  cooldown         <seconds>   quiet time after a sound, per event, so a
                               burst of commits plays once (default 3, 0 off)
  commit.enabled   true|false  play on git commit
  commit.sounds    <file>...   sounds for commits; one is picked at random
  push.enabled     true|false  play on git push
  push.sounds      <file>...   sounds for pushes; one is picked at random
  push.when        success|attempt
                               success (default): play once the push went
                               through; attempt: play when pre-push passes

Booleans also accept on/off. Sound files (` + "mp3 wav aiff aif m4a ogg flac" + `) are
copied into jingle's data dir, so moving the original later is fine.
"config set <event>.sounds" replaces the whole list ("" clears it);
"config add" and "config remove" change one entry at a time.

push.when success needs git 2.28+. With older git, and for pushes to a URL
rather than a named remote (no remote-tracking ref moves), push plays on
attempt.`

func newConfigCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and change settings",
		Long:  "Read and change jingle's settings. Changes are validated and take effect immediately.\n\n" + configKeysHelp,
		Example: `  jingle config list --json
  jingle config get commit.sounds
  jingle config set commit.sounds ~/Music/tag.mp3
  jingle config add commit.sounds ~/Music/tag2.mp3 ~/Music/tag3.wav
  jingle config remove commit.sounds tag2.mp3
  jingle config set push.enabled false
  jingle config set push.when attempt
  jingle config set cooldown 0`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigListCmd(printer), newConfigGetCmd(printer), newConfigSetCmd(printer),
		newConfigAddCmd(printer), newConfigRemoveCmd(printer))
	return cmd
}

func loadConfig() (config.Paths, config.Config, error) {
	p, err := paths()
	if err != nil {
		return p, config.Config{}, err
	}
	cfg, err := config.Load(p)
	if err != nil {
		return p, cfg, output.Errorf("config_invalid", output.ExitError, "%v", err)
	}
	return p, cfg, nil
}

type configList struct {
	Path   string         `json:"path"`
	Values map[string]any `json:"values"`
}

func newConfigListCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show every setting",
		Example: `  jingle config list
  jingle config list --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			res := configList{Path: p.ConfigFile(), Values: cfg.Flatten()}
			return printer(cmd).Result(res, func(w io.Writer) {
				for _, k := range config.Keys {
					fmt.Fprintf(w, "%s = %s\n", k, formatValue(res.Values[k]))
				}
			})
		},
	}
}

type configValue struct {
	Key     string `json:"key"`
	Value   any    `json:"value"`
	Changed *bool  `json:"changed,omitempty"`
}

func newConfigGetCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one setting",
		Long:  "Print one setting.\n\n" + configKeysHelp,
		Example: `  jingle config get enabled
  jingle config get push.sounds --json`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: config.Keys,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			v, err := cfg.Get(args[0])
			if err != nil {
				return output.Errorf("invalid_key", output.ExitUsage, "%v", err)
			}
			return printer(cmd).Result(configValue{Key: args[0], Value: v}, func(w io.Writer) {
				if list, ok := v.([]string); ok {
					for _, s := range list { // one per line, easy to read back
						fmt.Fprintln(w, s)
					}
					return
				}
				fmt.Fprintln(w, v)
			})
		},
	}
}

func newConfigSetCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>...",
		Short: "Change one setting",
		Long: "Change one setting. Setting a value it already has reports changed=false.\n" +
			"A sounds key takes one or more files and replaces the whole list.\n\n" + configKeysHelp,
		Example: `  jingle config set enabled off
  jingle config set commit.sounds ./tags/commit.wav --json
  jingle config set push.sounds ~/Music/a.mp3 ~/Music/b.mp3
  jingle config set push.sounds ""`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, values := args[0], args[1:]
			p, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if _, err := cfg.Get(key); err != nil {
				return output.Errorf("invalid_key", output.ExitUsage, "%v", err)
			}
			event, isSounds := config.SoundsKey(key)
			if !isSounds {
				if len(values) != 1 {
					return output.Errorf("usage", output.ExitUsage, "%s takes exactly one value, got %d", key, len(values))
				}
				changed, err := cfg.Set(key, values[0])
				if err != nil {
					return output.Errorf("invalid_value", output.ExitUsage, "%v", err)
				}
				return saveAndPrint(cmd, printer, p, cfg, key, changed, nil)
			}
			list, wrote, err := importSounds(p, event, values)
			if err != nil {
				return err
			}
			ev := cfg.Event(event)
			old := ev.Sounds
			changed := ev.SetSounds(list)
			return saveAndPrint(cmd, printer, p, cfg, key, changed || wrote, dropped(old, ev.Sounds))
		},
	}
}

func newConfigAddCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "add <event>.sounds <file>...",
		Short: "Add sounds to an event's list",
		Long: `Add one or more sound files to an event's list. Each sound is copied
into jingle's data dir; on every commit or push one sound from the list is
picked at random. Adding a sound that is already in the list reports
changed=false.`,
		Example: `  jingle config add commit.sounds ~/Music/tag2.mp3
  jingle config add push.sounds ./a.wav ./b.wav --json`,
		Args:      cobra.MinimumNArgs(2),
		ValidArgs: config.SoundsKeys(),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, cfg, event, err := loadSoundsKey(args[0])
			if err != nil {
				return err
			}
			list, wrote, err := importSounds(p, event, args[1:])
			if err != nil {
				return err
			}
			changed := wrote
			for _, s := range list {
				changed = cfg.Event(event).AddSound(s) || changed
			}
			return saveAndPrint(cmd, printer, p, cfg, args[0], changed, nil)
		},
	}
}

func newConfigRemoveCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <event>.sounds <file|name>...",
		Short: "Remove sounds from an event's list",
		Long: `Remove one or more sounds from an event's list. Name a sound by its entry
(as "config get" prints it), by the original file, or by its file name.
jingle's copy is deleted; your original file is never touched. Naming a
sound that is not in the list is an error (invalid_value), and nothing
changes.`,
		Example: `  jingle config remove commit.sounds tag2.mp3
  jingle config remove push.sounds ~/Music/a.wav --json`,
		Args:      cobra.MinimumNArgs(2),
		ValidArgs: config.SoundsKeys(),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			p, cfg, event, err := loadSoundsKey(key)
			if err != nil {
				return err
			}
			ev := cfg.Event(event)
			var entries []string
			for _, name := range args[1:] {
				entry, ok := cfg.MatchSound(p, event, name)
				if !ok {
					return output.Errorf("invalid_value", output.ExitUsage, "%q is not in %s (have: %s)", name, key, formatValue(ev.Sounds))
				}
				entries = append(entries, entry)
			}
			old := slices.Clone(ev.Sounds)
			changed := false
			for _, e := range entries {
				changed = ev.RemoveSound(e) || changed
			}
			return saveAndPrint(cmd, printer, p, cfg, key, changed, dropped(old, ev.Sounds))
		},
	}
}

// loadSoundsKey loads the config and checks that key is a sounds list key.
func loadSoundsKey(key string) (config.Paths, config.Config, string, error) {
	p, cfg, err := loadConfig()
	if err != nil {
		return p, cfg, "", err
	}
	event, ok := config.SoundsKey(key)
	if !ok {
		return p, cfg, "", output.Errorf("invalid_key", output.ExitUsage, "%q is not a sounds key (use one of: %s)", key, strings.Join(config.SoundsKeys(), ", "))
	}
	return p, cfg, event, nil
}

// importSounds copies each file into the sounds dir and returns the list
// entries. A lone "" means an empty list.
func importSounds(p config.Paths, event string, files []string) (list []string, wrote bool, err error) {
	for _, f := range files {
		if f == "" {
			continue
		}
		dest, w, err := config.ImportSound(p, event, f)
		if err != nil {
			return nil, false, soundError(err)
		}
		list, wrote = append(list, dest), wrote || w
	}
	return list, wrote, nil
}

// dropped returns the entries of old that are not in cur.
func dropped(old, cur []string) []string {
	var out []string
	for _, s := range old {
		if !slices.Contains(cur, s) {
			out = append(out, s)
		}
	}
	return out
}

// saveAndPrint saves cfg if it changed, deletes jingle's copies of sounds
// that left a list, and prints the key's new value.
func saveAndPrint(cmd *cobra.Command, printer func(*cobra.Command) *output.Printer, p config.Paths, cfg config.Config, key string, changed bool, removed []string) error {
	if changed {
		if err := config.Save(p, cfg); err != nil {
			return output.Errorf("io_error", output.ExitError, "%v", err)
		}
		if event, ok := config.SoundsKey(key); ok {
			for _, s := range removed {
				// Best effort: a stale copy only wastes a little disk.
				_ = config.RemoveImportedSound(p, event, s)
			}
		}
	}
	v, _ := cfg.Get(key)
	return printer(cmd).Result(configValue{Key: key, Value: v, Changed: &changed}, func(w io.Writer) {
		if changed {
			fmt.Fprintf(w, "%s = %s\n", key, formatValue(v))
		} else {
			fmt.Fprintf(w, "%s = %s (unchanged)\n", key, formatValue(v))
		}
	})
}

// formatValue renders a config value for plain output; lists as [a, b].
func formatValue(v any) string {
	if list, ok := v.([]string); ok {
		return "[" + strings.Join(list, ", ") + "]"
	}
	return fmt.Sprint(v)
}

func soundError(err error) error {
	switch {
	case errors.Is(err, config.ErrSoundNotFound):
		return output.Errorf("sound_not_found", output.ExitError, "%v", err)
	case errors.Is(err, config.ErrUnsupportedSound):
		return output.Errorf("invalid_value", output.ExitUsage, "%v", err)
	}
	return output.Errorf("io_error", output.ExitError, "%v", err)
}
