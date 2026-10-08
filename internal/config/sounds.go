package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OPDhaker/jingle/internal/fsutil"
)

// SoundExts are the audio file extensions jingle accepts.
var SoundExts = []string{".mp3", ".wav", ".aiff", ".aif", ".m4a", ".ogg", ".flac"}

var (
	ErrSoundNotFound    = errors.New("sound file not found")
	ErrUnsupportedSound = errors.New("unsupported audio format")
)

// ImportSound copies src into the sounds dir as "<event>-<name>", so moving
// or deleting the original later doesn't break playback. If that name holds
// different audio (another sound with the same file name), the copy becomes
// "<event>-<stem>-2<ext>", "-3", and so on. It returns the copy's path and
// whether a file was written; importing identical content again writes
// nothing and returns the existing copy.
func ImportSound(p Paths, event, src string) (dest string, wrote bool, err error) {
	abs, err := filepath.Abs(src)
	if err != nil {
		return "", false, err
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return "", false, fmt.Errorf("%w: %s", ErrSoundNotFound, src)
	}
	if ext := strings.ToLower(filepath.Ext(abs)); !slices.Contains(SoundExts, ext) {
		return "", false, fmt.Errorf("%w %q (supported: %s)", ErrUnsupportedSound, filepath.Ext(abs), strings.Join(SoundExts, " "))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", false, err
	}
	name := filepath.Base(abs)
	if !strings.HasPrefix(name, event+"-") {
		name = event + "-" + name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		dest = filepath.Join(p.SoundsDir(), name)
		if i > 1 {
			dest = filepath.Join(p.SoundsDir(), fmt.Sprintf("%s-%d%s", stem, i, ext))
		}
		cur, err := os.ReadFile(dest)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return dest, true, fsutil.WriteFileAtomic(dest, data, 0o644)
		case err != nil:
			return "", false, err
		case bytes.Equal(cur, data):
			return dest, false, nil
		}
	}
}

// RemoveImportedSound deletes path if it is a copy jingle made for event.
// User files outside the sounds dir are never touched.
func RemoveImportedSound(p Paths, event, path string) error {
	if path == "" {
		return nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(p.SoundsDir(), path)
	}
	if filepath.Dir(path) != filepath.Clean(p.SoundsDir()) || !strings.HasPrefix(filepath.Base(path), event+"-") {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
