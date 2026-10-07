package event

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/OPDhaker/jingle/internal/config"
)

func TestTakeTurn(t *testing.T) {
	dir := t.TempDir()
	p := config.Paths{ConfigDir: dir, DataDir: dir}
	now := time.Now()
	at := func(off time.Duration) func() time.Time {
		return func() time.Time { return now.Add(off) }
	}
	const d = 3 * time.Second

	if !takeTurn(p, "commit", d, at(0)) {
		t.Fatal("first play refused")
	}
	if takeTurn(p, "commit", d, at(time.Second)) {
		t.Fatal("played inside the cooldown")
	}
	if !takeTurn(p, "push", d, at(time.Second)) {
		t.Fatal("cooldown leaked across events")
	}
	if !takeTurn(p, "commit", d, at(d)) {
		t.Fatal("refused after the cooldown")
	}
	if takeTurn(p, "commit", d, at(d-time.Second)) {
		t.Fatal("played just behind the last stamp")
	}
	if !takeTurn(p, "commit", d, at(-time.Hour)) {
		t.Fatal("refused after the clock moved back")
	}
	if !takeTurn(p, "commit", 0, at(0)) {
		t.Fatal("cooldown 0 refused")
	}
}

func TestTakeTurnBurstPlaysOnce(t *testing.T) {
	dir := t.TempDir()
	p := config.Paths{ConfigDir: dir, DataDir: dir}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		played int
	)
	for range 20 {
		wg.Go(func() {
			if takeTurn(p, "commit", time.Minute, time.Now) {
				mu.Lock()
				played++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if played != 1 {
		t.Fatalf("burst of 20 played %d times", played)
	}
}

func TestTakeTurnBreaksStaleLock(t *testing.T) {
	dir := t.TempDir()
	p := config.Paths{ConfigDir: dir, DataDir: dir}
	lock := filepath.Join(p.RunDir(), "lock-commit")
	if err := os.MkdirAll(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	if takeTurn(p, "commit", time.Minute, time.Now) {
		t.Fatal("played while another process held the lock")
	}
	past := time.Now().Add(-2 * lockStale)
	if err := os.Chtimes(lock, past, past); err != nil {
		t.Fatal(err)
	}
	if !takeTurn(p, "commit", time.Minute, time.Now) {
		t.Fatal("stale lock not broken")
	}
	if _, err := os.Stat(lock); err == nil {
		t.Fatal("lock not released")
	}
}
