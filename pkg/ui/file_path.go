package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func commandParts(raw string) []string {
	parts := strings.Fields(raw)
	if len(parts) > 1 && strings.EqualFold(parts[0], "/send") {
		return []string{parts[0], strings.TrimSpace(strings.TrimSpace(raw)[len(parts[0]):])}
	}
	return parts
}

func resolveAttachmentPath(raw string) (string, error) {
	path := strings.TrimSpace(raw)
	if path == "" {
		return "", fmt.Errorf("file path is empty")
	}
	if path[0] == '\'' || path[0] == '"' {
		if len(path) < 2 || path[len(path)-1] != path[0] {
			return "", fmt.Errorf("file path has an unclosed quote")
		}
		path = path[1 : len(path)-1]
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return filepath.Abs(path)
}

func attachmentInfo(path string, limit int64) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("select a regular file: %s", path)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds the maximum size of %s", formatBytes(limit))
	}
	return info, nil
}
