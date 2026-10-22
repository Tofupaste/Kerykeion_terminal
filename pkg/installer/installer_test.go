package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSystem(t *testing.T) {
	info := DetectSystem()
	if info.OS == "" {
		t.Errorf("expected detected OS not to be empty")
	}
	if info.Arch == "" {
		t.Errorf("expected detected Arch not to be empty")
	}
	if info.Distro == "" {
		t.Errorf("expected detected Distro not to be empty")
	}
}

func TestCopyFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerxt-install-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	src := filepath.Join(tempDir, "src.txt")
	dst := filepath.Join(tempDir, "dst.txt")
	content := []byte("kerykeion terminal installer test")

	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("failed to write src: %v", err)
	}

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	readBack, err := os.ReadFile(dst)
	if err != nil || string(readBack) != string(content) {
		t.Errorf("expected content %s, got %s, err: %v", content, readBack, err)
	}
}

func TestDefaultInstallDir(t *testing.T) {
	info := DetectSystem()
	dir := DefaultInstallDir(info)
	if dir == "" {
		t.Errorf("expected default install dir not to be empty")
	}
}
