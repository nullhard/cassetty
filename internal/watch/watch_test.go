package watch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newWatcher(t *testing.T) *Watcher {
	t.Helper()
	w, err := New()
	if errors.Is(err, ErrUnsupported) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

// expect waits for a notification; drain discards pending ones first.
func expect(t *testing.T, w *Watcher, what string) {
	t.Helper()
	select {
	case <-w.C:
	case <-time.After(2 * time.Second):
		t.Fatalf("no notification after %s", what)
	}
}

func drain(w *Watcher) {
	time.Sleep(50 * time.Millisecond)
	for {
		select {
		case <-w.C:
		default:
			return
		}
	}
}

func quiet(t *testing.T, w *Watcher, what string) {
	t.Helper()
	select {
	case <-w.C:
		t.Fatalf("unexpected notification after %s", what)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchDirectoryEntries(t *testing.T) {
	dir := t.TempDir()
	w := newWatcher(t)
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}

	f := filepath.Join(dir, "new song.mp3")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect(t, w, "creating a file")
	drain(w)

	if err := os.Mkdir(filepath.Join(dir, "New Album"), 0o755); err != nil {
		t.Fatal(err)
	}
	expect(t, w, "creating a folder")
	drain(w)

	if err := os.Rename(f, filepath.Join(dir, "renamed.mp3")); err != nil {
		t.Fatal(err)
	}
	expect(t, w, "renaming a file")
	drain(w)

	if err := os.Remove(filepath.Join(dir, "renamed.mp3")); err != nil {
		t.Fatal(err)
	}
	expect(t, w, "removing a file")
	drain(w)

	// Changes inside a sub folder are not changes of the watched folder.
	if err := os.WriteFile(filepath.Join(dir, "New Album", "x.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	quiet(t, w, "writing into a sub folder")
}

func TestWatchSwitchesDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	w := newWatcher(t)
	if err := w.Watch(a); err != nil {
		t.Fatal(err)
	}
	if err := w.Watch(a); err != nil { // re-watching is a no-op
		t.Fatal(err)
	}
	if err := w.Watch(b); err != nil {
		t.Fatal(err)
	}
	drain(w)

	os.WriteFile(filepath.Join(a, "old.mp3"), []byte("x"), 0o644)
	quiet(t, w, "changing the previously watched folder")

	os.WriteFile(filepath.Join(b, "new.mp3"), []byte("x"), 0o644)
	expect(t, w, "changing the watched folder")
}

func TestWatchedDirectoryRemoved(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "gone")
	os.Mkdir(dir, 0o755)
	w := newWatcher(t)
	if err := w.Watch(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	expect(t, w, "removing the watched folder")

	if err := w.Watch(dir); err == nil {
		t.Fatal("watching a missing folder should fail")
	}
}

func TestCloseEndsChannel(t *testing.T) {
	w := newWatcher(t)
	w.Watch(t.TempDir())
	w.Close()
	select {
	case _, ok := <-w.C:
		if ok {
			// a pending notification may still be buffered; the next read must see the close
			if _, ok := <-w.C; ok {
				t.Fatal("channel not closed")
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not stop the watcher")
	}
}
