package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/event"
	"github.com/OPDhaker/jingle/internal/gitcfg"
	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/output"
	"github.com/OPDhaker/jingle/internal/player"
)

type problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

type gitInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

type statusInfo struct {
	hooks.Inspection
	JingleExecutable string         `json:"jingle_executable"`
	ConfigPath       string         `json:"config_path"`
	Config           map[string]any `json:"config"`
	Player           *player.Player `json:"player"`
	Git              *gitInfo       `json:"git"`
	Problems         []problem      `json:"problems"`
}

func newStatusCmd(printer func(*cobra.Command) *output.Printer) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show install state, settings, player, and problems",
		Long: `Report everything needed to decide what to do next: whether jingle is
installed, the global/system/previous core.hooksPath, the effective config,
the detected audio player, the git version, and any problems (each with a
fix). Always exits 0 when it can report; check "problems" for issues.`,
		Example: `  jingle status
  jingle status --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env, err := hooksEnv()
			if err != nil {
				return err
			}
			st := gatherStatus(env)
			return printer(cmd).Result(st, func(w io.Writer) { printStatus(w, st) })
		},
	}
}

func gatherStatus(env hooks.Env) statusInfo {
	st := statusInfo{
		Inspection:       hooks.Inspection{HooksDir: env.Paths.HooksDir()},
		JingleExecutable: env.JinglePath,
		ConfigPath:       env.Paths.ConfigFile(),
		Problems:         []problem{},
	}
	add := func(code, msg, fix string) { st.Problems = append(st.Problems, problem{code, msg, fix}) }

	if gitPath, err := env.Git.Path(); err != nil {
		add("git_not_found", "git is not installed or not on PATH", "install git")
	} else if ver, v, err := env.Git.Version(); err != nil {
		add("git_failed", fmt.Sprintf("cannot run git: %v", err), "")
	} else {
		st.Git = &gitInfo{Path: gitPath, Version: ver}
		if gitcfg.VersionLess(v, gitcfg.MinPushDetect) {
			add("git_too_old", fmt.Sprintf("git %s is older than 2.28, so jingle cannot tell when a push succeeded; push plays on attempt instead", ver), "upgrade git")
		}
		if in, err := hooks.Inspect(env); err != nil {
			add("git_failed", fmt.Sprintf("cannot read git config: %v", err), "")
		} else {
			st.Inspection = in
			switch {
			case in.StateInstalled && !in.Installed:
				add("hooks_path_overridden", "global core.hooksPath was changed after install, so jingle's hooks don't run", "jingle install --yes")
			case !in.Installed:
				add("not_installed", "jingle's hooks are not installed", "jingle install --yes")
			case !in.ShimsCurrent:
				add("shims_outdated", "the installed hook scripts differ from what this jingle writes", "jingle install --yes")
			}
		}
	}

	cfg, err := config.Load(env.Paths)
	if err != nil {
		add("config_invalid", err.Error(), "fix or delete "+env.Paths.ConfigFile())
	} else {
		st.Config = cfg.Flatten()
		for _, ev := range config.Events {
			if !cfg.Enabled || !cfg.Event(ev).Enabled {
				continue
			}
			switch path := cfg.SoundPath(env.Paths, ev); {
			case path == "":
				add("no_sound", "no sound set for "+ev, "jingle config set "+ev+".sound <file>")
			case !isFile(path):
				add("sound_missing", "sound file for "+ev+" not found: "+path, "jingle config set "+ev+".sound <file>")
			}
		}
	}

	if p, ok := event.DetectPlayer(); ok {
		st.Player = &p
	} else {
		add("no_player", "no audio player found (macOS: afplay; Linux: pw-play, paplay, aplay, or ffplay)", "install one of the listed players")
	}
	return st
}

func printStatus(w io.Writer, st statusInfo) {
	yn := map[bool]string{true: "yes", false: "no"}
	fmt.Fprintf(w, "installed:  %s\n", yn[st.Installed])
	fmt.Fprintf(w, "hooks dir:  %s\n", st.HooksDir)
	if st.GlobalHooksPath != nil {
		fmt.Fprintf(w, "global core.hooksPath: %s\n", *st.GlobalHooksPath)
	}
	if st.PreviousHooksPath != nil {
		fmt.Fprintf(w, "previous core.hooksPath: %s\n", *st.PreviousHooksPath)
	}
	fmt.Fprintf(w, "config:     %s\n", st.ConfigPath)
	for _, k := range config.Keys {
		if v, ok := st.Config[k]; ok {
			fmt.Fprintf(w, "  %s = %v\n", k, v)
		}
	}
	if st.Player != nil {
		fmt.Fprintf(w, "player:     %s (%s)\n", st.Player.Name, st.Player.Path)
	}
	if st.Git != nil {
		fmt.Fprintf(w, "git:        %s (%s)\n", st.Git.Version, st.Git.Path)
	}
	if len(st.Problems) == 0 {
		fmt.Fprintln(w, "problems:   none")
		return
	}
	fmt.Fprintln(w, "problems:")
	for _, p := range st.Problems {
		fmt.Fprintf(w, "  %s: %s\n", p.Code, p.Message)
		if p.Fix != "" {
			fmt.Fprintf(w, "    fix: %s\n", p.Fix)
		}
	}
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
