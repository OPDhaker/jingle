package event

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
	"github.com/OPDhaker/jingle/internal/fsutil"
)

// Hooks run in parallel during a burst of commits, so the cooldown check
// and the stamp update happen under a lock. The lock is a directory because
// os.Mkdir is atomic on every OS.
const (
	lockStale = 5 * time.Second        // a lock this old was left by a crash
	lockWait  = 200 * time.Millisecond // give up after this: a burst is in progress
)

// takeTurn reports whether event may play given a cooldown of d, and if so
// records the current time as its last play. The clock is read only once
// the lock is held, so stamps are written in order. When jingle can't
// coordinate (the run dir can't be created), it lets the sound play.
func takeTurn(paths config.Paths, event string, d time.Duration, clock func() time.Time) bool {
	if d <= 0 {
		return true
	}
	run := paths.RunDir()
	if err := os.MkdirAll(run, 0o755); err != nil {
		return true
	}
	lock := filepath.Join(run, "lock-"+event)
	if !acquire(lock) {
		return false
	}
	defer func() { _ = os.Remove(lock) }()

	now := clock()
	stamp := filepath.Join(run, "last-"+event)
	if data, err := os.ReadFile(stamp); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			// A stamp far in the future means the clock moved back: play.
			if since := now.Sub(time.Unix(0, n)); since < d && since > -d {
				return false
			}
		}
	}
	_ = fsutil.WriteFileAtomic(stamp, []byte(strconv.FormatInt(now.UnixNano(), 10)+"\n"), 0o644)
	return true
}

func acquire(lock string) bool {
	deadline := time.Now().Add(lockWait)
	for {
		if os.Mkdir(lock, 0o755) == nil {
			return true
		}
		if fi, err := os.Stat(lock); err == nil && time.Since(fi.ModTime()) > lockStale {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
