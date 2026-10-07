package cli

import (
	"errors"
	"os"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/gitcfg"
	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/output"
)

// paths resolves jingle's directories or returns a typed error.
func paths() (config.Paths, error) {
	p, err := config.ResolvePaths()
	if err != nil {
		return p, output.Errorf("paths_unavailable", output.ExitError, "cannot locate config dirs: %v", err)
	}
	return p, nil
}

// hooksEnv builds the environment for install/uninstall/status.
func hooksEnv() (hooks.Env, error) {
	p, err := paths()
	if err != nil {
		return hooks.Env{}, err
	}
	// Not symlink-resolved: a package manager's stable symlink (e.g.
	// /opt/homebrew/bin/jingle) survives upgrades; its target does not.
	exe, err := os.Executable()
	if err != nil {
		return hooks.Env{}, output.Errorf("io_error", output.ExitError, "cannot locate the jingle binary: %v", err)
	}
	home, _ := os.UserHomeDir()
	return hooks.Env{Paths: p, JinglePath: exe, Home: home, Getenv: os.Getenv, Now: time.Now}, nil
}

// gitError maps an error from git or the filesystem to a typed CLI error.
func gitError(code string, err error) error {
	if errors.Is(err, gitcfg.ErrNotFound) {
		return output.Errorf("git_not_found", output.ExitError, "git is not installed or not on PATH")
	}
	return output.Errorf(code, output.ExitError, "%v", err)
}
