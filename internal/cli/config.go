package cli

import (
	"errors"
	"fmt"
	"io"
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
  commit.sound     <file>      sound for commits ("" to clear)
  push.enabled     true|false  play on git push
  push.sound       <file>      sound for pushes ("" to clear)
  push.when        success|attempt
                               success (default): play once the push went
                               through; attempt: play when pre-push passes

Booleans also accept on/off. Sound files (` + "mp3 wav aiff aif m4a ogg flac" + `) are
copied into jingle's data dir, so moving the original later is fine.

push.when success needs git 2.28+. With older git, and for pushes to a URL
rather than a named remote (no remote-tracking ref moves), push plays on
attempt.`

func newConfigCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and change settings",
		Long:  "Read and change jingle's settings. Changes are validated and take effect immediately.\n\n" + configKeysHelp,
		Example: `  jingle config list --json
  jingle config get commit.sound
  jingle config set commit.sound ~/Music/tag.mp3
  jingle config set push.enabled false
  jingle config set push.when attempt
  jingle config set cooldown 0`,
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newConfigListCmd(printer), newConfigGetCmd(printer), newConfigSetCmd(printer))
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
					fmt.Fprintf(w, "%s = %v\n", k, res.Values[k])
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
  jingle config get push.sound --json`,
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
				fmt.Fprintln(w, v)
			})
		},
	}
}

func newConfigSetCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Change one setting",
		Long:  "Change one setting. Setting a value it already has reports changed=false.\n\n" + configKeysHelp,
		Example: `  jingle config set enabled off
  jingle config set commit.sound ./tags/commit.wav --json
  jingle config set push.sound ""`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, raw := args[0], args[1]
			p, cfg, err := loadConfig()
			if err != nil {
				return err
			}
			old, err := cfg.Get(key)
			if err != nil {
				return output.Errorf("invalid_key", output.ExitUsage, "%v", err)
			}
			event, isSound := strings.CutSuffix(key, ".sound")
			wrote := false
			if isSound && raw != "" {
				if raw, wrote, err = config.ImportSound(p, event, raw); err != nil {
					return soundError(err)
				}
			}
			changed, err := cfg.Set(key, raw)
			if err != nil {
				return output.Errorf("invalid_value", output.ExitUsage, "%v", err)
			}
			if changed {
				if err := config.Save(p, cfg); err != nil {
					return output.Errorf("io_error", output.ExitError, "%v", err)
				}
				if isSound {
					// Best effort: a stale copy only wastes a little disk.
					_ = config.RemoveImportedSound(p, event, old.(string))
				}
			}
			changed = changed || wrote
			v, _ := cfg.Get(key)
			return printer(cmd).Result(configValue{Key: key, Value: v, Changed: &changed}, func(w io.Writer) {
				if changed {
					fmt.Fprintf(w, "%s = %v\n", key, v)
				} else {
					fmt.Fprintf(w, "%s = %v (unchanged)\n", key, v)
				}
			})
		},
	}
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
