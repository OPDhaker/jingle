package hooks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShimQuoting(t *testing.T) {
	got := shQuote("/it's a dir")
	if got != `'/it'\''s a dir'` {
		t.Fatalf("got %s", got)
	}
}

func TestShimsAreValidShAndManaged(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	c := ShimConfig{Jingle: "/opt/my jingle/bin/jingle", Chain: "/it's/hooks", Self: "/self", Run: "/my run's dir"}
	for name, data := range Shims(c) {
		if !IsManaged(data) {
			t.Errorf("%s: missing marker", name)
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(sh, "-n", p).CombinedOutput(); err != nil {
			t.Errorf("%s: sh -n: %v\n%s\n%s", name, err, out, data)
		}
		_, wantPlay := events[name]
		if strings.Contains(string(data), "hook-event") != wantPlay {
			t.Errorf("%s: hook-event presence wrong", name)
		}
		if wantPlay && !strings.Contains(string(data), `run='/my run'\''s dir'`) {
			t.Errorf("%s: run dir missing or unquoted", name)
		}
	}
}

// reference-transaction fires for every ref update in every git command, so
// unless a push is marked it must hand off to the user's hook untouched.
func TestReferenceTransactionFastPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("running shims under Git for Windows is verified in Phase 3")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	dir := t.TempDir()
	chain := filepath.Join(dir, "chain")
	run := filepath.Join(dir, "run")
	log := filepath.Join(dir, "log")
	if err := os.MkdirAll(chain, 0o755); err != nil {
		t.Fatal(err)
	}
	user := "#!/bin/sh\nprintf '%s|' \"$*\" >> '" + log + "'\ncat >> '" + log + "'\nexit 3\n"
	if err := os.WriteFile(filepath.Join(chain, "reference-transaction"), []byte(user), 0o755); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "shim")
	data := RenderShim("reference-transaction", ShimConfig{Jingle: "/nonexistent", Chain: chain, Self: dir, Run: run})
	if err := os.WriteFile(shim, data, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"prepared", "committed"} {
		cmd := exec.Command(sh, shim, phase)
		cmd.Stdin = strings.NewReader("a b refs/remotes/origin/main\n")
		err := cmd.Run()
		if ee := (*exec.ExitError)(nil); !errors.As(err, &ee) || ee.ExitCode() != 3 {
			t.Fatalf("%s: err %v, want exit 3 from the user hook", phase, err)
		}
	}
	got, _ := os.ReadFile(log)
	want := "prepared|a b refs/remotes/origin/main\ncommitted|a b refs/remotes/origin/main\n"
	if string(got) != want {
		t.Fatalf("user hook saw %q, want %q", got, want)
	}
}

func TestIsManaged(t *testing.T) {
	if IsManaged([]byte("#!/bin/sh\necho user hook\n")) {
		t.Fatal("user hook reported as managed")
	}
}
