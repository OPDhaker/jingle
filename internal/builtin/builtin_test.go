package builtin

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
)

func TestWAVIsValidPCM(t *testing.T) {
	for _, name := range Names() {
		data, ok := WAV(name)
		if !ok {
			t.Fatalf("%s: not found", name)
		}
		if string(data[0:4]) != "RIFF" || string(data[8:16]) != "WAVEfmt " || string(data[36:40]) != "data" {
			t.Fatalf("%s: bad header %q", name, data[:44])
		}
		le := binary.LittleEndian
		if got := le.Uint32(data[4:8]); int(got) != len(data)-8 {
			t.Errorf("%s: RIFF size %d, file %d", name, got, len(data))
		}
		dataLen := int(le.Uint32(data[40:44]))
		if dataLen != len(data)-44 {
			t.Errorf("%s: data size %d, have %d", name, dataLen, len(data)-44)
		}
		if le.Uint16(data[20:22]) != 1 || le.Uint16(data[22:24]) != 1 || le.Uint32(data[24:28]) != rate || le.Uint16(data[34:36]) != 16 {
			t.Errorf("%s: not mono 16-bit PCM at %d Hz", name, rate)
		}
		secs := float64(dataLen/2) / rate
		if secs < 0.5 || secs > 1.5 {
			t.Errorf("%s: %.2fs long, want 0.5 to 1.5", name, secs)
		}
		// Peak near -3 dBFS, never clipped; silent at both ends (no click).
		var peak int
		for i := 44; i < len(data); i += 2 {
			s := int(int16(le.Uint16(data[i:])))
			peak = max(peak, s, -s)
		}
		if want := 0.7 * math.MaxInt16; math.Abs(float64(peak)-want) > 2 {
			t.Errorf("%s: peak %d, want about %.0f", name, peak, want)
		}
		if first, last := int16(le.Uint16(data[44:])), int16(le.Uint16(data[len(data)-2:])); first != 0 || last != 0 {
			t.Errorf("%s: first/last sample %d/%d, want 0", name, first, last)
		}
		again, _ := WAV(name)
		if !bytes.Equal(data, again) {
			t.Errorf("%s: two renders differ", name)
		}
		if Doc(name) == "" {
			t.Errorf("%s: no doc", name)
		}
	}
	if _, ok := WAV("nope"); ok {
		t.Fatal("WAV(nope) ok")
	}
}

func TestParse(t *testing.T) {
	if n, ok := Parse("builtin:chime"); !ok || n != "chime" {
		t.Fatalf("Parse = %q %v", n, ok)
	}
	if _, ok := Parse("/x/builtin:chime.wav"); ok {
		t.Fatal("path parsed as built-in")
	}
}

func TestEnsure(t *testing.T) {
	dir := t.TempDir()
	path, err := Ensure(dir, "chime")
	if err != nil || path != File(dir, "chime") {
		t.Fatalf("Ensure = %q, %v", path, err)
	}
	want, _ := WAV("chime")
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, want) {
		t.Fatal("written file differs from WAV()")
	}
	fi, _ := os.Stat(path)
	if _, err := Ensure(dir, "chime"); err != nil {
		t.Fatal(err)
	}
	if fi2, _ := os.Stat(path); !fi2.ModTime().Equal(fi.ModTime()) {
		t.Error("Ensure rewrote an up-to-date file")
	}
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(dir, "chime"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, want) {
		t.Fatal("Ensure did not repair a changed file")
	}
	if _, err := Ensure(dir, "nope"); err == nil {
		t.Fatal("Ensure(nope) succeeded")
	}
}
