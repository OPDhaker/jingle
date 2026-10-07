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
