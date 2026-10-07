package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func TestResultJSON(t *testing.T) {
	var out bytes.Buffer
	p := New(true, &out, io.Discard)
	if err := p.Result(map[string]string{"k": "v"}, func(io.Writer) { t.Fatal("plain called in JSON mode") }); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got["k"] != "v" {
		t.Fatalf("got %q, err %v", out.String(), err)
	}
}

func TestResultPlainNoColorOffTTY(t *testing.T) {
	var out bytes.Buffer
	p := New(false, &out, io.Discard)
	if p.Color {
		t.Fatal("color enabled for a non-terminal writer")
	}
	_ = p.Result(nil, func(w io.Writer) { _, _ = io.WriteString(w, "hello\n") })
	if out.String() != "hello\n" {
		t.Fatalf("got %q", out.String())
	}
}

func TestErrorEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantExit int
	}{
		{"typed", Errorf("not_installed", ExitError, "not installed"), "not_installed", ExitError},
		{"untyped is usage", errors.New("unknown flag: --nope"), "usage", ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errw bytes.Buffer
			exit := New(true, io.Discard, &errw).Error(tt.err)
			var env errorEnvelope
			if err := json.Unmarshal(errw.Bytes(), &env); err != nil {
				t.Fatalf("stderr not JSON: %q", errw.String())
			}
			if env.Error.Code != tt.wantCode || exit != tt.wantExit {
				t.Fatalf("code=%q exit=%d, want %q %d", env.Error.Code, exit, tt.wantCode, tt.wantExit)
			}
		})
	}
}
