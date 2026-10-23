package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLanguageDefaultsAndPersistence(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "en" {
		t.Fatalf("default language = %q", cfg.Language)
	}
	cfg.Language = "id"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(dir)
	if err != nil || loaded.Language != "id" {
		t.Fatalf("saved language was not restored: %+v, %v", loaded, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"language":"unknown"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadConfig(dir)
	if err != nil || loaded.Language != "en" {
		t.Fatalf("invalid language should fall back to English: %+v, %v", loaded, err)
	}
}
