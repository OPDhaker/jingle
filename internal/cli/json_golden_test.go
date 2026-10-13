package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/json golden files")

// TestJSONGolden pins the --json output of every command (stdout, stderr,
// and exit code) to testdata/json. The JSON output is a public contract
// (docs/json.md): if this test fails, the schema changed. Additive changes
// are fine; regenerate with
//
//	go test ./internal/cli -run TestJSONGolden -update
//
// Renaming, removing, or retyping a field also needs a schema_version bump
// and a "Schema history" entry in docs/json.md.
func TestJSONGolden(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install writes POSIX sh hooks; Windows is not supported yet")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root, err := filepath.EvalSymlinks(t.TempDir()) // macOS: /var -> /private/var
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"home", "repo", "src"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	player := filepath.Join(root, "player")
	writeTestFile(t, player, "#!/bin/sh\nexit 0\n", 0o755)
	writeTestFile(t, filepath.Join(root, "src", "tag.wav"), "RIFF fake", 0o644)
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "XDG_") {
			t.Setenv(k, "")
			_ = os.Unsetenv(k)
		}
	}
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("JINGLE_HOME", filepath.Join(root, "jingle"))
	t.Setenv("JINGLE_PLAYER", player)
	if out, err := exec.Command("git", "init", "-q", filepath.Join(root, "repo")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	wav := filepath.Join(root, "src", "tag.wav")

	// Order matters: each step runs against the state the previous one left.
	steps := []struct {
		name string
		args []string
	}{
		{"version", []string{"version"}},
		{"status-not-installed", []string{"status"}},
		{"install-dry-run", []string{"install", "--dry-run"}},
		{"install", []string{"install", "--yes"}},
		{"install-unchanged", []string{"install", "--yes"}},
		{"config-set-sounds", []string{"config", "set", "commit.sounds", wav}},
		{"config-add-sounds", []string{"config", "add", "push.sounds", wav}},
		{"config-remove-sounds", []string{"config", "remove", "push.sounds", "push-tag.wav"}},
		{"config-set-unchanged", []string{"config", "set", "volume", "100"}},
		{"config-get", []string{"config", "get", "push.when"}},
		{"config-list", []string{"config", "list"}},
		{"play", []string{"play", "commit"}},
		{"status-installed", []string{"status"}},
		{"doctor-problems", []string{"doctor", "--repo", filepath.Join(root, "repo")}},
		{"error-invalid-key", []string{"config", "get", "nope"}},
		{"error-confirmation-required", []string{"uninstall"}},
		{"error-usage", []string{"status", "--no-such-flag"}},
		{"uninstall-purge-dry-run", []string{"uninstall", "--yes", "--purge", "--dry-run"}},
		{"uninstall-purge", []string{"uninstall", "--yes", "--purge"}},
		{"uninstall-unchanged", []string{"uninstall", "--yes", "--purge"}},
	}
	for i, st := range steps {
		code, stdout, stderr := run(append(st.args, "--json")...)
		got := normalizeGolden(t, root, code, stdout, stderr)
		file := filepath.Join("testdata", "json", fmt.Sprintf("%02d-%s.json", i+1, st.name))
		if *update {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, file, got, 0o644)
			continue
		}
		want, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v (run with -update to create it)", st.name, err)
		}
		if got != string(want) {
			t.Errorf("jingle %s --json changed (%s):\n--- got\n%s--- want\n%s", strings.Join(st.args, " "), file, got, want)
		}
	}
}

// normalizeGolden renders one command's result as stable JSON: temp paths
// become $ROOT, and values that depend on the machine (versions, the test
// binary's path) become "<machine>".
func normalizeGolden(t *testing.T, root string, code int, stdout, stderr string) string {
	t.Helper()
	parse := func(s string) any {
		if s == "" {
			return nil
		}
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatalf("output is not JSON: %q", s)
		}
		return scrub(v, root)
	}
	doc := map[string]any{"exit": code, "stdout": parse(stdout), "stderr": parse(stderr)}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// machineKeys hold values that differ between machines and builds.
var machineKeys = map[string]bool{
	"jingle_executable": true, "jingle_path": true,
	"version": true, "commit": true, "go": true, "os": true, "arch": true,
}

func scrub(v any, root string) any {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			switch {
			case k == "git" && x != nil:
				v[k] = map[string]any{"path": "<machine>", "version": "<machine>"}
			case machineKeys[k] && isString(x):
				v[k] = "<machine>"
			default:
				v[k] = scrub(x, root)
			}
		}
		return v
	case []any:
		for i := range v {
			v[i] = scrub(v[i], root)
		}
		return v
	case string:
		return strings.ReplaceAll(v, root, "$ROOT")
	}
	return v
}

func isString(v any) bool { _, ok := v.(string); return ok }

func writeTestFile(t *testing.T, path, data string, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), perm); err != nil {
		t.Fatal(err)
	}
}
