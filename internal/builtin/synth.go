package builtin

import (
	"encoding/binary"
	"math"
)

// rate is the sample rate of every built-in sound (mono, 16-bit PCM).
const rate = 44100

// Note frequencies (Hz), equal temperament, A4 = 440.
const (
	g4 = 392.00
	f4 = 349.23
	a4 = 440.00
	c5 = 523.25
	e5 = 659.25
	g5 = 783.99
	a5 = 880.00
	c6 = 1046.50
	e6 = 1318.51
)

// track is a mono buffer of samples in about [-1, 1] before master().
type track []float64

func newTrack(seconds float64) track { return make(track, int(seconds*rate)) }

// add mixes voice v, scaled by gain, into t from start for dur seconds. v
// gets the time since the voice started.
func (t track) add(start, dur, gain float64, v func(x float64) float64) {
	i0, n := int(start*rate), int(dur*rate)
	for i := 0; i < n && i0+i < len(t); i++ {
		t[i0+i] += gain * v(float64(i)/rate)
	}
}

// envelope: linear attack, exponential decay, linear release over the last
// rel seconds of dur.
func envelope(x, dur, attack, decay, rel float64) float64 {
	a := math.Exp(-x * decay)
	if x < attack {
		a *= x / attack
	}
	if x > dur-rel {
		a *= math.Max(0, (dur-x)/rel)
	}
	return a
}

func sine(f, x float64) float64 { return math.Sin(2 * math.Pi * f * x) }

// bell is an FM bell: a harmonic modulator whose depth dies away (bright
// strike, pure tail) plus a quiet inharmonic partial for the metallic color.
func bell(f, dur float64) func(float64) float64 {
	return func(x float64) float64 {
		depth := 1.8 * math.Exp(-x*7)
		tone := math.Sin(2*math.Pi*f*x + depth*sine(f, x))
		tone += 0.2 * sine(f*2.76, x) * math.Exp(-x*9)
		return envelope(x, dur, 0.002, 3.2, 0.05) * tone
	}
}

// brass is a band-limited sawtooth that brightens during the attack, doubled
// with a slightly detuned copy for width.
func brass(f, dur float64) func(float64) float64 {
	return func(x float64) float64 {
		bright := math.Min(1, x/0.04)
		var s float64
		for k := 1.0; k <= 8; k++ {
			amp := math.Exp(-k*(0.55-0.35*bright)) / k
			s += amp * (sine(f*k, x) + sine(f*1.003*k, x))
		}
		return envelope(x, dur, 0.015, 1.2, 0.12) * s * 0.5
	}
}

// pad is a soft organ-like tone.
func pad(f, dur float64) func(float64) float64 {
	return func(x float64) float64 {
		s := sine(f, x) + 0.3*sine(2*f, x) + 0.1*sine(3*f, x)
		return envelope(x, dur, 0.04, 1.4, 0.15) * s
	}
}

// kick808 is a sine whose pitch drops from 130 Hz to 45 Hz, softly
// saturated so it reads on small speakers.
func kick808(dur float64) func(float64) float64 {
	const hi, lo, speed = 130.0, 45.0, 12.0
	return func(x float64) float64 {
		phase := 2 * math.Pi * (lo*x + (hi-lo)*(1-math.Exp(-speed*x))/speed)
		return envelope(x, dur, 0.002, 2.2, 0.1) * math.Tanh(1.6*math.Sin(phase))
	}
}

// snare is high-passed noise over a short 190 Hz body. The noise comes from
// a fixed-seed xorshift generator, so the sound is the same every time.
func snare(dur float64) func(float64) float64 {
	state := uint32(0x9e3779b9)
	var prev float64
	return func(x float64) float64 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		n := float64(state)/math.MaxUint32*2 - 1
		hp := n - prev // one-zero high-pass: crisper, less hiss-low-end
		prev = n
		body := 0.6 * sine(190, x) * math.Exp(-x*25)
		return envelope(x, dur, 0.001, 18, 0.02) * (0.7*hp + body)
	}
}

func chime() []float64 {
	t := newTrack(1.0)
	t.add(0, 1.0, 0.6, bell(a5, 1.0))
	t.add(0.11, 0.89, 0.6, bell(e6, 0.89))
	return t
}

func fanfare() []float64 {
	t := newTrack(1.3)
	for i, f := range []float64{c5, e5, g5} {
		t.add(float64(i)*0.08, 0.14, 0.5, brass(f, 0.14))
	}
	const start, hold = 0.24, 1.06
	t.add(start, hold, 0.55, brass(c6, hold))
	t.add(start, hold, 0.3, brass(g5, hold))
	t.add(start, hold, 0.3, brass(g4, hold))
	return t
}

func harmony() []float64 {
	t := newTrack(1.4)
	t.add(0, 0.38, 0.5, pad(f4, 0.38))
	t.add(0, 0.38, 0.5, pad(a4, 0.38))
	const start, hold = 0.3, 1.1
	for _, f := range []float64{c5, e5, g5} {
		t.add(start, hold, 0.4, pad(f, hold))
	}
	t.add(start, hold, 0.25, bell(c6, hold))
	return t
}

func boom() []float64 {
	t := newTrack(1.4)
	t.add(0, 0.25, 0.6, snare(0.25))
	t.add(0.02, 1.38, 0.9, kick808(1.38))
	return t
}

// master normalizes the peak to about -3 dBFS and fades both ends over 5 ms
// so playback never clicks.
func master(t []float64) []float64 {
	var peak float64
	for _, s := range t {
		peak = math.Max(peak, math.Abs(s))
	}
	if peak == 0 {
		return t
	}
	g := 0.7 / peak
	fade := rate / 200 // 5 ms
	for i := range t {
		t[i] *= g
		if i < fade {
			t[i] *= float64(i) / float64(fade)
		}
		if j := len(t) - 1 - i; j < fade {
			t[i] *= float64(j) / float64(fade)
		}
	}
	return t
}

// encodeWAV writes samples in [-1, 1] as a mono 16-bit PCM WAV file.
func encodeWAV(samples []float64) []byte {
	const channels, bits = 1, 16
	dataLen := len(samples) * bits / 8
	b := make([]byte, 0, 44+dataLen)
	le := binary.LittleEndian
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, uint32(36+dataLen))
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16) // fmt chunk size
	b = le.AppendUint16(b, 1)  // PCM
	b = le.AppendUint16(b, channels)
	b = le.AppendUint32(b, rate)
	b = le.AppendUint32(b, rate*channels*bits/8) // byte rate
	b = le.AppendUint16(b, channels*bits/8)      // block align
	b = le.AppendUint16(b, bits)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(dataLen))
	for _, s := range samples {
		b = le.AppendUint16(b, uint16(int16(math.Round(math.Max(-1, math.Min(1, s))*math.MaxInt16))))
	}
	return b
}
