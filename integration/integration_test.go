// Package integration runs the jingle binary against real git in a sandbox:
// temp HOME, GIT_CONFIG_GLOBAL, and JINGLE_HOME, and a fake audio player
// (JINGLE_PLAYER) that logs what it was asked to play. The real ~/.gitconfig
// is never touched.
package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

var jingleBin string

func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		fmt.Println("integration tests need POSIX sh hooks; Windows support is Phase 3")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "jingle-it-bin")
	if err != nil {
		panic(err)
	}
	jingleBin = filepath.Join(dir, "jingle")
	build := exec.Command("go", "build", "-o", jingleBin, "github.com/OPDhaker/jingle/cmd/jingle")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("building jingle: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type sandbox struct {
	t         *testing.T
	root      string
	env       []string
	gitconfig string
	jingleDir string
	playLog   string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir()) // macOS: /var -> /private/var
	if err != nil {
		t.Fatal(err)
	}
	s := &sandbox{
		t:         t,
		root:      root,
		gitconfig: filepath.Join(root, "gitconfig"),
		jingleDir: filepath.Join(root, "jingle"),
		playLog:   filepath.Join(root, "played.log"),
	}
	home := filepath.Join(root, "home")
	player := filepath.Join(root, "player")
	mkdir(t, home)
	writeExec(t, player, fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$1\" >> '%s'\n", s.playLog))

	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") || strings.HasPrefix(k, "JINGLE_") || strings.HasPrefix(k, "XDG_") || k == "HOME" {
			continue
		}
		s.env = append(s.env, kv)
	}
	s.env = append(s.env,
		"HOME="+home,
		"GIT_CONFIG_GLOBAL="+s.gitconfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"JINGLE_HOME="+s.jingleDir,
		"JINGLE_PLAYER="+player,
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	return s
}

// run runs a command in dir and returns combined output and exit code.
func (s *sandbox) run(dir, name string, args ...string) (string, int) {
	s.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = s.env
	out, err := cmd.CombinedOutput()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
		return string(out), ee.ExitCode()
	} else if err != nil {
		s.t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out), 0
}

func (s *sandbox) git(dir string, args ...string) (string, int) {
	s.t.Helper()
	return s.run(dir, "git", args...)
}

func (s *sandbox) mustGit(dir string, args ...string) string {
	s.t.Helper()
	out, code := s.git(dir, args...)
	if code != 0 {
		s.t.Fatalf("git %v: exit %d\n%s", args, code, out)
	}
	return out
}

// jingle runs the binary with stdout only (stderr is checked via exit code).
func (s *sandbox) jingle(args ...string) (string, int) {
	s.t.Helper()
	cmd := exec.Command(jingleBin, args...)
	cmd.Dir = s.root
	cmd.Env = s.env
	out, err := cmd.Output()
	if ee := (*exec.ExitError)(nil); errors.As(err, &ee) {
		return string(out), ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return string(out), 0
}

func (s *sandbox) mustJingle(args ...string) map[string]any {
	s.t.Helper()
	out, code := s.jingle(append(args, "--json")...)
	if code != 0 {
		s.t.Fatalf("jingle %v: exit %d\n%s", args, code, out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		s.t.Fatalf("jingle %v: stdout not JSON: %q", args, out)
	}
	return m
}

// installWithSounds installs and points both events at generated WAV files.
// The cooldown is off so tests can count every sound.
func (s *sandbox) installWithSounds() {
	s.t.Helper()
	s.mustJingle("install", "--yes")
	s.mustJingle("config", "set", "cooldown", "0")
	for _, ev := range []string{"commit", "push"} {
		wav := filepath.Join(s.root, "tag.wav")
		writeWAV(s.t, wav)
		s.mustJingle("config", "set", ev+".sound", wav)
	}
}

func (s *sandbox) repo(name string) string {
	s.t.Helper()
	dir := filepath.Join(s.root, name)
	s.mustGit(s.root, "init", "-q", "-b", "main", dir)
	return dir
}

// played returns the player log lines, waiting up to 3s for at least want.
// With want == 0 it waits briefly so a stray background play would show up.
func (s *sandbox) played(want int) []string {
	s.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	if want == 0 {
		deadline = time.Now().Add(500 * time.Millisecond)
	}
	var lines []string
	for {
		data, _ := os.ReadFile(s.playLog)
		lines = strings.Fields(string(data))
		if (want > 0 && len(lines) >= want) || time.Now().After(deadline) {
			return lines
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *sandbox) wantPlayed(events ...string) {
	s.t.Helper()
	lines := s.played(len(events))
	if len(lines) != len(events) {
		s.t.Fatalf("played %d sounds %v, want %v", len(lines), lines, events)
	}
	for i, ev := range events {
		if !strings.HasPrefix(filepath.Base(lines[i]), ev+"-") {
			s.t.Fatalf("sound %d = %s, want the %s sound", i, lines[i], ev)
		}
	}
}

func TestInstallDryRunConfirmAndIdempotence(t *testing.T) {
	s := newSandbox(t)

	if res := s.mustJingle("install", "--dry-run"); res["status"] != "would_change" {
		t.Fatalf("dry run status %v", res["status"])
	}
	if _, err := os.Stat(s.gitconfig); err == nil {
		t.Fatal("dry run wrote the gitconfig")
	}
	if _, err := os.Stat(s.jingleDir); err == nil {
		t.Fatal("dry run created the jingle dir")
	}
	if _, code := s.jingle("install"); code != 2 {
		t.Fatalf("install without --yes: exit %d, want 2", code)
	}

	if res := s.mustJingle("install", "--yes"); res["status"] != "installed" {
		t.Fatalf("status %v", res["status"])
	}
	if res := s.mustJingle("install", "--yes"); res["status"] != "unchanged" {
		t.Fatalf("second install status %v", res["status"])
	}
	got := strings.TrimSpace(s.mustGit(s.root, "config", "--global", "core.hooksPath"))
	if got != filepath.Join(s.jingleDir, "hooks") {
		t.Fatalf("core.hooksPath = %q", got)
	}
	st := s.mustJingle("status")
	if st["installed"] != true || st["shims_current"] != true {
		t.Fatalf("status: %v", st)
	}
}

func TestCommitPlaysAndRepoHooksStillRun(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	repo := s.repo("repo")
	hooks := filepath.Join(repo, ".git", "hooks")
	preMarker := filepath.Join(s.root, "pre-commit.ran")
	postMarker := filepath.Join(s.root, "post-commit.ran")
	failFlag := filepath.Join(s.root, "fail")
	writeExec(t, filepath.Join(hooks, "pre-commit"), fmt.Sprintf(
		"#!/bin/sh\ntouch '%s'\n[ -f '%s' ] && exit 1\nexit 0\n", preMarker, failFlag))
	writeExec(t, filepath.Join(hooks, "post-commit"), fmt.Sprintf("#!/bin/sh\ntouch '%s'\n", postMarker))

	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "one")
	mustExist(t, preMarker)
	mustExist(t, postMarker)
	s.wantPlayed("commit")

	// The repo's pre-commit fails: the commit must fail and nothing plays.
	writeFile(t, failFlag, "")
	if out, code := s.git(repo, "commit", "-q", "--allow-empty", "-m", "two"); code == 0 {
		t.Fatalf("commit succeeded despite failing pre-commit hook\n%s", out)
	}
	s.wantPlayed("commit")

	// Turned off: commits work, no sound.
	remove(t, failFlag)
	s.mustJingle("config", "set", "enabled", "false")
	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "three")
	s.wantPlayed("commit")

	// play ignores the switch and reports it.
	if res := s.mustJingle("play", "commit"); res["enabled"] != false {
		t.Fatalf("play: %v", res)
	}
	s.wantPlayed("commit", "commit")
}

func TestPushPassesStdinAndExitStatus(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	remote := filepath.Join(s.root, "remote.git")
	s.mustGit(s.root, "init", "-q", "--bare", "-b", "main", remote)
	repo := filepath.Join(s.root, "clone")
	s.mustGit(s.root, "clone", "-q", remote, repo)
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", "one")

	stdinCopy := filepath.Join(s.root, "pre-push.stdin")
	argsCopy := filepath.Join(s.root, "pre-push.args")
	failFlag := filepath.Join(s.root, "fail")
	writeExec(t, filepath.Join(repo, ".git", "hooks", "pre-push"), fmt.Sprintf(
		"#!/bin/sh\necho \"$@\" > '%s'\ncat > '%s'\n[ -f '%s' ] && exit 1\nexit 0\n", argsCopy, stdinCopy, failFlag))

	head := strings.TrimSpace(s.mustGit(repo, "rev-parse", "HEAD"))
	wantStdin := fmt.Sprintf("refs/heads/main %s refs/heads/main %s\n", head, strings.Repeat("0", len(head)))

	writeFile(t, failFlag, "")
	if out, code := s.git(repo, "push", "-q", "origin", "main"); code == 0 {
		t.Fatalf("push succeeded despite failing pre-push hook\n%s", out)
	}
	s.wantPlayed()
	if got := readFile(t, stdinCopy); got != wantStdin {
		t.Fatalf("pre-push stdin = %q, want %q", got, wantStdin)
	}
	if got := readFile(t, argsCopy); got != "origin "+remote+"\n" {
		t.Fatalf("pre-push args = %q", got)
	}

	remove(t, failFlag)
	s.mustGit(repo, "push", "-q", "origin", "main")
	s.wantPlayed("push")

	// Everything up to date: nothing is sent, so no sound.
	s.mustGit(repo, "push", "-q", "origin", "main")
	s.wantPlayed("push")
}

// pushSetup returns a clone of a bare remote with one unpushed commit.
func (s *sandbox) pushSetup() (remote, repo string) {
	s.t.Helper()
	remote = filepath.Join(s.root, "remote.git")
	s.mustGit(s.root, "init", "-q", "--bare", "-b", "main", remote)
	repo = filepath.Join(s.root, "clone")
	s.mustGit(s.root, "clone", "-q", remote, repo)
	s.commitQuietly(repo, "one")
	return remote, repo
}

// commitQuietly commits with hooks off, so setup commits play nothing.
func (s *sandbox) commitQuietly(repo, msg string) {
	s.t.Helper()
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "--allow-empty", "-m", msg)
}

func TestPushPlaysOnlyOnSuccess(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	remote, repo := s.pushSetup()

	// The remote rejects the push after pre-push passed: no sound.
	reject := filepath.Join(remote, "hooks", "pre-receive")
	writeExec(t, reject, "#!/bin/sh\necho rejected >&2\nexit 1\n")
	if out, code := s.git(repo, "push", "-q", "origin", "main"); code == 0 {
		t.Fatalf("push succeeded despite rejecting pre-receive\n%s", out)
	}
	s.wantPlayed()

	// A dry run sends nothing: no sound, and the marker it leaves behind
	// must not make a later fetch play.
	remove(t, reject)
	s.mustGit(repo, "push", "-q", "--dry-run", "origin", "main")
	s.mustGit(repo, "fetch", "-q", "origin")
	s.wantPlayed()

	s.mustGit(repo, "push", "-q", "origin", "main")
	s.wantPlayed("push")

	// A raw path updates no remote-tracking ref, so it plays on attempt.
	s.commitQuietly(repo, "two")
	s.mustGit(repo, "push", "-q", remote, "main")
	s.wantPlayed("push", "push")

	// The dry run's marker is still there; uninstall clears the run dir.
	s.mustJingle("uninstall", "--yes")
	if _, err := os.Stat(filepath.Join(s.jingleDir, "run")); err == nil {
		t.Fatal("run dir left behind after uninstall")
	}
}

func TestPushWhenAttemptPlaysOnRejectedPush(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	s.mustJingle("config", "set", "push.when", "attempt")
	remote, repo := s.pushSetup()
	writeExec(t, filepath.Join(remote, "hooks", "pre-receive"), "#!/bin/sh\nexit 1\n")
	if _, code := s.git(repo, "push", "-q", "origin", "main"); code == 0 {
		t.Fatal("push succeeded despite rejecting pre-receive")
	}
	s.wantPlayed("push")
}

func TestUserReferenceTransactionHookStillRuns(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	_, repo := s.pushSetup()
	log := filepath.Join(s.root, "rt.log")
	writeExec(t, filepath.Join(repo, ".git", "hooks", "reference-transaction"), fmt.Sprintf(
		"#!/bin/sh\n[ \"$1\" = committed ] || exit 0\nwhile read -r old new ref; do echo \"$ref\" >> '%s'; done\n", log))

	s.commitQuietly(repo, "two") // hooks off: must not log
	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "three")
	if got := readFile(t, log); !strings.Contains(got, "refs/heads/main\n") {
		t.Fatalf("after commit the user hook logged %q", got)
	}
	remove(t, log)
	s.mustGit(repo, "push", "-q", "origin", "main")
	s.wantPlayed("commit", "push")
	if got := readFile(t, log); !strings.Contains(got, "refs/remotes/origin/main\n") {
		t.Fatalf("during push the user hook logged %q", got)
	}
}

// change commits an edit to a file with hooks off.
func (s *sandbox) change(repo, file, msg string) {
	s.t.Helper()
	p := filepath.Join(repo, file)
	old, _ := os.ReadFile(p)
	writeFile(s.t, p, string(old)+msg+"\n")
	s.mustGit(repo, "add", file)
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", msg)
}

func TestReplayedCommitsAreQuiet(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	repo := s.repo("repo")
	for _, m := range []string{"a", "b", "c", "d"} {
		s.change(repo, m+".txt", m)
	}

	s.mustGit(repo, "rebase", "-q", "--force-rebase", "HEAD~3")
	s.wantPlayed()

	s.mustGit(repo, "revert", "--no-edit", "HEAD~2..HEAD")
	s.wantPlayed()

	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "checkout", "-q", "-b", "side")
	s.change(repo, "side.txt", "side")
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "checkout", "-q", "main")
	s.mustGit(repo, "cherry-pick", "side")
	s.wantPlayed()

	// A rebase in a linked worktree keeps its state in the worktree's git dir.
	wt := filepath.Join(s.root, "wt")
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "worktree", "add", "-q", "-b", "wt", wt)
	s.change(wt, "w1.txt", "w1")
	s.change(wt, "w2.txt", "w2")
	s.mustGit(wt, "rebase", "-q", "--force-rebase", "HEAD~2")
	s.wantPlayed()

	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "plain")
	s.wantPlayed("commit")
}

func TestPullRebaseIsQuiet(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	remote, repo := s.pushSetup()
	s.mustGit(repo, "-c", "core.hooksPath=/dev/null", "push", "-q", "origin", "main")
	other := filepath.Join(s.root, "other")
	s.mustGit(s.root, "-c", "core.hooksPath=/dev/null", "clone", "-q", remote, other)
	s.change(other, "theirs.txt", "theirs")
	s.mustGit(other, "-c", "core.hooksPath=/dev/null", "push", "-q", "origin", "main")

	s.change(repo, "ours.txt", "ours")
	s.mustGit(repo, "pull", "-q", "--rebase", "origin", "main")
	s.wantPlayed()
}

func TestCooldownPlaysBurstOnce(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	s.mustJingle("config", "set", "cooldown", "60")
	repo := s.repo("repo")
	for _, m := range []string{"one", "two", "three"} {
		s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", m)
	}
	s.wantPlayed("commit")

	// play is for testing setup: it ignores the cooldown.
	s.mustJingle("play", "commit")
	s.wantPlayed("commit", "commit")
}

// doctorReport is the part of the doctor/status JSON these tests read.
type doctorReport struct {
	Problems []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"problems"`
	Repo *struct {
		Path           string  `json:"path"`
		HooksPath      *string `json:"hooks_path"`
		HooksPathScope string  `json:"hooks_path_scope"`
		Husky          bool    `json:"husky"`
	} `json:"repo"`
}

func (r doctorReport) codes() []string {
	var c []string
	for _, p := range r.Problems {
		c = append(c, p.Code)
	}
	return c
}

// doctor runs `jingle <args> --json` in dir with extra env and returns the
// parsed report, stderr, and exit code.
func (s *sandbox) doctor(dir string, env []string, args ...string) (doctorReport, string, int) {
	s.t.Helper()
	cmd := exec.Command(jingleBin, append(args, "--json")...)
	cmd.Dir = dir
	cmd.Env = append(append([]string{}, s.env...), env...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		ee := (*exec.ExitError)(nil)
		if !errors.As(err, &ee) {
			s.t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	var r doctorReport
	if stdout.Len() > 0 {
		if err := json.Unmarshal([]byte(stdout.String()), &r); err != nil {
			s.t.Fatalf("jingle %v: stdout not JSON: %q", args, stdout.String())
		}
	}
	return r, stderr.String(), code
}

func TestDoctorReportsAndExitCodes(t *testing.T) {
	s := newSandbox(t)

	r, stderr, code := s.doctor(s.root, nil, "doctor")
	if code != 1 || !strings.Contains(stderr, "problems_found") {
		t.Fatalf("fresh doctor: exit %d, stderr %q", code, stderr)
	}
	want := []string{"not_installed", "no_sound", "no_sound"}
	if got := r.codes(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("fresh doctor problems %v, want %v", got, want)
	}
	if r.Repo != nil {
		t.Fatalf("checked a repo outside any repo: %+v", r.Repo)
	}
	// status reports the same problems but always exits 0.
	if st, _, code := s.doctor(s.root, nil, "status"); code != 0 || len(st.Problems) != len(want) {
		t.Fatalf("status: exit %d, problems %v", code, st.codes())
	}

	s.installWithSounds()
	repo := s.repo("repo")
	r, _, code = s.doctor(repo, nil, "doctor")
	if code != 0 || len(r.Problems) != 0 {
		t.Fatalf("healthy doctor: exit %d, problems %v", code, r.codes())
	}
	if r.Repo == nil || r.Repo.Path != repo || r.Repo.HooksPathScope != "global" {
		t.Fatalf("repo info %+v", r.Repo)
	}

	// husky sets a repo-local core.hooksPath, which overrides ours.
	s.mustGit(repo, "config", "core.hooksPath", ".husky/_")
	for _, args := range [][]string{{"doctor"}, {"doctor", "--repo", repo}} {
		dir := repo
		if len(args) > 1 {
			dir = s.root
		}
		r, _, code := s.doctor(dir, nil, args...)
		if code != 1 || strings.Join(r.codes(), ",") != "repo_hooks_path_override" {
			t.Fatalf("%v: exit %d, problems %v", args, code, r.codes())
		}
		if !r.Repo.Husky || r.Repo.HooksPathScope != "local" || !strings.Contains(r.Problems[0].Message, "husky") {
			t.Fatalf("%v: repo %+v, problem %+v", args, r.Repo, r.Problems[0])
		}
	}
	// status only looks at the global setup.
	if _, _, code := s.doctor(repo, nil, "status"); code != 0 {
		t.Fatalf("status in husky repo: exit %d", code)
	}

	if _, stderr, code := s.doctor(s.root, nil, "doctor", "--repo", s.root); code != 1 || !strings.Contains(stderr, "not_a_repo") {
		t.Fatalf("--repo on a non-repo: exit %d, stderr %q", code, stderr)
	}

	// A deleted sound and a bad player override.
	sounds, _ := filepath.Glob(filepath.Join(s.jingleDir, "sounds", "commit-*"))
	for _, f := range sounds {
		remove(t, f)
	}
	r, _, code = s.doctor(s.root, []string{"JINGLE_PLAYER=/nope"}, "doctor")
	if code != 1 || strings.Join(r.codes(), ",") != "sound_missing,no_player" {
		t.Fatalf("exit %d, problems %v", code, r.codes())
	}
}

func TestDoctorBinaryMissing(t *testing.T) {
	s := newSandbox(t)
	copyBin := filepath.Join(s.root, "bin", "jingle")
	data, err := os.ReadFile(jingleBin)
	if err != nil {
		t.Fatal(err)
	}
	writeExec(t, copyBin, string(data))
	if out, code := s.run(s.root, copyBin, "install", "--yes"); code != 0 {
		t.Fatalf("install via copy: exit %d\n%s", code, out)
	}
	for _, ev := range []string{"commit", "push"} {
		wav := filepath.Join(s.root, "tag.wav")
		writeWAV(t, wav)
		s.mustJingle("config", "set", ev+".sound", wav)
	}
	remove(t, copyBin)

	r, _, code := s.doctor(s.root, []string{"PATH=/usr/bin:/bin"}, "doctor")
	if code != 1 || strings.Join(r.codes(), ",") != "jingle_binary_missing" {
		t.Fatalf("exit %d, problems %v", code, r.codes())
	}
}

func TestChainsToPreviousGlobalHooksPath(t *testing.T) {
	s := newSandbox(t)
	prev := filepath.Join(s.root, "prev-hooks")
	prevMarker := filepath.Join(s.root, "prev.ran")
	repoMarker := filepath.Join(s.root, "repo.ran")
	writeExec(t, filepath.Join(prev, "pre-commit"), fmt.Sprintf("#!/bin/sh\ntouch '%s'\n", prevMarker))
	original := fmt.Sprintf("[core]\n\thooksPath = %s\n", prev)
	writeFile(t, s.gitconfig, original)

	s.installWithSounds()
	repo := s.repo("repo")
	// git ignored repo hooks before install (global hooksPath was set); it still must.
	writeExec(t, filepath.Join(repo, ".git", "hooks", "pre-commit"), fmt.Sprintf("#!/bin/sh\ntouch '%s'\n", repoMarker))

	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "one")
	mustExist(t, prevMarker)
	if _, err := os.Stat(repoMarker); err == nil {
		t.Fatal("repo hook ran although the previous global hooksPath shadowed it")
	}
	s.wantPlayed("commit")

	s.mustJingle("uninstall", "--yes")
	if got := readFile(t, s.gitconfig); got != original {
		t.Fatalf("gitconfig after uninstall:\n%q\nwant\n%q", got, original)
	}
}

func TestLinkedWorktreeRunsRepoHook(t *testing.T) {
	s := newSandbox(t)
	s.installWithSounds()
	repo := s.repo("repo")
	marker := filepath.Join(s.root, "pre-commit.ran")
	writeExec(t, filepath.Join(repo, ".git", "hooks", "pre-commit"), fmt.Sprintf("#!/bin/sh\ntouch '%s'\n", marker))
	s.mustGit(repo, "commit", "-q", "--allow-empty", "-m", "base")
	remove(t, marker)
	s.played(1)
	remove(t, s.playLog)

	wt := filepath.Join(s.root, "wt")
	s.mustGit(repo, "worktree", "add", "-q", "-b", "side", wt)
	s.mustGit(wt, "commit", "-q", "--allow-empty", "-m", "in worktree")
	mustExist(t, marker)
	s.wantPlayed("commit")
}

func TestUninstallRestoresGitconfigByteForByte(t *testing.T) {
	cases := map[string]*string{
		"absent":           nil,
		"no core section":  ptr("[user]\n\tname = someone\n"),
		"core other keys":  ptr("# my config\n[core]\n\teditor = vim\n[user]\n\tname = someone\n"),
		"previous path":    ptr("[core]\n\thooksPath = ~/my-hooks\n"),
		"no final newline": ptr("[alias]\n\tco = checkout"),
	}
	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSandbox(t)
			if original != nil {
				writeFile(t, s.gitconfig, *original)
			}
			s.mustJingle("install", "--yes")
			got := strings.TrimSpace(s.mustGit(s.root, "config", "--global", "core.hooksPath"))
			if got != filepath.Join(s.jingleDir, "hooks") {
				t.Fatalf("core.hooksPath after install = %q", got)
			}

			if res := s.mustJingle("uninstall", "--yes"); res["status"] != "uninstalled" {
				t.Fatalf("status %v", res["status"])
			}
			data, err := os.ReadFile(s.gitconfig)
			switch {
			case original == nil && err == nil:
				t.Fatalf("gitconfig should not exist, has %q", data)
			case original != nil && string(data) != *original:
				t.Fatalf("gitconfig after uninstall:\n%q\nwant\n%q", data, *original)
			}
			if res := s.mustJingle("uninstall", "--yes"); res["status"] != "unchanged" {
				t.Fatalf("second uninstall status %v", res["status"])
			}
			if _, err := os.Stat(filepath.Join(s.jingleDir, "hooks")); err == nil {
				t.Fatal("hooks dir left behind")
			}
		})
	}
}

func ptr(s string) *string { return &s }

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExec(t *testing.T, path, data string) {
	t.Helper()
	writeFile(t, path, data)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s: %v", filepath.Base(path), err)
	}
}

// writeWAV writes a tiny valid 8 kHz mono 8-bit WAV (10 ms of silence).
func writeWAV(t *testing.T, path string) {
	t.Helper()
	const n = 80
	b := []byte("RIFF")
	b = le32(b, 36+n)
	b = append(b, "WAVEfmt "...)
	b = le32(b, 16)
	b = append(b, 1, 0, 1, 0) // PCM, mono
	b = le32(b, 8000)
	b = le32(b, 8000)
	b = append(b, 1, 0, 8, 0) // block align, bits per sample
	b = append(b, "data"...)
	b = le32(b, n)
	for range n {
		b = append(b, 128)
	}
	writeFile(t, path, string(b))
}

func le32(b []byte, v uint32) []byte {
	return append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
