// Package viz turns raw audio frames into the levels drawn by the spectrum
// analyzer and the VU meters.
package viz

import (
	"math"
	"math/cmplx"
)

// fft is a precomputed iterative radix-2 Cooley-Tukey transform.
type fft struct {
	n       int
	rev     []int
	twiddle []complex128
}

func newFFT(n int) *fft {
	bits := 0
	for 1<<bits < n {
		bits++
	}
	f := &fft{n: n, rev: make([]int, n), twiddle: make([]complex128, n/2)}
	for i := range n {
		r := 0
		for b := range bits {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		f.rev[i] = r
	}
	for i := range n / 2 {
		f.twiddle[i] = cmplx.Exp(complex(0, -2*math.Pi*float64(i)/float64(n)))
	}
	return f
}

func (f *fft) transform(x []complex128) {
	for i, r := range f.rev {
		if i < r {
			x[i], x[r] = x[r], x[i]
		}
	}
	for size := 2; size <= f.n; size <<= 1 {
		half, step := size/2, f.n/size
		for start := 0; start < f.n; start += size {
			for k := range half {
				t := f.twiddle[k*step] * x[start+k+half]
				x[start+k+half] = x[start+k] - t
				x[start+k] += t
			}
		}
	}
}

// Spectrum is a Winamp style spectrum analyzer with falling bars and
// floating peak caps. Levels and Peaks are normalised to [0, 1].
type Spectrum struct {
	Levels []float64
	Peaks  []float64

	fft    *fft
	window []float64
	x      []complex128
	mags   []float64
	hold   []float64
}

// FrameSize is the number of frames Spectrum.Update expects.
const FrameSize = 4096

func NewSpectrum() *Spectrum {
	s := &Spectrum{
		fft:    newFFT(FrameSize),
		window: make([]float64, FrameSize),
		x:      make([]complex128, FrameSize),
		mags:   make([]float64, FrameSize/2),
	}
	for i := range s.window {
		s.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(FrameSize-1))
	}
	return s
}

// Update analyses frames (len FrameSize) into bands bars. dt is the time
// since the previous update in seconds. When active is false the bars just
// fall back to zero.
func (s *Spectrum) Update(frames [][2]float64, bands int, rate, dt float64, active bool) {
	if len(s.Levels) != bands {
		s.Levels = make([]float64, bands)
		s.Peaks = make([]float64, bands)
		s.hold = make([]float64, bands)
	}

	if active && len(frames) == FrameSize {
		for i, f := range frames {
			s.x[i] = complex((f[0]+f[1])*0.5*s.window[i], 0)
		}
		s.fft.transform(s.x)
		norm := float64(FrameSize) / 4 // full scale sine through a Hann window
		for i := range s.mags {
			s.mags[i] = cmplx.Abs(s.x[i]) / norm
		}
	}

	binHz := rate / FrameSize
	fmin, fmax := 40.0, math.Min(16000, rate/2*0.95)
	for b := range bands {
		target := 0.0
		if active {
			lo := fmin * math.Pow(fmax/fmin, float64(b)/float64(bands))
			hi := fmin * math.Pow(fmax/fmin, float64(b+1)/float64(bands))
			i0 := int(lo / binHz)
			i1 := max(i0+1, int(math.Ceil(hi/binHz)))
			peak := 0.0
			for i := i0; i < i1 && i < len(s.mags); i++ {
				peak = math.Max(peak, s.mags[i])
			}
			db := 20 * math.Log10(peak+1e-12)
			// Tilt +3 dB/octave around 1 kHz so treble is as lively as bass.
			db += 3 * math.Log2(math.Sqrt(lo*hi)/1000)
			target = clamp01((db + 62) / 58)
		}

		lv := s.Levels[b]
		if target > lv {
			lv += (target - lv) * 0.7
		} else {
			lv = math.Max(target, lv-1.8*dt)
		}
		s.Levels[b] = lv

		if lv >= s.Peaks[b] {
			s.Peaks[b] = lv
			s.hold[b] = 0.45
		} else if s.hold[b] > 0 {
			s.hold[b] -= dt
		} else {
			s.Peaks[b] = math.Max(0, s.Peaks[b]-0.7*dt)
		}
	}
}

// VU models a stereo pair of VU meters with peak hold, levels in [0, 1].
type VU struct {
	L, R         float64
	PeakL, PeakR float64
	holdL, holdR float64
}

// Update integrates the most recent frames (about 50 ms worth is plenty).
func (v *VU) Update(frames [][2]float64, dt float64, active bool) {
	var tl, tr float64
	if active && len(frames) > 0 {
		var sl, sr float64
		for _, f := range frames {
			sl += f[0] * f[0]
			sr += f[1] * f[1]
		}
		n := float64(len(frames))
		tl = dbLevel(math.Sqrt(sl / n))
		tr = dbLevel(math.Sqrt(sr / n))
	}
	v.L = ballistic(v.L, tl, dt)
	v.R = ballistic(v.R, tr, dt)
	v.PeakL, v.holdL = peakHold(v.PeakL, v.holdL, v.L, dt)
	v.PeakR, v.holdR = peakHold(v.PeakR, v.holdR, v.R, dt)
}

// dbLevel maps an RMS amplitude onto [0, 1] over a 42 dB range.
func dbLevel(rms float64) float64 {
	return clamp01((20*math.Log10(rms+1e-12) + 42) / 42)
}

func ballistic(cur, target, dt float64) float64 {
	if target > cur {
		return cur + (target-cur)*math.Min(1, dt*18)
	}
	return cur + (target-cur)*math.Min(1, dt*5)
}

func peakHold(peak, hold, level, dt float64) (float64, float64) {
	if level >= peak {
		return level, 0.8
	}
	if hold > 0 {
		return peak, hold - dt
	}
	return math.Max(level, peak-0.5*dt), 0
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
