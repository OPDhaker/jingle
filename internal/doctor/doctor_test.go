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
