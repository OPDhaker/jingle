package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/OPDhaker/jingle/internal/fsutil"
)

// State is what jingle records about the machine when it installs: enough to
// chain to the user's previous hooks and to undo the install exactly. It is
// tool-owned and kept apart from the user's config (see package doc).
type State struct {
	Version int `json:"version"`
	// Installed is true between a successful install and uninstall.
	Installed bool   `json:"installed"`
	HooksDir  string `json:"hooks_dir,omitempty"`
	// JinglePath is the binary the shims call first.
	JinglePath string `json:"jingle_path,omitempty"`
	// PreviousHooksPath is the global core.hooksPath before install; nil if unset.
	PreviousHooksPath *string `json:"previous_hooks_path"`
	// ChainDir is the hooks dir the shims chain to; "" means each repo's own hooks.
	ChainDir string `json:"chain_dir,omitempty"`
	// GitconfigFile is the global git config file install wrote to.
	GitconfigFile string `json:"gitconfig_file,omitempty"`
	// GitconfigExisted and CoreHadKeys let uninstall remove the file or the
	// [core] section if install was what created them.
	GitconfigExisted bool `json:"gitconfig_existed"`
	CoreHadKeys      bool `json:"core_had_keys"`
	// GitconfigNoFinalNewline: git adds a newline before appending [core];
	// uninstall drops it again.
	GitconfigNoFinalNewline bool      `json:"gitconfig_no_final_newline,omitempty"`
	InstalledAt             time.Time `json:"installed_at,omitzero"`
}

// LoadState reads state.json. A missing file yields a zero State.
func LoadState(p Paths) (State, error) {
	var s State
	data, err := os.ReadFile(p.StateFile())
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(data, &s)
	return s, err
}

// SaveState writes state.json atomically.
func SaveState(p Paths, s State) error {
	s.Version = 1
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(p.StateFile(), append(data, '\n'), 0o644)
}

// RemoveState deletes state.json; a missing file is not an error.
func RemoveState(p Paths) error {
	err := os.Remove(p.StateFile())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
