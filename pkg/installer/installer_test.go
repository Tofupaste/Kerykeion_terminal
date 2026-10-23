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

func TestUninstallPreservesDataUnlessPurged(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "local"))
	t.Setenv("APPDATA", filepath.Join(root, "config"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	configRoot, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	configPath, cachePath := filepath.Join(configRoot, "kerykeion"), filepath.Join(cacheRoot, "kerykeion")
	for _, path := range []string{configPath, cachePath} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := RunUninstall(false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{configPath, cachePath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("non-purge uninstall removed data: %v", err)
		}
	}
	if err := RunUninstall(true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{configPath, cachePath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("purged directory still exists: %s", path)
		}
	}
}
