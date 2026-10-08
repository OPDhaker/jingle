// Package doctor finds every known reason jingle would stay silent and says
// how to fix each one. It is shared by `jingle status`, `jingle doctor`, and
// (later) the GUI, so the checks live in one place.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/event"
	"github.com/OPDhaker/jingle/internal/gitcfg"
	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/player"
)

// Severity says whether a problem stops sounds (error) or only degrades
// them (warning).
type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

// Problem is one finding. Code is stable; Message and Fix are for people.
type Problem struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix,omitempty"`
}

// GitInfo is the git binary jingle found.
type GitInfo struct {
	Path    string `json:"path"`
	Version string `json:"version"`
}

// RepoInfo describes the repo that was checked.
type RepoInfo struct {
	Path string `json:"path"`
	// HooksPath is the core.hooksPath git uses in this repo; nil if unset.
	HooksPath      *string `json:"hooks_path"`
	HooksPathScope string  `json:"hooks_path_scope,omitempty"`
	// Husky: the repo's core.hooksPath is husky's.
	Husky bool `json:"husky"`
}

// Report is everything Check found.
type Report struct {
	hooks.Inspection
	JingleExecutable string         `json:"jingle_executable"`
	ConfigPath       string         `json:"config_path"`
	Config           map[string]any `json:"config"`
	Player           *player.Player `json:"player"`
	Git              *GitInfo       `json:"git"`
	Repo             *RepoInfo      `json:"repo,omitempty"`
	Problems         []Problem      `json:"problems"`
}

// Errors counts the problems with error severity.
func (r Report) Errors() int {
	n := 0
	for _, p := range r.Problems {
		if p.Severity == Error {
			n++
		}
	}
	return n
}

// Check inspects the install, config, player, and git. If repoRoot is not
// empty (see gitcfg.Git.RepoRoot), it also checks that repo's view of
// core.hooksPath.
func Check(env hooks.Env, repoRoot string) Report {
	r := Report{
		Inspection:       hooks.Inspection{HooksDir: env.Paths.HooksDir()},
		JingleExecutable: env.JinglePath,
		ConfigPath:       env.Paths.ConfigFile(),
		Problems:         []Problem{},
	}
	gitOK := r.checkGit(env)
	if gitOK {
		r.checkInstall(env)
	}
	r.checkConfig(env)
	r.checkPlayer(env)
	if gitOK && repoRoot != "" {
		r.checkRepo(env, repoRoot)
	}
	return r
}

func (r *Report) add(sev Severity, code, msg, fix string) {
	r.Problems = append(r.Problems, Problem{Code: code, Severity: sev, Message: msg, Fix: fix})
}

func (r *Report) checkGit(env hooks.Env) bool {
	gitPath, err := env.Git.Path()
	if err != nil {
		r.add(Error, "git_not_found", "git is not installed or not on PATH", "install git")
		return false
	}
	ver, v, err := env.Git.Version()
	if err != nil {
		r.add(Error, "git_failed", fmt.Sprintf("cannot run git: %v", err), "")
		return false
	}
	r.Git = &GitInfo{Path: gitPath, Version: ver}
	if gitcfg.VersionLess(v, gitcfg.MinPushDetect) {
		r.add(Warning, "git_too_old", fmt.Sprintf("git %s is older than 2.28, so jingle cannot tell when a push succeeded; push plays on attempt instead", ver), "upgrade git")
	}
	return true
}

func (r *Report) checkInstall(env hooks.Env) {
	in, err := hooks.Inspect(env)
	if err != nil {
		r.add(Error, "git_failed", fmt.Sprintf("cannot read git config: %v", err), "")
		return
	}
	r.Inspection = in
	switch {
	case in.StateInstalled && !in.Installed:
		r.add(Error, "hooks_path_overridden", "global core.hooksPath was changed after install, so jingle's hooks don't run", "jingle install --yes")
		return
	case !in.Installed:
		r.add(Error, "not_installed", "jingle's hooks are not installed", "jingle install --yes")
		return
	case !in.ShimsCurrent:
		r.add(Warning, "shims_outdated", "the installed hook scripts differ from what this jingle writes; they still run, but push may play on attempt rather than on success", "jingle install --yes")
	}
	if in.JinglePath != "" && !isExecutable(in.JinglePath) {
		if onPath, err := exec.LookPath("jingle"); err == nil {
			r.add(Warning, "jingle_binary_moved", fmt.Sprintf("the hooks call %s, which no longer exists; they fall back to %s from PATH", in.JinglePath, onPath), "jingle install --yes")
		} else {
			r.add(Error, "jingle_binary_missing", fmt.Sprintf("the hooks call %s, which no longer exists, and jingle is not on PATH, so nothing plays", in.JinglePath), "run `jingle install --yes` with the jingle binary the hooks should use")
		}
	}
}

func (r *Report) checkConfig(env hooks.Env) {
	cfg, err := config.Load(env.Paths)
	if err != nil {
		r.add(Error, "config_invalid", err.Error(), "fix or delete "+env.Paths.ConfigFile())
		return
	}
	r.Config = cfg.Flatten()
	for _, ev := range config.Events {
		if !cfg.Enabled || !cfg.Event(ev).Enabled {
			continue
		}
		switch path := cfg.SoundPath(env.Paths, ev); {
		case path == "":
			r.add(Error, "no_sound", "no sound set for "+ev, "jingle config set "+ev+".sound <file>")
		case !isFile(path):
			r.add(Error, "sound_missing", "sound file for "+ev+" not found: "+path, "jingle config set "+ev+".sound <file>")
		}
	}
}

func (r *Report) checkPlayer(env hooks.Env) {
	if p, ok := event.DetectPlayer(); ok {
		r.Player = &p
		return
	}
	if v := env.Getenv(player.EnvOverride); v != "" {
		r.add(Error, "no_player", fmt.Sprintf("%s is set to %q, which is not an executable", player.EnvOverride, v), "fix or unset "+player.EnvOverride)
		return
	}
	r.add(Error, "no_player", "no audio player found (macOS: afplay; Linux: pw-play, paplay, aplay, or ffplay)", "install one of the listed players")
}

func (r *Report) checkRepo(env hooks.Env, root string) {
	info := &RepoInfo{Path: root}
	r.Repo = info
	val, scope, ok, err := env.Git.RepoHooksPath(root)
	if err != nil {
		r.add(Error, "git_failed", fmt.Sprintf("cannot read core.hooksPath in %s: %v", root, err), "")
		return
	}
	if !ok {
		return // no value anywhere: git uses .git/hooks, and jingle isn't installed (reported above)
	}
	info.HooksPath, info.HooksPathScope = &val, scope
	if hooks.PointsAtHooksDir(env, val, root) {
		return
	}
	if scope == "global" || scope == "system" {
		// The same value `jingle status` sees; already reported as not
		// installed or overridden.
		return
	}
	if isHusky(val) {
		info.Husky = true
		r.add(Error, "repo_hooks_path_override",
			fmt.Sprintf("husky manages git hooks in %s (core.hooksPath = %s, %s scope), so jingle's hooks don't run there", root, val, scope),
			"none yet: husky support is planned; until then jingle stays silent in this repo")
		return
	}
	r.add(Error, "repo_hooks_path_override",
		fmt.Sprintf("%s sets core.hooksPath = %s (%s scope), which overrides jingle's global hooks, so they don't run there", root, val, scope),
		fmt.Sprintf("git -C %s config --unset core.hooksPath (the hooks in %s then stop running)", shellQuote(root), val))
}

// isHusky reports whether a core.hooksPath value is husky's: .husky/_ (v9)
// or .husky (v4 to v8).
func isHusky(val string) bool {
	clean := filepath.ToSlash(filepath.Clean(val))
	return clean == ".husky/_" || clean == ".husky" ||
		strings.HasSuffix(clean, "/.husky/_") || strings.HasSuffix(clean, "/.husky")
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || fi.Mode()&0o111 != 0 // no exec bits on Windows
}

// shellQuote quotes p for a copy-pasteable fix when it needs it.
func shellQuote(p string) string {
	if !strings.ContainsAny(p, " \t'\"$`\\") {
		return p
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}
