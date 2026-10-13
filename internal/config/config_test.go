package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OPDhaker/jingle/internal/builtin"
)

func testPaths(t *testing.T) Paths {
	d := t.TempDir()
	return Paths{ConfigDir: d, DataDir: d}
}

func TestLoadMissingIsDefault(t *testing.T) {
	cfg, err := Load(testPaths(t))
	if err != nil || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	if _, err := cfg.Set("push.enabled", "off"); err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Set("commit.sounds", "/x/tag.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || !reflect.DeepEqual(got, cfg) {
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
		"merge.when":  "version = 1\n[merge]\nwhen = \"attempt\"\n",
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
	for _, k := range []string{"nope", "commit", "commit.volume", "commit.when", "merge.when"} {
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

func TestMergeIsOffByDefault(t *testing.T) {
	cfg := Default()
	if v, _ := cfg.Get("merge.enabled"); v != false {
		t.Fatalf("merge.enabled = %v", v)
	}
	if changed, err := cfg.Set("merge.enabled", "on"); err != nil || !changed || !cfg.Merge.Enabled {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
}

func TestVolume(t *testing.T) {
	cfg := Default()
	if v, _ := cfg.Get("volume"); v != 100 {
		t.Fatalf("default volume = %v", v)
	}
	if changed, err := cfg.Set("volume", "40"); err != nil || !changed || cfg.Volume != 40 {
		t.Fatalf("changed=%v err=%v volume=%d", changed, err, cfg.Volume)
	}
	for _, bad := range []string{"-1", "101", "50%", "0.5", ""} {
		if _, err := cfg.Set("volume", bad); err == nil {
			t.Errorf("accepted volume %q", bad)
		}
	}
	p := testPaths(t)
	if err := os.WriteFile(p.ConfigFile(), []byte("version = 1\nvolume = 200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("loaded volume 200")
	}
	// A config written before volume existed plays at full volume.
	if err := os.WriteFile(p.ConfigFile(), []byte("version = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got.Volume != 100 {
		t.Fatalf("got volume %d, %v", got.Volume, err)
	}
}

func TestSoundPathsRelative(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	cfg.Push.Sounds = []string{"push-x.wav", "/abs/y.wav"}
	want := []string{filepath.Join(p.SoundsDir(), "push-x.wav"), "/abs/y.wav"}
	if got := cfg.SoundPaths(p, "push"); !slices.Equal(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := cfg.SoundPaths(p, "commit"); !slices.Equal(got, []string{filepath.Join(p.SoundsDir(), "builtin", "chime.wav")}) {
		t.Fatalf("default commit sound resolved to %v", got)
	}
	cfg.Commit.Sounds = nil
	if got := cfg.SoundPaths(p, "commit"); len(got) != 0 {
		t.Fatalf("unset sounds resolved to %v", got)
	}
}

func TestClearedSoundsStayCleared(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	cfg.Commit.SetSounds(nil)
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || len(got.Commit.Sounds) != 0 || len(got.Push.Sounds) != 1 {
		t.Fatalf("after clearing commit.sounds: commit %v, push %v, %v", got.Commit.Sounds, got.Push.Sounds, err)
	}
}

func TestDefaultSoundsAreBuiltins(t *testing.T) {
	cfg := Default()
	for _, ev := range Events {
		sounds := cfg.Event(ev).Sounds
		if len(sounds) != 1 {
			t.Fatalf("%s: default sounds %v", ev, sounds)
		}
		if name, ok := builtin.Parse(sounds[0]); !ok || !builtin.Exists(name) {
			t.Fatalf("%s: default %q is not a built-in sound", ev, sounds[0])
		}
	}
}

func TestKeyInfos(t *testing.T) {
	cfg := Default()
	for _, k := range KeyInfos {
		v, err := cfg.Get(k.Key)
		if err != nil {
			t.Fatalf("%s: %v", k.Key, err)
		}
		want := map[string]string{"bool": "bool", "int": "int", "string": "string", "list": "[]string"}[k.Type]
		if got := fmt.Sprintf("%T", v); got != want || k.Doc == "" {
			t.Errorf("%s: type %s, value is %s, doc %q", k.Key, k.Type, got, k.Doc)
		}
		if (k.Min == nil) != (k.Type != "int") || (len(k.Values) > 0) != (k.Key == "push.when") {
			t.Errorf("%s: bounds/values don't fit type %s", k.Key, k.Type)
		}
	}
}

func TestLegacySoundMigrates(t *testing.T) {
	p := testPaths(t)
	body := "version = 1\n[commit]\nenabled = true\nsound = \"/a.wav\"\nsounds = [\"/b.wav\", \"/a.wav\"]\n"
	if err := os.WriteFile(p.ConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil || !slices.Equal(cfg.Commit.Sounds, []string{"/a.wav", "/b.wav"}) || cfg.Commit.Sound != "" {
		t.Fatalf("got %+v, %v", cfg.Commit, err)
	}
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p.ConfigFile())
	if strings.Contains(string(data), "sound =") {
		t.Fatalf("legacy key written back:\n%s", data)
	}
}

func TestLegacySoundReplacesDefault(t *testing.T) {
	p := testPaths(t)
	body := "version = 1\n[commit]\nsound = \"/a.wav\"\n[push]\nsound = \"\"\n"
	if err := os.WriteFile(p.ConfigFile(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil || !slices.Equal(cfg.Commit.Sounds, []string{"/a.wav"}) || len(cfg.Push.Sounds) != 0 {
		t.Fatalf("commit %v, push %v, %v; want [/a.wav] and []", cfg.Commit.Sounds, cfg.Push.Sounds, err)
	}
	if !slices.Equal(cfg.Merge.Sounds, Default().Merge.Sounds) {
		t.Fatalf("merge (no legacy key) %v, want the default", cfg.Merge.Sounds)
	}
}

func TestSoundList(t *testing.T) {
	cfg := Default()
	ev := &cfg.Commit
	if !ev.SetSounds([]string{"/a.wav", "", "/b.wav", "/a.wav"}) || !slices.Equal(ev.Sounds, []string{"/a.wav", "/b.wav"}) {
		t.Fatalf("SetSounds: %v", ev.Sounds)
	}
	if ev.SetSounds([]string{"/a.wav", "/b.wav"}) {
		t.Fatal("same list reported a change")
	}
	if !ev.AddSound("/c.wav") || ev.AddSound("/c.wav") {
		t.Fatal("AddSound not idempotent")
	}
	if !ev.RemoveSound("/b.wav") || ev.RemoveSound("/b.wav") {
		t.Fatal("RemoveSound not idempotent")
	}
	if v, _ := cfg.Get("commit.sounds"); !reflect.DeepEqual(v, []string{"/a.wav", "/c.wav"}) {
		t.Fatalf("Get = %#v", v)
	}
	cfg.Push.Sounds = nil
	if v, _ := cfg.Get("push.sounds"); v == nil || len(v.([]string)) != 0 {
		t.Fatalf("empty list Get = %#v, want []string{}", v)
	}
	if changed, _ := cfg.Set("commit.sounds", ""); !changed || len(ev.Sounds) != 0 {
		t.Fatalf("clearing: %v", ev.Sounds)
	}
}

func TestMatchSound(t *testing.T) {
	p := testPaths(t)
	cfg := Default()
	cfg.Commit.Sounds = []string{filepath.Join(p.SoundsDir(), "commit-tag.wav"), filepath.Join(p.SoundsDir(), "commit-tag-2.wav"), "builtin:boom"}
	for name, want := range map[string]string{
		"builtin:boom":              "builtin:boom",
		"boom":                      "builtin:boom",
		cfg.Commit.Sounds[1]:        cfg.Commit.Sounds[1],
		"commit-tag-2.wav":          cfg.Commit.Sounds[1],
		"tag.wav":                   cfg.Commit.Sounds[0],
		"/Users/me/Music/tag.wav":   cfg.Commit.Sounds[0],
		"/elsewhere/commit-tag.wav": cfg.Commit.Sounds[0],
	} {
		if got, ok := cfg.MatchSound(p, "commit", name); !ok || got != want {
			t.Errorf("MatchSound(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for _, name := range []string{"", "other.wav", "tag-2.wav.bak", "chime", "builtin:chime"} {
		if got, ok := cfg.MatchSound(p, "commit", name); ok {
			t.Errorf("MatchSound(%q) matched %q", name, got)
		}
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

	// A different file with the same name gets its own copy; importing either
	// again finds its copy.
	other := filepath.Join(t.TempDir(), "My Tag.WAV")
	if err := os.WriteFile(other, []byte("RIFF2"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest2, wrote, err := ImportSound(p, "commit", other)
	if err != nil || !wrote || dest2 != filepath.Join(p.SoundsDir(), "commit-My Tag-2.WAV") {
		t.Fatalf("collision: dest=%s wrote=%v err=%v", dest2, wrote, err)
	}
	for src, want := range map[string]string{src: dest, other: dest2} {
		if got, wrote, _ := ImportSound(p, "commit", src); got != want || wrote {
			t.Fatalf("re-import %s: %s wrote=%v, want %s", src, got, wrote, want)
		}
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
