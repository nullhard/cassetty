// Package watch reports changes to the entries of one directory at a time:
// files or folders being created, removed or renamed in it.
//
// Unlike general purpose file watchers it holds a single handle for the
// watched directory and none for the files inside it, so watching a folder
// with thousands of tracks costs one file descriptor, and on macOS the
// handle is event-only, which keeps external drives ejectable.
package watch

import "errors"

// ErrUnsupported is returned by New on platforms without a watcher backend.
var ErrUnsupported = errors.New("watching folders is not supported on this platform")
