package viz

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestFFTPeak(t *testing.T) {
	const n = 1024
	f := newFFT(n)
	x := make([]complex128, n)
	for i := range x {
		x[i] = complex(math.Sin(2*math.Pi*37*float64(i)/n), 0)
	}
	f.transform(x)
	best := 0
	for i := 1; i < n/2; i++ {
		if cmplx.Abs(x[i]) > cmplx.Abs(x[best]) {
			best = i
		}
	}
	if best != 37 {
		t.Fatalf("peak at bin %d, want 37", best)
	}
	if got := cmplx.Abs(x[37]); math.Abs(got-n/2) > 1e-6 {
		t.Fatalf("magnitude %v, want %v", got, n/2)
	}
}

func TestSpectrumRespondsToTone(t *testing.T) {
	s := NewSpectrum()
	frames := make([][2]float64, FrameSize)
	for i := range frames {
		v := 0.8 * math.Sin(2*math.Pi*1000*float64(i)/44100)
		frames[i] = [2]float64{v, v}
	}
	for range 10 {
		s.Update(frames, 32, 44100, 0.04, true)
	}
	loudest := 0
	for i, l := range s.Levels {
		if l > s.Levels[loudest] {
			loudest = i
		}
	}
	// 1 kHz on a 40 Hz..16 kHz log scale over 32 bands sits around band 17.
	if loudest < 15 || loudest > 19 || s.Levels[loudest] < 0.8 {
		t.Fatalf("loudest band %d level %.2f", loudest, s.Levels[loudest])
	}
	if s.Levels[2] > 0.3 {
		t.Fatalf("bass band lit up for a 1 kHz tone: %.2f", s.Levels[2])
	}
	for range 50 {
		s.Update(nil, 32, 44100, 0.04, false)
	}
	for i := range s.Levels {
		if s.Levels[i] != 0 || s.Peaks[i] != 0 {
			t.Fatalf("bars did not fall back to zero: %v %v", s.Levels[i], s.Peaks[i])
		}
	}
}
