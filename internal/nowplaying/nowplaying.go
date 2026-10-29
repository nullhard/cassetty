// Package nowplaying connects the player to the system's media controls: on
// macOS the media keys, headphone buttons and the Now Playing widget of
// Control Center reach cassetty, and the widget shows the loaded track.
//
// macOS hands media commands to the app that is "now playing", which is the
// one that most recently published a playing track. A Session therefore only
// claims the keys once Update has published a track.
package nowplaying

import (
	"errors"
	"time"
)

// ErrUnsupported is returned by Start on platforms without media controls.
var ErrUnsupported = errors.New("media controls are not supported on this platform")

// Kind says what a Command asks for.
type Kind int

const (
	TogglePlayPause Kind = iota
	Play
	Pause
	Stop
	Next
	Previous
	Seek // jump to Command.Position
)

// Command is a request from the system's media controls.
type Command struct {
	Kind     Kind
	Position time.Duration // for Seek
}

// State is the play state of the loaded track.
type State int

const (
	Stopped State = iota
	Playing
	Paused
)

// Info describes the loaded track. The zero Info means no track is loaded.
type Info struct {
	Title, Artist, Album string
	Length               time.Duration // 0 if unknown
	Position             time.Duration
	State                State
}

// maxDrift is how far the reported position may stray from where the system
// expects it before it is published again. The system advances the position
// of a playing track by itself, so only jumps need publishing.
const maxDrift = time.Second

// tracker filters the updates worth passing on to the system.
type tracker struct {
	last Info
	at   time.Time // when last was published
}

// update reports whether info should be published at time now, and if so
// remembers it as published.
func (t *tracker) update(info Info, now time.Time) bool {
	last := t.last
	want := last.Position
	if last.State == Playing {
		want += now.Sub(t.at)
	}
	drift := info.Position - want
	same := info.Title == last.Title && info.Artist == last.Artist && info.Album == last.Album &&
		info.Length == last.Length && info.State == last.State
	if same && drift < maxDrift && drift > -maxDrift {
		return false
	}
	t.last, t.at = info, now
	return true
}
