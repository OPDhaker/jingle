// Package gitcfg runs the git commands jingle needs: reading the git version
// and reading/writing the user's global (and system) config.
//
// All writes go through `git config --global`, so git decides which file is
// the global one (GIT_CONFIG_GLOBAL, ~/.gitconfig, or the XDG location).
package gitcfg

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotFound means git is not on PATH.
var ErrNotFound = errors.New("git not found on PATH")

// Git runs git commands. The zero value uses "git" from PATH.
type Git struct {
	Bin string
}

// Path returns the absolute path of the git binary.
func (g Git) Path() (string, error) {
	p, err := exec.LookPath(g.bin())
	if err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

func (g Git) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	return "git"
}

// run executes git with args outside of any repository, so a repo-local
// config never leaks into global/system reads. It returns stdout and the
// exit code; err is set only if git could not be run or failed unexpectedly.
func (g Git) run(args ...string) (string, int, error) {
	cmd := exec.Command(g.bin(), args...)
	cmd.Dir = os.TempDir()
	cmd.Env = append(repoFreeEnv(), "LC_ALL=C")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0, nil
	case errors.As(err, &ee):
		return out.String(), ee.ExitCode(), nil
	case errors.Is(err, exec.ErrNotFound):
		return "", -1, ErrNotFound
	default:
		return "", -1, err
	}
}

// repoFreeEnv is the environment minus variables that point git at a repo
// (set, for example, when jingle runs inside a hook).
func repoFreeEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE":
			continue
		}
		env = append(env, kv)
	}
	return env
}

// must runs git and turns any non-zero exit into an error.
func (g Git) must(args ...string) (string, error) {
	out, code, err := g.run(args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git %s: exit %d", strings.Join(args, " "), code)
	}
	return out, nil
}

// MinPushDetect is the first git version with the reference-transaction
// hook, which jingle needs to tell a successful push from an attempt.
var MinPushDetect = [3]int{2, 28, 0}

// VersionLess reports whether version a is older than b.
func VersionLess(a, b [3]int) bool {
	for i := range 3 {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// Version returns git's version string and its [major, minor, patch].
func (g Git) Version() (string, [3]int, error) {
	out, err := g.must("--version")
	if err != nil {
		return "", [3]int{}, err
	}
	m := versionRE.FindStringSubmatch(out)
	if m == nil {
		return "", [3]int{}, fmt.Errorf("cannot parse %q", strings.TrimSpace(out))
	}
	var v [3]int
	for i := range 3 {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return m[0], v, nil
}

// get reads key from one scope ("--global" or "--system").
func (g Git) get(scope, key string) (string, bool, error) {
	out, code, err := g.run("config", scope, "--get", key)
	if err != nil {
		return "", false, err
	}
	switch code {
	case 0:
		return strings.TrimSuffix(out, "\n"), true, nil
	case 1: // key not set
		return "", false, nil
	default:
		return "", false, fmt.Errorf("git config %s --get %s: exit %d", scope, key, code)
	}
}

// GetGlobal reads key from the global config. ok is false if it is unset.
func (g Git) GetGlobal(key string) (val string, ok bool, err error) {
	return g.get("--global", key)
}

// GetSystem reads key from the system config. ok is false if it is unset.
func (g Git) GetSystem(key string) (val string, ok bool, err error) {
	return g.get("--system", key)
}

// SetGlobal sets key in the global config.
func (g Git) SetGlobal(key, val string) error {
	_, err := g.must("config", "--global", key, val)
	return err
}

// UnsetGlobal removes key from the global config. Unsetting a missing key is not an error.
func (g Git) UnsetGlobal(key string) error {
	_, code, err := g.run("config", "--global", "--unset-all", key)
	if err != nil {
		return err
	}
	if code != 0 && code != 5 { // 5: key not present
		return fmt.Errorf("git config --global --unset-all %s: exit %d", key, code)
	}
	return nil
}

// GlobalOrigin returns the file that defines key in the global config.
func (g Git) GlobalOrigin(key string) (string, error) {
	out, err := g.must("config", "--global", "--show-origin", "--get", key)
	if err != nil {
		return "", err
	}
	origin, _, _ := strings.Cut(out, "\t")
	return strings.TrimPrefix(origin, "file:"), nil
}

// GlobalSectionHasKeys reports whether the global config sets any key in section.
func (g Git) GlobalSectionHasKeys(section string) (bool, error) {
	_, code, err := g.run("config", "--global", "--get-regexp", "^"+regexp.QuoteMeta(section)+`\.`)
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

// RemoveGlobalSection removes an (empty) section header from the global
// config. A missing section is not an error.
func (g Git) RemoveGlobalSection(section string) error {
	_, _, err := g.run("config", "--global", "--remove-section", section)
	return err
}
