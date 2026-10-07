package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/OPDhaker/jingle/internal/output"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errw bytes.Buffer
	code = Run(args, &out, &errw)
	return code, out.String(), errw.String()
}

func TestVersionJSON(t *testing.T) {
	code, stdout, _ := run("version", "--json")
	if code != output.ExitOK {
		t.Fatalf("exit %d", code)
	}
	var v versionInfo
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("stdout not JSON: %q", stdout)
	}
	if v.Version == "" || v.OS == "" {
		t.Fatalf("missing fields: %+v", v)
	}
}

func TestVersionPlain(t *testing.T) {
	code, stdout, _ := run("version")
	if code != output.ExitOK || !strings.HasPrefix(stdout, "jingle ") {
		t.Fatalf("exit %d, stdout %q", code, stdout)
	}
}

func TestUsageErrorsAreJSONWithExit2(t *testing.T) {
	for _, args := range [][]string{
		{"--json", "--no-such-flag"},
		{"no-such-command", "--json"},
		{"version", "extra-arg", "--json"},
	} {
		code, stdout, stderr := run(args...)
		if code != output.ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, output.ExitUsage)
		}
		if stdout != "" {
			t.Errorf("%v: unexpected stdout %q", args, stdout)
		}
		var env struct {
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(stderr), &env); err != nil || env.Error.Code != "usage" {
			t.Errorf("%v: stderr %q", args, stderr)
		}
	}
}

func TestInstallUninstallNeedYes(t *testing.T) {
	t.Setenv("JINGLE_HOME", t.TempDir())
	for _, cmd := range []string{"install", "uninstall"} {
		code, stdout, stderr := run(cmd, "--json")
		if code != output.ExitUsage || stdout != "" || !strings.Contains(stderr, `"confirmation_required"`) {
			t.Errorf("%s: exit %d stdout %q stderr %q", cmd, code, stdout, stderr)
		}
	}
}

func TestHookEventNeverFailsOrPrints(t *testing.T) {
	t.Setenv("JINGLE_HOME", t.TempDir())
	for _, args := range [][]string{
		{"hook-event"},
		{"hook-event", "bogus", "--json"},
		{"hook-event", "commit"}, // no sound configured
	} {
		code, stdout, stderr := run(args...)
		if code != output.ExitOK || stdout != "" || stderr != "" {
			t.Errorf("%v: exit %d stdout %q stderr %q", args, code, stdout, stderr)
		}
	}
}

func TestConfigErrors(t *testing.T) {
	t.Setenv("JINGLE_HOME", t.TempDir())
	tests := []struct {
		args []string
		code string
		exit int
	}{
		{[]string{"config", "get", "nope"}, "invalid_key", output.ExitUsage},
		{[]string{"config", "set", "enabled", "maybe"}, "invalid_value", output.ExitUsage},
		{[]string{"config", "set", "commit.sound", "/no/such/file.mp3"}, "sound_not_found", output.ExitError},
		{[]string{"play", "merge"}, "unknown_event", output.ExitUsage},
		{[]string{"play", "commit"}, "no_sound", output.ExitError},
	}
	for _, tt := range tests {
		code, _, stderr := run(append(tt.args, "--json")...)
		var env struct {
			Error struct{ Code string } `json:"error"`
		}
		_ = json.Unmarshal([]byte(stderr), &env)
		if code != tt.exit || env.Error.Code != tt.code {
			t.Errorf("%v: exit %d code %q, want %d %q", tt.args, code, env.Error.Code, tt.exit, tt.code)
		}
	}
}

func TestConfigSetIsIdempotent(t *testing.T) {
	t.Setenv("JINGLE_HOME", t.TempDir())
	for i, want := range []bool{true, false} {
		code, stdout, _ := run("config", "set", "push.enabled", "off", "--json")
		var v struct{ Changed bool }
		if err := json.Unmarshal([]byte(stdout), &v); err != nil || code != 0 || v.Changed != want {
			t.Fatalf("run %d: exit %d stdout %q", i, code, stdout)
		}
	}
}
