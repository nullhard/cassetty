package audio

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOpenLibrary opens every file of a real music folder when
// CASSETTY_TEST_MUSIC is set.
func TestOpenLibrary(t *testing.T) {
	root := os.Getenv("CASSETTY_TEST_MUSIC")
	if root == "" {
		t.Skip("CASSETTY_TEST_MUSIC not set")
	}
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		src, err := Open(p)
		if err != nil {
			t.Logf("%s: %v", filepath.Base(p), err)
			return nil
		}
		defer src.Close()
		buf := make([][2]float64, 4096)
		n, ok := src.stream.Stream(buf)
		if err := src.stream.Seek(src.stream.Len() / 2); err != nil {
			t.Errorf("%s: seek: %v", p, err)
		}
		t.Logf("%-60s %-6s %6d Hz %d ch %8v %4d kbps  read=%d ok=%v", filepath.Base(p), src.Codec, src.SampleRate, src.Channels, src.Length.Round(1e7), src.Bitrate, n, ok)
		return nil
	})
}
