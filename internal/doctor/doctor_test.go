package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/hooks"
	"github.com/OPDhaker/jingle/internal/player"
)

func TestIsHusky(t *testing.T) {
	for val, want := range map[string]bool{
		".husky/_":           true,
		".husky":             true,
		"./.husky/_":         true,
		"/abs/repo/.husky/_": true,
		".husky/_/":          true,
		".githooks":          false,
		"husky":              false,
		"/x/not.husky":       false,
	} {
		if got := isHusky(val); got != want {
			t.Errorf("isHusky(%q) = %v, want %v", val, got, want)
		}
	}
}

func TestErrorsCountsOnlyErrors(t *testing.T) {
	r := Report{Problems: []Problem{{Severity: Warning}, {Severity: Error}, {Severity: Warning}}}
	if r.Errors() != 1 {
		t.Fatalf("Errors() = %d", r.Errors())
	}
	if (Report{Problems: []Problem{{Severity: Warning}}}).Errors() != 0 {
		t.Fatal("a warning counted as an error")
	}
}

// sandboxEnv isolates git and jingle from the real machine.
func sandboxEnv(t *testing.T) hooks.Env {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return hooks.Env{
		Paths:  config.Paths{ConfigDir: dir, DataDir: dir},
		Home:   dir,
		Getenv: os.Getenv,
		Now:    time.Now,
	}
}

func TestNoPlayerNamesOverride(t *testing.T) {
	env := sandboxEnv(t)
	t.Setenv(player.EnvOverride, "/no/such/player")
	r := Check(env, "")
	for _, p := range r.Problems {
		if p.Code == "no_player" {
			if p.Severity != Error || !strings.Contains(p.Message, "/no/such/player") {
				t.Fatalf("no_player = %+v", p)
			}
			return
		}
	}
	t.Fatalf("no no_player problem in %+v", r.Problems)
}

func TestSoundMissingSeverity(t *testing.T) {
	env := sandboxEnv(t)
	dir := t.TempDir()
	have, gone := filepath.Join(dir, "have.wav"), filepath.Join(dir, "gone.wav")
	if err := os.WriteFile(have, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Push.Enabled = false
	for _, tt := range []struct {
		sounds []string
		code   string
		sev    Severity
	}{
		{nil, "no_sound", Error},
		{[]string{gone}, "sound_missing", Error},
		{[]string{have, gone}, "sound_missing", Warning},
		{[]string{have}, "", ""},
	} {
		cfg.Commit.Sounds = tt.sounds
		if err := config.Save(env.Paths, cfg); err != nil {
			t.Fatal(err)
		}
		var got []Problem
		for _, p := range Check(env, "").Problems {
			if p.Code == "no_sound" || p.Code == "sound_missing" {
				got = append(got, p)
			}
		}
		if tt.code == "" {
			if len(got) != 0 {
				t.Errorf("%v: unexpected %+v", tt.sounds, got)
			}
			continue
		}
		if len(got) != 1 || got[0].Code != tt.code || got[0].Severity != tt.sev {
			t.Errorf("%v: got %+v, want one %s %s", tt.sounds, got, tt.sev, tt.code)
		}
	}
}
