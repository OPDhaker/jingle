package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func testPaths(t *testing.T) Paths {
	d := t.TempDir()
	return Paths{ConfigDir: d, DataDir: d}
}

func TestLoadMissingIsDefault(t *testing.T) {
	cfg, err := Load(testPaths(t))
	if err != nil || cfg != Default() {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	if _, err := cfg.Set("push.enabled", "off"); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Set("commit.sound", "/x/tag.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got != cfg {
		t.Fatalf("got %+v, %v; want %+v", got, err, cfg)
	}
}

func TestLoadRejectsUnknownKeysAndVersions(t *testing.T) {
	for name, body := range map[string]string{
		"unknown key": "version = 1\nenabeld = false\n",
		"bad version": "version = 2\n",
		"bad toml":    "enabled = \n",
		"commit.when": "version = 1\n[commit]\nwhen = \"attempt\"\n",
		"bad when":    "version = 1\n[push]\nwhen = \"sometimes\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := testPaths(t)
			if err := os.WriteFile(p.ConfigFile(), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(p); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestGetSet(t *testing.T) {
	cfg := Default()
	if changed, err := cfg.Set("enabled", "true"); err != nil || changed {
		t.Fatalf("setting the current value: changed=%v err=%v", changed, err)
	}
	if changed, err := cfg.Set("commit.enabled", "OFF"); err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if v, _ := cfg.Get("commit.enabled"); v != false {
		t.Fatalf("commit.enabled = %v", v)
	}
	if _, err := cfg.Set("enabled", "yes please"); err == nil {
		t.Fatal("accepted a bad boolean")
	}
	for _, k := range []string{"nope", "commit", "commit.volume", "commit.when", "merge.enabled"} {
		if _, err := cfg.Get(k); !errors.Is(err, ErrUnknownKey) {
			t.Errorf("Get(%q): %v", k, err)
		}
	}
	if got := len(cfg.Flatten()); got != len(Keys) {
		t.Fatalf("Flatten has %d keys", got)
	}
}

func TestPushWhen(t *testing.T) {
	cfg := Default()
	if v, _ := cfg.Get("push.when"); v != PushSuccess {
		t.Fatalf("default push.when = %v", v)
	}
	if changed, err := cfg.Set("push.when", "Attempt"); err != nil || !changed || cfg.Push.When != PushAttempt {
		t.Fatalf("changed=%v err=%v when=%q", changed, err, cfg.Push.When)
	}
	if _, err := cfg.Set("push.when", "sometimes"); err == nil {
		t.Fatal("accepted a bad push.when")
	}
	if _, err := cfg.Set("commit.when", "attempt"); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("commit.when: %v", err)
	}

	// A config written before push.when existed loads with the default.
	p := testPaths(t)
	if err := os.WriteFile(p.ConfigFile(), []byte("version = 1\n[push]\nenabled = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got.Push.When != PushSuccess {
		t.Fatalf("got %+v, %v", got.Push, err)
	}
}

func TestCooldown(t *testing.T) {
	cfg := Default()
	if v, _ := cfg.Get("cooldown"); v != 3 {
		t.Fatalf("default cooldown = %v", v)
	}
	if changed, err := cfg.Set("cooldown", "0"); err != nil || !changed || cfg.Cooldown != 0 {
		t.Fatalf("changed=%v err=%v cooldown=%d", changed, err, cfg.Cooldown)
	}
	for _, bad := range []string{"-1", "1.5", "3s", "", "3601"} {
		if _, err := cfg.Set("cooldown", bad); err == nil {
			t.Errorf("accepted cooldown %q", bad)
		}
	}
	p := testPaths(t)
	if err := os.WriteFile(p.ConfigFile(), []byte("version = 1\ncooldown = -5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("loaded a negative cooldown")
	}
}

func TestSoundPathRelative(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	cfg.Push.Sound = "push-x.wav"
	if got := cfg.SoundPath(p, "push"); got != filepath.Join(p.SoundsDir(), "push-x.wav") {
		t.Fatalf("got %s", got)
	}
	if got := cfg.SoundPath(p, "commit"); got != "" {
		t.Fatalf("unset sound resolved to %q", got)
	}
}

func TestImportSound(t *testing.T) {
	p := testPaths(t)
	src := filepath.Join(t.TempDir(), "My Tag.WAV")
	if err := os.WriteFile(src, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest, wrote, err := ImportSound(p, "commit", src)
	if err != nil || !wrote || dest != filepath.Join(p.SoundsDir(), "commit-My Tag.WAV") {
		t.Fatalf("dest=%s wrote=%v err=%v", dest, wrote, err)
	}
	if _, wrote, _ := ImportSound(p, "commit", src); wrote {
		t.Fatal("re-import of identical content wrote again")
	}
	// Importing the copy itself keeps its name.
	if again, _, _ := ImportSound(p, "commit", dest); again != dest {
		t.Fatalf("re-import of copy went to %s", again)
	}

	if _, _, err := ImportSound(p, "commit", src+".missing"); !errors.Is(err, ErrSoundNotFound) {
		t.Fatalf("missing file: %v", err)
	}
	txt := filepath.Join(t.TempDir(), "notes.txt")
	_ = os.WriteFile(txt, nil, 0o644)
	if _, _, err := ImportSound(p, "commit", txt); !errors.Is(err, ErrUnsupportedSound) {
		t.Fatalf("txt file: %v", err)
	}

	// Only jingle's own copies are ever removed.
	if err := RemoveImportedSound(p, "commit", src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("removed the user's original")
	}
	if err := RemoveImportedSound(p, "commit", dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("copy not removed: %v", err)
	}
}
