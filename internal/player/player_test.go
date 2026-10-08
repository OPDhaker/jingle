package player

import (
	"errors"
	"strings"
	"testing"
)

func fakeLookPath(have ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, h := range have {
			if h == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestDetect(t *testing.T) {
	none := func(string) string { return "" }
	tests := []struct {
		name   string
		getenv func(string) string
		have   []string
		goos   string
		want   string
		wantOK bool
	}{
		{"mac", none, []string{"afplay"}, "darwin", "afplay", true},
		{"linux prefers pw-play", none, []string{"aplay", "pw-play"}, "linux", "pw-play", true},
		{"linux falls back to ffplay", none, []string{"ffplay"}, "linux", "ffplay", true},
		{"linux nothing", none, nil, "linux", "", false},
		{"windows not yet", none, []string{"powershell"}, "windows", "", false},
		{"override", func(string) string { return "fake" }, []string{"fake", "afplay"}, "darwin", EnvOverride, true},
		{"override missing", func(string) string { return "fake" }, []string{"afplay"}, "darwin", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, ok := Detect(tt.getenv, fakeLookPath(tt.have...), tt.goos)
			if ok != tt.wantOK || p.Name != tt.want {
				t.Fatalf("got %+v %v, want %q %v", p, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCommandLineVolume(t *testing.T) {
	none := func(string) string { return "" }
	all := fakeLookPath("afplay", "pw-play", "paplay", "aplay", "ffplay", "fake")
	tests := []struct {
		player string
		goos   string
		volume int
		want   string
	}{
		{"afplay", "darwin", 100, "f.wav"},
		{"afplay", "darwin", 35, "-v 0.35 f.wav"},
		{"afplay", "darwin", 0, "-v 0 f.wav"},
		{"pw-play", "linux", 50, "--volume 0.5 f.wav"},
		{"paplay", "linux", 50, "--volume 32768 f.wav"},
		{"paplay", "linux", 100, "f.wav"},
		{"aplay", "linux", 50, "-q f.wav"},
		{"ffplay", "linux", 50, "-nodisp -autoexit -loglevel quiet -volume 50 f.wav"},
		{EnvOverride, "darwin", 50, "f.wav"},
	}
	for _, tt := range tests {
		getenv, look := none, all
		if tt.player == EnvOverride {
			getenv = func(string) string { return "fake" }
		} else if tt.goos == "linux" {
			look = fakeLookPath(tt.player)
		}
		p, ok := Detect(getenv, look, tt.goos)
		if !ok || p.Name != tt.player {
			t.Fatalf("detected %+v, want %s", p, tt.player)
		}
		if got := strings.Join(p.CommandLine("f.wav", tt.volume), " "); got != tt.want {
			t.Errorf("%s at %d: %q, want %q", tt.player, tt.volume, got, tt.want)
		}
		if wantVol := tt.player != "aplay" && tt.player != EnvOverride; p.Volume != wantVol {
			t.Errorf("%s: Volume = %v", tt.player, p.Volume)
		}
	}
}
