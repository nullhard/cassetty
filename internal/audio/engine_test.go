package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// writeWAV writes a 16 bit stereo sine wave.
func writeWAV(t *testing.T, path string, rate int, d time.Duration) {
	t.Helper()
	n := int(d.Seconds() * float64(rate))
	data := make([]byte, n*4)
	for i := 0; i < n; i++ {
		v := int16(0.5 * 32767 * math.Sin(2*math.Pi*440*float64(i)/float64(rate)))
		binary.LittleEndian.PutUint16(data[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(data[i*4+2:], uint16(v))
	}
	h := make([]byte, 44)
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(data)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*4))
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(data)))
	if err := os.WriteFile(path, append(h, data...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnginePlaysSeeksAndEnds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tone.wav")
	writeWAV(t, path, 48000, 600*time.Millisecond) // resampled to 44.1k

	src, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if src.Codec != "WAV" || src.SampleRate != 48000 || src.Channels != 2 {
		t.Fatalf("unexpected source info %+v", src)
	}
	if d := src.Length - 600*time.Millisecond; d < -time.Millisecond || d > time.Millisecond {
		t.Fatalf("length %v", src.Length)
	}

	e, _ := NewEngine(true)
	defer e.Close()
	gen := e.Start(src, true)
	time.Sleep(150 * time.Millisecond)
	if p := e.Position(); p <= 0 {
		t.Fatalf("position did not advance: %v", p)
	}

	frames := make([][2]float64, 1024)
	e.Snapshot(frames)
	var energy float64
	for _, f := range frames {
		energy += f[0] * f[0]
	}
	if energy == 0 {
		t.Fatal("tap captured no audio")
	}

	if err := e.Seek(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-e.Events():
		if ev.Gen != gen || ev.Err != nil {
			t.Fatalf("unexpected event %+v (gen %d)", ev, gen)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no end-of-track event")
	}

	// Pausing stops the play head.
	if err := e.Seek(0); err != nil {
		t.Fatal(err)
	}
	e.SetPlaying(false)
	p0 := e.Position()
	time.Sleep(100 * time.Millisecond)
	if p1 := e.Position(); p1 != p0 {
		t.Fatalf("position moved while paused: %v -> %v", p0, p1)
	}
}

// fakeOutput records whether the idle watcher has suspended it.
type fakeOutput struct{ suspended atomic.Bool }

func (o *fakeOutput) Suspend() error { o.suspended.Store(true); return nil }
func (o *fakeOutput) Resume() error  { o.suspended.Store(false); return nil }

func TestEngineSuspendsOutputWhileIdle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.wav")
	writeWAV(t, path, 44100, 300*time.Millisecond)
	src, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	// The null output pulls the deck in real time, as the device would.
	e, _ := NewEngine(true)
	defer e.Close()
	out := &fakeOutput{}
	done := make(chan struct{})
	defer close(done)
	go e.deck.suspendWhenIdle(out, 20*time.Millisecond, done)

	expect := func(suspended bool, when string) {
		t.Helper()
		deadline := time.Now().Add(time.Second)
		for out.suspended.Load() != suspended {
			if time.Now().After(deadline) {
				t.Fatalf("output suspended = %v %s", !suspended, when)
			}
			time.Sleep(time.Millisecond)
		}
	}
	expect(true, "with nothing loaded")
	e.Start(src, true)
	expect(false, "after starting a track")
	e.SetPlaying(false)
	expect(true, "while paused")
	e.SetPlaying(true)
	expect(false, "after unpausing")
	select {
	case <-e.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("no end-of-track event")
	}
	expect(true, "after the track ended")
	if err := e.Seek(0); err != nil {
		t.Fatal(err)
	}
	expect(false, "after seeking back into the track")
	e.Eject()
	expect(true, "after ejecting")
}

func TestOpenRejectsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.mp3")
	os.WriteFile(path, []byte("definitely not an mp3"), 0o644)
	if _, err := Open(path); err == nil {
		t.Fatal("expected an error for a broken file")
	}
}

// TestDevice exercises the real sound device at zero volume when
// CASSETTY_TEST_DEVICE is set.
func TestDevice(t *testing.T) {
	if os.Getenv("CASSETTY_TEST_DEVICE") == "" {
		t.Skip("CASSETTY_TEST_DEVICE not set")
	}
	path := filepath.Join(t.TempDir(), "tone.wav")
	writeWAV(t, path, 44100, time.Second)
	e, err := NewEngine(false)
	if err != nil {
		t.Fatalf("device init: %v", err)
	}
	defer e.Close()
	if e.Silent() {
		t.Fatal("engine fell back to silent mode")
	}
	e.SetVolume(0)
	src, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	e.Start(src, true)
	time.Sleep(400 * time.Millisecond)
	if p := e.Position(); p < 200*time.Millisecond {
		t.Fatalf("device is not pulling samples, position %v", p)
	}
}
