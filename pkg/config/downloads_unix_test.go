//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDownloadsFollowsXDGSettings(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS uses the standard home Downloads directory")
	}
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	if err := os.WriteFile(filepath.Join(xdg, "user-dirs.dirs"), []byte("# user directories\nXDG_DOWNLOAD_DIR=\"$HOME/Unduhan Saya\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := DefaultConfig()
	if err != nil || cfg.DownloadsDir != filepath.Join(home, "Unduhan Saya") {
		t.Fatalf("XDG Downloads: %+v %v", cfg, err)
	}
	if cfg.PartialDir == filepath.Join(cfg.DownloadsDir, ".partial") {
		t.Fatal("partial files should stay in the application cache")
	}
}

func TestDownloadsFallbackAndLegacyMigration(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("XDG-specific migration fixture")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DOWNLOAD_DIR", "")
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DownloadsDir = filepath.Join(cfg.DataDir, "downloads")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	migrated, err := LoadConfig("")
	if err != nil || migrated.DownloadsDir != filepath.Join(home, "Downloads") {
		t.Fatalf("legacy download setting not migrated: %+v %v", migrated, err)
	}
	cfg.DownloadsDir = filepath.Join(home, "custom-downloads")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig("")
	if err != nil || loaded.DownloadsDir != cfg.DownloadsDir {
		t.Fatal("explicit download location was lost")
	}
}
