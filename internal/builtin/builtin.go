// Package builtin holds the sounds that ship inside the jingle binary, so a
// fresh install plays something with no files supplied. They are synthesized
// in code (see synth.go): no audio assets in the repo, nothing to license.
//
// A config sound entry "builtin:<name>" names one of them. Players need a
// file, so Ensure renders the sound into the sounds dir on first use.
package builtin

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OPDhaker/jingle/internal/fsutil"
)

// Prefix marks a sounds list entry as a built-in sound.
const Prefix = "builtin:"

type sound struct {
	name   string
	doc    string
	render func() []float64
}

// sounds are listed in display order.
var sounds = []sound{
	{"chime", "two-note rising bell", chime},
	{"fanfare", "quick rising brass arpeggio", fanfare},
	{"harmony", "two voices resolving to a chord", harmony},
	{"boom", "snare hit into an 808 drop", boom},
}

// Parse reports whether entry is a built-in entry, and its name. The name
// is not checked; see Exists.
func Parse(entry string) (name string, ok bool) {
	return strings.CutPrefix(entry, Prefix)
}

// Names lists the built-in sound names (without Prefix).
func Names() []string {
	names := make([]string, len(sounds))
	for i, s := range sounds {
		names[i] = s.name
	}
	return names
}

// Entries lists the built-in sounds as config entries ("builtin:<name>").
func Entries() []string {
	names := Names()
	for i, n := range names {
		names[i] = Prefix + n
	}
	return names
}

// Exists reports whether name is a built-in sound.
func Exists(name string) bool { return find(name) != nil }

// Doc describes the sound, for help text and `jingle schema`.
func Doc(name string) string {
	if s := find(name); s != nil {
		return s.doc
	}
	return ""
}

func find(name string) *sound {
	for i := range sounds {
		if sounds[i].name == name {
			return &sounds[i]
		}
	}
	return nil
}

// WAV renders the named sound as a WAV file.
func WAV(name string) ([]byte, bool) {
	s := find(name)
	if s == nil {
		return nil, false
	}
	return encodeWAV(master(s.render())), true
}

// File is where Ensure puts the named sound inside soundsDir.
func File(soundsDir, name string) string {
	return filepath.Join(soundsDir, "builtin", name+".wav")
}

// Ensure writes the named sound to File(soundsDir, name) unless that file
// already holds exactly these bytes, and returns its path. Rewriting on any
// difference means a jingle upgrade that changes a sound takes effect.
func Ensure(soundsDir, name string) (string, error) {
	data, ok := WAV(name)
	if !ok {
		return "", fmt.Errorf("unknown built-in sound %q (have: %s)", name, strings.Join(Names(), ", "))
	}
	path := File(soundsDir, name)
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, data) {
		return path, nil
	}
	return path, fsutil.WriteFileAtomic(path, data, 0o644)
}
