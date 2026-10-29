//go:build !darwin && !linux

package watch

// Watcher is unavailable on this platform; callers fall back to polling.
type Watcher struct {
	C <-chan struct{}
}

// New always fails with ErrUnsupported on this platform.
func New() (*Watcher, error) { return nil, ErrUnsupported }

func (w *Watcher) Watch(string) error { return ErrUnsupported }

func (w *Watcher) Close() error { return nil }
