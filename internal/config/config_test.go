package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultMusicDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := DefaultMusicDir(); got != "." {
		t.Fatalf("without ~/Music: got %q, want %q", got, ".")
	}
	music := filepath.Join(home, "Music")
	if err := os.Mkdir(music, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := DefaultMusicDir(); got != music {
		t.Fatalf("with ~/Music: got %q, want %q", got, music)
	}
}

func TestLoadLeavesMusicDirUnset(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MusicDir != "" {
		t.Fatalf("MusicDir = %q, want empty", cfg.MusicDir)
	}
}
