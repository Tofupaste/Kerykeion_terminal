//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func downloadsDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	fallback := filepath.Join(home, "Downloads")
	if runtime.GOOS == "darwin" {
		return fallback, nil
	}
	if value := expandDownloadDirectory(os.Getenv("XDG_DOWNLOAD_DIR"), home); value != "" {
		return value, nil
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(configHome, "user-dirs.dirs"))
	if err != nil {
		return fallback, nil
	}
	return parseDownloadsDirectory(string(data), home, fallback), nil
}

func expandDownloadDirectory(value, home string) string {
	value = strings.ReplaceAll(value, "${HOME}", home)
	value = strings.ReplaceAll(value, "$HOME", home)
	if !filepath.IsAbs(value) {
		return ""
	}
	return filepath.Clean(value)
}

func parseDownloadsDirectory(data, home, fallback string) string {
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "XDG_DOWNLOAD_DIR" {
			continue
		}
		decoded, err := strconv.Unquote(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		if resolved := expandDownloadDirectory(decoded, home); resolved != "" {
			return resolved
		}
	}
	return fallback
}
