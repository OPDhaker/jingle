package hooks

import (
	"bytes"
	"errors"
	"io/fs"
	"os"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/fsutil"
)

// Uninstall undoes Install: it restores the previous global core.hooksPath
// (or unsets it, removing a [core] section or gitconfig file that install
// created), removes the shims, and deletes state.json. Config and sounds are
// kept. It is idempotent: with nothing to undo it reports "unchanged".
func Uninstall(env Env, dryRun bool) (Result, error) {
	hooksDir := env.Paths.HooksDir()
	res := Result{HooksDir: hooksDir, Changes: []Change{}}

	cur, curSet, err := env.Git.GetGlobal(HooksPathKey)
	if err != nil {
		return res, err
	}
	st, err := config.LoadState(env.Paths)
	if err != nil {
		return res, err
	}
	res.PreviousHooksPath, res.ChainDir = st.PreviousHooksPath, st.ChainDir
	ours := curSet && samePath(expandHome(cur, env.Home), hooksDir)

	// Cleanup applies only when unsetting a value install added to a file or
	// section that did not exist before.
	cleanup := false
	switch {
	case ours && st.PreviousHooksPath != nil:
		res.Changes = append(res.Changes, Change{Action: "set_git_config", Target: HooksPathKey, From: cur, To: *st.PreviousHooksPath})
	case ours:
		res.Changes = append(res.Changes, Change{Action: "unset_git_config", Target: HooksPathKey, From: cur})
		if !st.Installed {
			res.Notes = append(res.Notes, "state.json was missing, so no previous core.hooksPath could be restored; unset it instead")
		}
		cleanup = st.Installed
		if cleanup && !st.CoreHadKeys {
			res.Changes = append(res.Changes, Change{Action: "remove_git_section", Target: "core", Note: "only if no core.* keys remain"})
		}
		if cleanup && !st.GitconfigExisted && st.GitconfigFile != "" {
			res.Changes = append(res.Changes, Change{Action: "remove_file", Target: st.GitconfigFile, Note: "only if it is now empty"})
		}
	case st.Installed:
		res.Notes = append(res.Notes, "global core.hooksPath no longer points at jingle ("+cur+"); left it unchanged")
	}

	shims, err := managedFiles(hooksDir)
	if err != nil {
		return res, err
	}
	for _, p := range shims {
		res.Changes = append(res.Changes, Change{Action: "remove_hook", Target: p})
	}
	if len(shims) > 0 {
		res.Changes = append(res.Changes, Change{Action: "remove_dir", Target: hooksDir, Note: "only if empty"})
	}
	if fileExists(env.Paths.StateFile()) {
		res.Changes = append(res.Changes, Change{Action: "remove_state", Target: env.Paths.StateFile()})
	}

	switch {
	case len(res.Changes) == 0:
		res.Status = "unchanged"
		return res, nil
	case dryRun:
		res.Status = "would_change"
		return res, nil
	}

	// Git config first: if it fails, the shims and state are still in place
	// and a retry can finish the job.
	if ours {
		if st.PreviousHooksPath != nil {
			err = env.Git.SetGlobal(HooksPathKey, *st.PreviousHooksPath)
		} else {
			err = env.Git.UnsetGlobal(HooksPathKey)
		}
		if err != nil {
			return res, err
		}
	}
	if cleanup {
		if err := cleanupGitconfig(env, st); err != nil {
			return res, err
		}
	}
	for _, p := range shims {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return res, err
		}
	}
	_ = os.Remove(hooksDir) // fails, harmlessly, if the user keeps other files there
	if err := config.RemoveState(env.Paths); err != nil {
		return res, err
	}
	res.Status = "uninstalled"
	return res, nil
}

// cleanupGitconfig removes what setting core.hooksPath created: an empty
// [core] section (newer git already drops it on unset), the newline git added
// before it, and, if the global config file did not exist before, the
// now-empty file.
func cleanupGitconfig(env Env, st config.State) error {
	if !st.CoreHadKeys {
		has, err := env.Git.GlobalSectionHasKeys("core")
		if err != nil {
			return err
		}
		if !has {
			if err := env.Git.RemoveGlobalSection("core"); err != nil {
				return err
			}
			if st.GitconfigNoFinalNewline && st.GitconfigFile != "" {
				if err := trimFinalNewline(st.GitconfigFile); err != nil {
					return err
				}
			}
		}
	}
	if !st.GitconfigExisted && st.GitconfigFile != "" {
		data, err := os.ReadFile(st.GitconfigFile)
		if err == nil && len(bytes.TrimSpace(data)) == 0 {
			return os.Remove(st.GitconfigFile)
		}
	}
	return nil
}

// trimFinalNewline removes one trailing newline from path, keeping its mode.
func trimFinalNewline(path string) error {
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasSuffix(data, []byte("\n")) {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, data[:len(data)-1], fi.Mode().Perm())
}
