package hooks

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/fsutil"
	"github.com/OPDhaker/jingle/internal/gitcfg"
)

// HooksPathKey is the git config key jingle takes over.
const HooksPathKey = "core.hooksPath"

// Env is everything install/uninstall/inspect need from the outside world.
type Env struct {
	Paths      config.Paths
	Git        gitcfg.Git
	JinglePath string // the binary the shims should call
	Home       string
	Getenv     func(string) string
	Now        func() time.Time
}

// Change is one step install or uninstall took (or, with --dry-run, would take).
type Change struct {
	Action string `json:"action"` // write_state, remove_state, write_hook, remove_hook, remove_dir, set_git_config, unset_git_config, remove_git_section, remove_file
	Target string `json:"target"` // a path, git config key, or section
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Result is the outcome of Install or Uninstall.
type Result struct {
	// Status is installed|uninstalled, unchanged, or would_change (dry run).
	Status            string   `json:"status"`
	HooksDir          string   `json:"hooks_dir"`
	PreviousHooksPath *string  `json:"previous_hooks_path"`
	ChainDir          string   `json:"chain_dir"`
	Changes           []Change `json:"changes"`
	Notes             []string `json:"notes,omitempty"`
}

// Install points the global core.hooksPath at jingle's shims, saving the
// previous value so the shims can chain to it and uninstall can restore it.
// It is idempotent: a second run reports "unchanged".
func Install(env Env, dryRun bool) (Result, error) {
	hooksDir := env.Paths.HooksDir()
	res := Result{HooksDir: hooksDir, Changes: []Change{}}

	cur, curSet, err := env.Git.GetGlobal(HooksPathKey)
	if err != nil {
		return res, err
	}
	old, err := config.LoadState(env.Paths)
	if err != nil {
		return res, err
	}
	ours := curSet && samePath(expandHome(cur, env.Home), hooksDir)

	want := config.State{
		Installed:   true,
		HooksDir:    hooksDir,
		JinglePath:  env.JinglePath,
		InstalledAt: env.Now().UTC().Truncate(time.Second),
	}
	switch {
	case ours && old.Installed:
		// Re-install: keep what the first install recorded.
		want.PreviousHooksPath = old.PreviousHooksPath
		want.GitconfigFile = old.GitconfigFile
		want.GitconfigExisted = old.GitconfigExisted
		want.CoreHadKeys = old.CoreHadKeys
		want.GitconfigNoFinalNewline = old.GitconfigNoFinalNewline
		want.InstalledAt = old.InstalledAt
	case ours:
		// Our path is set but the state is gone; the previous value is
		// unknowable. Be conservative so uninstall never deletes anything.
		want.GitconfigFile = globalConfigFile(env)
		want.GitconfigExisted, want.CoreHadKeys = true, true
		res.Notes = append(res.Notes, "core.hooksPath already pointed at jingle but state.json was missing; uninstall will unset it rather than restore a previous value")
	default:
		if curSet {
			want.PreviousHooksPath = &cur
		}
		want.GitconfigFile = globalConfigFile(env)
		if data, err := os.ReadFile(want.GitconfigFile); err == nil {
			want.GitconfigExisted = true
			want.GitconfigNoFinalNewline = len(data) > 0 && data[len(data)-1] != '\n'
		}
		if want.CoreHadKeys, err = env.Git.GlobalSectionHasKeys("core"); err != nil {
			return res, err
		}
	}
	if want.PreviousHooksPath != nil {
		want.ChainDir = expandHome(*want.PreviousHooksPath, env.Home)
	} else if sys, ok, err := env.Git.GetSystem(HooksPathKey); err != nil {
		return res, err
	} else if ok {
		// Without a global value git used the system one; keep running those hooks.
		want.ChainDir = expandHome(sys, env.Home)
	}
	res.PreviousHooksPath, res.ChainDir = want.PreviousHooksPath, want.ChainDir

	writeState := !sameState(old, want)
	if writeState {
		res.Changes = append(res.Changes, Change{Action: "write_state", Target: env.Paths.StateFile()})
	}
	shims := Shims(ShimConfig{Jingle: env.JinglePath, Chain: want.ChainDir, Self: hooksDir})
	writes, stale, err := diffShims(hooksDir, shims)
	if err != nil {
		return res, err
	}
	for _, h := range writes {
		res.Changes = append(res.Changes, Change{Action: "write_hook", Target: filepath.Join(hooksDir, h)})
	}
	for _, p := range stale {
		res.Changes = append(res.Changes, Change{Action: "remove_hook", Target: p})
	}
	if !ours {
		res.Changes = append(res.Changes, Change{Action: "set_git_config", Target: HooksPathKey, From: cur, To: hooksDir})
	}

	switch {
	case len(res.Changes) == 0:
		res.Status = "unchanged"
		return res, nil
	case dryRun:
		res.Status = "would_change"
		return res, nil
	}

	// State first: if a later step fails, uninstall still knows what to restore.
	if writeState {
		if err := config.SaveState(env.Paths, want); err != nil {
			return res, err
		}
	}
	for _, h := range writes {
		if err := fsutil.WriteFileAtomic(filepath.Join(hooksDir, h), shims[h], 0o755); err != nil {
			return res, err
		}
	}
	for _, p := range stale {
		if err := os.Remove(p); err != nil {
			return res, err
		}
	}
	if !ours {
		if err := env.Git.SetGlobal(HooksPathKey, hooksDir); err != nil {
			return res, err
		}
		// Record the file git actually wrote, in case our guess was wrong.
		if origin, err := env.Git.GlobalOrigin(HooksPathKey); err == nil && origin != want.GitconfigFile {
			want.GitconfigFile = origin
			if err := config.SaveState(env.Paths, want); err != nil {
				return res, err
			}
		}
	}
	res.Status = "installed"
	return res, nil
}

// diffShims returns the hooks whose file is missing or differs, and the
// jingle-managed files in dir that are no longer wanted.
func diffShims(dir string, shims map[string][]byte) (writes, stale []string, err error) {
	for h, data := range shims {
		fi, err := os.Stat(filepath.Join(dir, h))
		if err != nil || fi.Mode()&0o111 == 0 {
			writes = append(writes, h)
			continue
		}
		cur, err := os.ReadFile(filepath.Join(dir, h))
		if err != nil || !bytes.Equal(cur, data) {
			writes = append(writes, h)
		}
	}
	managed, err := managedFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range managed {
		if _, ok := shims[filepath.Base(p)]; !ok {
			stale = append(stale, p)
		}
	}
	sort.Strings(writes)
	return writes, stale, nil
}

// managedFiles lists the jingle shims in dir (a missing dir has none).
func managedFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if data, err := os.ReadFile(p); err == nil && IsManaged(data) {
			out = append(out, p)
		}
	}
	return out, nil
}

// sameState compares states ignoring the install timestamp.
func sameState(a, b config.State) bool {
	a.Version, b.Version = 0, 0
	a.InstalledAt, b.InstalledAt = time.Time{}, time.Time{}
	return reflect.DeepEqual(a, b)
}

// globalConfigFile mirrors how `git config --global` picks the file to write.
func globalConfigFile(env Env) string {
	if f := env.Getenv("GIT_CONFIG_GLOBAL"); f != "" {
		return f
	}
	home := filepath.Join(env.Home, ".gitconfig")
	if fileExists(home) {
		return home
	}
	xdg := env.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(env.Home, ".config")
	}
	if p := filepath.Join(xdg, "git", "config"); fileExists(p) {
		return p
	}
	return home
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// expandHome expands a leading ~/ the way git does for core.hooksPath.
func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}

// samePath compares two paths after cleaning and resolving symlinks.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
