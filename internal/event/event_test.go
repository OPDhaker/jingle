package event

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OPDhaker/jingle/internal/config"
)

func stubPick(t *testing.T, i int) {
	t.Helper()
	old := pick
	pick = func(n int) int { return min(i, n-1) }
	t.Cleanup(func() { pick = old })
}

// setSounds points push at the given files, creating those named in exist.
func setSounds(t *testing.T, p config.Paths, files []string, exist ...string) {
	t.Helper()
	for _, f := range exist {
		if err := os.WriteFile(f, []byte("RIFF"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Push.Sounds = files
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
}

func code(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestPlayPicksAmongExistingSounds(t *testing.T) {
	p, _ := setup(t)
	dir := t.TempDir()
	a, gone, b := filepath.Join(dir, "a.wav"), filepath.Join(dir, "gone.wav"), filepath.Join(dir, "b.wav")
	setSounds(t, p, []string{a, gone, b}, a, b)

	opts := Options{IgnoreCooldown: true, Wait: true}
	for i, want := range []string{a, b} {
		stubPick(t, i)
		res, err := Play(p, "push", opts)
		if err != nil || res.Sound != want || res.Sounds != 3 {
			t.Fatalf("pick %d: sound=%s sounds=%d err=%v; want %s (missing files skipped)", i, res.Sound, res.Sounds, err, want)
		}
	}

	opts.Sound = "b.wav"
	stubPick(t, 0)
	if res, err := Play(p, "push", opts); err != nil || res.Sound != b {
		t.Fatalf("--sound b.wav played %s, %v", res.Sound, err)
	}
	opts.Sound = "gone.wav"
	if _, err := Play(p, "push", opts); code(err) != CodeSoundMissing {
		t.Fatalf("missing chosen sound: %v", err)
	}
	opts.Sound = "nope.wav"
	if _, err := Play(p, "push", opts); code(err) != CodeSoundNotFound {
		t.Fatalf("unknown chosen sound: %v", err)
	}
}

func TestPlayAllSoundsMissingOrNone(t *testing.T) {
	p, _ := setup(t)
	opts := Options{IgnoreCooldown: true, Wait: true}
	setSounds(t, p, []string{filepath.Join(t.TempDir(), "gone.wav")})
	if _, err := Play(p, "push", opts); code(err) != CodeSoundMissing {
		t.Fatalf("all missing: %v", err)
	}
	setSounds(t, p, nil)
	if _, err := Play(p, "push", opts); code(err) != CodeNoSound {
		t.Fatalf("none set: %v", err)
	}
}
