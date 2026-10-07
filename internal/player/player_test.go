package player

import (
	"errors"
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
