package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePaths(t *testing.T) {
	j := filepath.Join
	tests := []struct {
		name string
		env  map[string]string
		goos string
		want Paths
	}{
		{"unix defaults", nil, "darwin", Paths{j("/h", ".config", "jingle"), j("/h", ".local", "share", "jingle")}},
		{"xdg overrides", map[string]string{"XDG_CONFIG_HOME": "/c", "XDG_DATA_HOME": "/d"}, "linux", Paths{j("/c", "jingle"), j("/d", "jingle")}},
		{"windows", map[string]string{"AppData": "/r", "LocalAppData": "/l"}, "windows", Paths{j("/r", "jingle"), j("/l", "jingle")}},
		{"JINGLE_HOME wins", map[string]string{"JINGLE_HOME": "/t", "XDG_CONFIG_HOME": "/c"}, "linux", Paths{"/t", "/t"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePaths(func(k string) string { return tt.env[k] }, tt.goos, "/h")
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolvePathsWindowsMissingEnv(t *testing.T) {
	if _, err := resolvePaths(func(string) string { return "" }, "windows", "/h"); err == nil {
		t.Fatal("expected error when AppData is unset")
	}
}
