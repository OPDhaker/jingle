package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/OPDhaker/jingle/internal/builtin"
	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/doctor"
	"github.com/OPDhaker/jingle/internal/output"
)

// schemaDoc is `jingle schema`: everything an agent needs to drive jingle,
// built from the live command tree and code tables, never copied by hand.
type schemaDoc struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	Commands      []schemaCommand   `json:"commands"`
	GlobalFlags   []schemaFlag      `json:"global_flags"`
	Events        []string          `json:"events"`
	ConfigKeys    []schemaKey       `json:"config_keys"`
	BuiltinSounds []schemaSound     `json:"builtin_sounds"`
	ExitCodes     []output.ExitCode `json:"exit_codes"`
	ErrorCodes    []output.Code     `json:"error_codes"`
	ProblemCodes  []doctor.CodeInfo `json:"problem_codes"`
	Env           []schemaEnv       `json:"env"`
}

type schemaCommand struct {
	// Path is the command without "jingle", e.g. "config set".
	Path  string `json:"path"`
	Usage string `json:"usage"`
	Short string `json:"short"`
	Long  string `json:"long"`
	// Runnable is false for a command that only groups subcommands.
	Runnable  bool         `json:"runnable"`
	Examples  []string     `json:"examples"`
	ValidArgs []string     `json:"valid_args"`
	Flags     []schemaFlag `json:"flags"`
}

type schemaFlag struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Type      string `json:"type"`
	Default   string `json:"default"`
	Usage     string `json:"usage"`
}

type schemaKey struct {
	config.KeyInfo
	Default any `json:"default"`
}

type schemaSound struct {
	Name       string   `json:"name"`
	Doc        string   `json:"doc"`
	DefaultFor []string `json:"default_for"`
}

type schemaEnv struct {
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}

var schemaEnvVars = []schemaEnv{
	{"JINGLE_HOME", "put jingle's config and data in this one dir (tests, portable installs)"},
	{"JINGLE_PLAYER", "use this executable as the audio player, called as <exe> <file>"},
	{"XDG_CONFIG_HOME", "config dir is $XDG_CONFIG_HOME/jingle (default ~/.config/jingle)"},
	{"XDG_DATA_HOME", "data dir (hooks, sounds, state) is $XDG_DATA_HOME/jingle (default ~/.local/share/jingle)"},
	{"NO_COLOR", "no color in plain output"},
}

func newSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Describe every command, flag, config key, and code as JSON",
		Long: `Print a machine-readable description of jingle: every command with its
usage, flags, and examples; the config keys with their types, defaults, and
allowed values; the built-in sounds; and the exit, error, and problem codes.
The output is always JSON (--json changes nothing). Its fields are part of
the JSON contract, versioned by schema_version.`,
		Example: `  jingle schema
  jingle schema | jq '.config_keys[] | {key, default}'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false) // usage strings are full of <arg>
			if err := enc.Encode(buildSchema(cmd.Root())); err != nil {
				return output.Errorf("io_error", output.ExitError, "%v", err)
			}
			return nil
		},
	}
}

func buildSchema(root *cobra.Command) schemaDoc {
	doc := schemaDoc{
		SchemaVersion: schemaVersion,
		Version:       version,
		Commands:      []schemaCommand{},
		GlobalFlags:   flagsOf(root.PersistentFlags()),
		Events:        config.Events,
		ExitCodes:     output.ExitCodes,
		ErrorCodes:    output.ErrorCodes,
		ProblemCodes:  doctor.Codes,
		Env:           schemaEnvVars,
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			doc.Commands = append(doc.Commands, describe(sub))
			walk(sub)
		}
	}
	walk(root)

	defaults := config.Default()
	for _, k := range config.KeyInfos {
		v, _ := defaults.Get(k.Key)
		doc.ConfigKeys = append(doc.ConfigKeys, schemaKey{KeyInfo: k, Default: v})
	}
	for _, name := range builtin.Names() {
		s := schemaSound{Name: builtin.Prefix + name, Doc: builtin.Doc(name), DefaultFor: []string{}}
		for _, ev := range config.Events {
			for _, d := range defaults.Event(ev).Sounds {
				if d == s.Name {
					s.DefaultFor = append(s.DefaultFor, ev)
				}
			}
		}
		doc.BuiltinSounds = append(doc.BuiltinSounds, s)
	}
	return doc
}

func describe(c *cobra.Command) schemaCommand {
	sc := schemaCommand{
		Path:      strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" "),
		Usage:     strings.TrimSuffix(c.UseLine(), " [flags]"),
		Short:     c.Short,
		Long:      c.Long,
		Runnable:  c.Runnable(),
		Examples:  []string{},
		ValidArgs: append([]string{}, c.ValidArgs...),
		Flags:     flagsOf(c.NonInheritedFlags()),
	}
	for _, line := range strings.Split(c.Example, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			sc.Examples = append(sc.Examples, line)
		}
	}
	return sc
}

func flagsOf(fs *pflag.FlagSet) []schemaFlag {
	flags := []schemaFlag{}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		flags = append(flags, schemaFlag{Name: f.Name, Shorthand: f.Shorthand, Type: f.Value.Type(), Default: f.DefValue, Usage: f.Usage})
	})
	return flags
}
