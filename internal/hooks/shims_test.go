package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
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
	c := ShimConfig{Jingle: "/opt/my jingle/bin/jingle", Chain: "/it's/hooks", Self: "/self"}
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
		wantPlay := name == "post-commit" || name == "pre-push"
		if strings.Contains(string(data), "hook-event") != wantPlay {
			t.Errorf("%s: hook-event presence wrong", name)
		}
	}
}

func TestIsManaged(t *testing.T) {
	if IsManaged([]byte("#!/bin/sh\necho user hook\n")) {
		t.Fatal("user hook reported as managed")
	}
}
