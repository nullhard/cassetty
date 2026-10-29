package nowplaying

import (
	"testing"
	"time"
)

func TestTracker(t *testing.T) {
	t0 := time.Now()
	song := Info{Title: "Song", Artist: "Band", Length: 3 * time.Minute, State: Playing}
	at := func(pos time.Duration, st State) Info {
		i := song
		i.Position, i.State = pos, st
		return i
	}

	var tr tracker
	steps := []struct {
		name string
		info Info
		now  time.Duration
		want bool
	}{
		{"nothing loaded at start", Info{}, 0, false},
		{"track loaded", at(0, Playing), 0, true},
		{"plays on", at(10*time.Second, Playing), 10 * time.Second, false},
		{"slightly late", at(19500*time.Millisecond, Playing), 20 * time.Second, false},
		{"seek forward", at(50*time.Second, Playing), 21 * time.Second, true},
		{"paused", at(52*time.Second, Paused), 23 * time.Second, true},
		{"stays paused", at(52*time.Second, Paused), time.Minute, false},
		{"seek while paused", at(90*time.Second, Paused), time.Minute, true},
		{"resumed", at(90*time.Second, Playing), 2 * time.Minute, true},
		{"next track", Info{Title: "Other", State: Playing}, 2 * time.Minute, true},
		{"ejected", Info{}, 3 * time.Minute, true},
		{"still empty", Info{}, 4 * time.Minute, false},
	}
	for _, s := range steps {
		if got := tr.update(s.info, t0.Add(s.now)); got != s.want {
			t.Errorf("%s: publish = %v, want %v", s.name, got, s.want)
		}
	}
}
