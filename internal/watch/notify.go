//go:build darwin || linux

package watch

// notify does a non-blocking send, coalescing bursts of events into one
// pending notification.
func notify(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}
