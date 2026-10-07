// Package config owns jingle's on-disk locations, the user config file
// (config.toml), and tool-owned machine state (state.json).
//
// User preferences live in config.toml; anything jingle records about the
// machine (e.g. the core.hooksPath it replaced) lives in state.json. The two
// are never mixed, so a user or agent resetting config cannot lose the value
// uninstall needs to restore.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const appName = "jingle"

// Paths are the directories jingle reads and writes.
type Paths struct {
	ConfigDir string // config.toml
	DataDir   string // hooks/, sounds/, state.json
}

func (p Paths) ConfigFile() string { return filepath.Join(p.ConfigDir, "config.toml") }
func (p Paths) StateFile() string  { return filepath.Join(p.DataDir, "state.json") }
func (p Paths) HooksDir() string   { return filepath.Join(p.DataDir, "hooks") }
func (p Paths) SoundsDir() string  { return filepath.Join(p.DataDir, "sounds") }

// ResolvePaths returns the paths for the current user and OS.
func ResolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	return resolvePaths(os.Getenv, runtime.GOOS, home)
}

// resolvePaths applies, in order:
//  1. JINGLE_HOME: everything under one directory (tests, portable installs).
//  2. Windows: %AppData%\jingle for config, %LocalAppData%\jingle for data.
//  3. Elsewhere: XDG dirs, defaulting to ~/.config and ~/.local/share on
//     macOS too, since that is where developers expect CLI tool config.
func resolvePaths(getenv func(string) string, goos, home string) (Paths, error) {
	if dir := getenv("JINGLE_HOME"); dir != "" {
		return Paths{ConfigDir: dir, DataDir: dir}, nil
	}
	if goos == "windows" {
		appData, localAppData := getenv("AppData"), getenv("LocalAppData")
		if appData == "" || localAppData == "" {
			return Paths{}, errors.New("AppData or LocalAppData is not set")
		}
		return Paths{
			ConfigDir: filepath.Join(appData, appName),
			DataDir:   filepath.Join(localAppData, appName),
		}, nil
	}
	configHome := getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	dataHome := getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	return Paths{
		ConfigDir: filepath.Join(configHome, appName),
		DataDir:   filepath.Join(dataHome, appName),
	}, nil
}
