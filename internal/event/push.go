package event

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/gitcfg"
)

// The push sound plays once per successful push. The pre-push shim writes a
// marker named after the git push process (run/push-<pid>) before anything
// is sent; the reference-transaction shim calls PushDone when that process
// moves a remote-tracking ref. A fetch runs in another process, so it never
// matches. Whoever removes the marker first plays, so a push plays at most
// once even if PrePush's fallback and PushDone race.

// markerTTL bounds how long a marker from a push that never finished (dry
// run, network error) is kept.
const markerTTL = time.Hour

// gitVersion is replaced in tests.
var gitVersion = func() ([3]int, error) {
	_, v, err := gitcfg.Git{}.Version()
	return v, err
}

// CodeBadArgs: a hook passed arguments jingle does not understand.
const CodeBadArgs = "bad_args"

// PrePush runs after the pre-push shim marked a push that is about to be
// sent. Normally it leaves the marker for PushDone. It plays right away
// instead when success can't be detected: push.when is "attempt", the push
// goes to a raw URL (git passes the URL as the remote name, and no
// remote-tracking ref will move), or git predates reference-transaction.
// played is true only if a sound started.
func PrePush(paths config.Paths, ppid, remote, url string) (played bool, err error) {
	marker, err := markerPath(paths, ppid)
	if err != nil {
		return false, err
	}
	pruneMarkers(paths, time.Now())
	cfg, err := config.Load(paths)
	if err != nil {
		return false, errorf(CodeConfigInvalid, "%v", err)
	}
	if !playOnAttempt(cfg, remote, url) || !claim(marker) {
		return false, nil
	}
	_, err = Play(paths, "push", Options{})
	return err == nil, err
}

// PushDone runs when the push process that pre-push marked has updated a
// remote-tracking ref, i.e. the push succeeded.
func PushDone(paths config.Paths, ppid string) (played bool, err error) {
	marker, err := markerPath(paths, ppid)
	if err != nil {
		return false, err
	}
	if !claim(marker) {
		return false, nil
	}
	_, err = Play(paths, "push", Options{})
	return err == nil, err
}

func playOnAttempt(cfg config.Config, remote, url string) bool {
	if cfg.Push.When == config.PushAttempt || remote == url {
		return true
	}
	v, err := gitVersion()
	return err == nil && gitcfg.VersionLess(v, gitcfg.MinPushDetect)
}

// markerPath returns the marker for the git push process ppid. ppid must be
// a plain number so it can't point outside the run dir.
func markerPath(paths config.Paths, ppid string) (string, error) {
	if ppid == "" || strings.Trim(ppid, "0123456789") != "" {
		return "", errorf(CodeBadArgs, "invalid process id %q", ppid)
	}
	return filepath.Join(paths.RunDir(), "push-"+ppid), nil
}

// claim removes marker and reports whether this call was the one that did.
func claim(marker string) bool {
	return os.Remove(marker) == nil
}

// pruneMarkers removes push markers older than markerTTL.
func pruneMarkers(paths config.Paths, now time.Time) {
	entries, err := os.ReadDir(paths.RunDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "push-") {
			continue
		}
		if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) > markerTTL {
			_ = os.Remove(filepath.Join(paths.RunDir(), e.Name()))
		}
	}
}
