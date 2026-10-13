package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/OPDhaker/jingle/internal/doctor"
	"github.com/OPDhaker/jingle/internal/event"
	"github.com/OPDhaker/jingle/internal/output"
)

// TestCodesDocumented keeps three lists in step: the codes the source
// returns, the tables in output.ErrorCodes and doctor.Codes (which feed
// `jingle schema` and --help), and the tables in docs/json.md.
func TestCodesDocumented(t *testing.T) {
	// Error codes: every literal passed to output.Errorf or gitError in the
	// CLI, plus the event codes `play` passes through.
	used := map[string]bool{"usage": true} // output.Printer.Error's fallback
	errRe := regexp.MustCompile(`(?:Errorf|gitError)\("([a-z_]+)"`)
	for _, src := range sources(t, ".") {
		for _, m := range errRe.FindAllStringSubmatch(src, -1) {
			used[m[1]] = true
		}
	}
	for _, c := range []string{event.CodeUnknownEvent, event.CodeConfigInvalid, event.CodeNoSound,
		event.CodeSoundMissing, event.CodeSoundNotFound, event.CodeNoPlayer, event.CodePlayerFailed} {
		used[c] = true
	}
	var table []string
	for _, c := range output.ErrorCodes {
		table = append(table, c.Code)
	}
	compareSets(t, "error codes used in internal/cli", keys(used), "output.ErrorCodes", table)

	// Problem codes: every r.add(<severity>, "<code>" in the doctor.
	sev := map[string][]doctor.Severity{}
	addRe := regexp.MustCompile(`r\.add\((Error|Warning), "([a-z_]+)"`)
	for _, src := range sources(t, "../doctor") {
		for _, m := range addRe.FindAllStringSubmatch(src, -1) {
			s := doctor.Severity(strings.ToLower(m[1]))
			if !slices.Contains(sev[m[2]], s) {
				sev[m[2]] = append(sev[m[2]], s)
			}
		}
	}
	table = nil
	for _, c := range doctor.Codes {
		table = append(table, c.Code)
		got := slices.Clone(sev[c.Code])
		slices.Sort(got)
		want := slices.Clone(c.Severities)
		slices.Sort(want)
		if len(got) > 0 && !slices.Equal(got, want) {
			t.Errorf("problem %s: doctor reports severities %v, doctor.Codes says %v", c.Code, got, want)
		}
	}
	compareSets(t, "problems reported by internal/doctor", keys(sev), "doctor.Codes", table)

	// docs/json.md has the same rows, word for word.
	doc, err := os.ReadFile("../../docs/json.md")
	if err != nil {
		t.Fatal(err)
	}
	var docRows, want []string
	rowRe := regexp.MustCompile("^\\| `[a-z_]+` \\| (\\d|error|warning)")
	for _, line := range strings.Split(string(doc), "\n") {
		if rowRe.MatchString(line) {
			docRows = append(docRows, line)
		}
	}
	for _, c := range output.ErrorCodes {
		want = append(want, fmt.Sprintf("| `%s` | %d | %s |", c.Code, c.Exit, c.Meaning))
	}
	for _, c := range doctor.Codes {
		sevs := make([]string, len(c.Severities))
		for i, s := range c.Severities {
			sevs[i] = string(s)
		}
		want = append(want, fmt.Sprintf("| `%s` | %s | %s |", c.Code, strings.Join(sevs, " / "), c.Meaning))
	}
	compareSets(t, "code rows in docs/json.md", docRows, "the code tables", want)
}

func sources(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(data))
	}
	return out
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func compareSets(t *testing.T, aName string, a []string, bName string, b []string) {
	t.Helper()
	for _, x := range a {
		if !slices.Contains(b, x) {
			t.Errorf("in %s but not in %s: %s", aName, bName, x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			t.Errorf("in %s but not in %s: %s", bName, aName, x)
		}
	}
}
