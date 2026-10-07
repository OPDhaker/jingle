package event

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/player"
)

// setup returns paths with a push sound configured and a fake player that
// appends each played file to the returned log.
func setup(t *testing.T) (config.Paths, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake player is a sh script")
	}
	dir := t.TempDir()
	p := config.Paths{ConfigDir: dir, DataDir: dir}
	log := filepath.Join(dir, "played.log")
	fake := filepath.Join(dir, "player")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + log + "'\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(player.EnvOverride, fake)

	sound := filepath.Join(dir, "push.wav")
	if err := os.WriteFile(sound, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Push.Sound = sound
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	stubGitVersion(t, [3]int{2, 54, 0})
	return p, log
}

func stubGitVersion(t *testing.T, v [3]int) {
	t.Helper()
	old := gitVersion
	gitVersion = func() ([3]int, error) { return v, nil }
	t.Cleanup(func() { gitVersion = old })
}

func mark(t *testing.T, p config.Paths, ppid string) string {
	t.Helper()
	m := filepath.Join(p.RunDir(), "push-"+ppid)
	if err := os.MkdirAll(p.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return m
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestPrePushLeavesMarkerForPushDone(t *testing.T) {
	p, _ := setup(t)
	m := mark(t, p, "123")
	if played, err := PrePush(p, "123", "origin", "git@example.com:x.git"); played || err != nil {
		t.Fatalf("played=%v err=%v", played, err)
	}
	if !exists(m) {
		t.Fatal("PrePush removed the marker")
	}
	if played, err := PushDone(p, "123"); !played || err != nil {
		t.Fatalf("PushDone: played=%v err=%v", played, err)
	}
	if exists(m) {
		t.Fatal("marker left after PushDone")
	}
	// A second transaction in the same push (or a fetch with a reused pid)
	// finds no marker and stays silent.
	if played, _ := PushDone(p, "123"); played {
		t.Fatal("played twice for one push")
	}
}

func TestPrePushFallsBackToAttempt(t *testing.T) {
	tests := map[string]struct {
		when        string
		remote, url string
		git         [3]int
	}{
		"push.when attempt": {config.PushAttempt, "origin", "/r.git", [3]int{2, 54, 0}},
		"raw URL":           {config.PushSuccess, "/r.git", "/r.git", [3]int{2, 54, 0}},
		"old git":           {config.PushSuccess, "origin", "/r.git", [3]int{2, 27, 9}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p, log := setup(t)
			stubGitVersion(t, tt.git)
			cfg, _ := config.Load(p)
			cfg.Push.When = tt.when
			if err := config.Save(p, cfg); err != nil {
				t.Fatal(err)
			}
			m := mark(t, p, "77")
			if played, err := PrePush(p, "77", tt.remote, tt.url); !played || err != nil {
				t.Fatalf("played=%v err=%v", played, err)
			}
			if exists(m) {
				t.Fatal("fallback did not claim the marker")
			}
			if played, _ := PushDone(p, "77"); played {
				t.Fatal("PushDone played after the fallback already did")
			}
			waitLines(t, log, 1)
		})
	}
}

func TestPushRejectsBadPID(t *testing.T) {
	p, _ := setup(t)
	for _, pid := range []string{"", "../x", "12a", "-1"} {
		if _, err := PushDone(p, pid); err == nil {
			t.Errorf("PushDone(%q) accepted", pid)
		}
		if _, err := PrePush(p, pid, "o", "u"); err == nil {
			t.Errorf("PrePush(%q) accepted", pid)
		}
	}
}

func TestPruneMarkers(t *testing.T) {
	p, _ := setup(t)
	old := mark(t, p, "1")
	fresh := mark(t, p, "2")
	other := filepath.Join(p.RunDir(), "keep-me")
	if err := os.WriteFile(other, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * markerTTL)
	for _, f := range []string{old, other} {
		if err := os.Chtimes(f, past, past); err != nil {
			t.Fatal(err)
		}
	}
	pruneMarkers(p, time.Now())
	if exists(old) || !exists(fresh) || !exists(other) {
		t.Fatalf("old=%v fresh=%v other=%v", exists(old), exists(fresh), exists(other))
	}
}

func waitLines(t *testing.T, log string, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, _ := os.ReadFile(log)
		got := strings.Fields(string(data))
		if len(got) >= want || time.Now().After(deadline) {
			if len(got) != want {
				t.Fatalf("played %d times, want %d", len(got), want)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
