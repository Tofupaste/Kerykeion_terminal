package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func installBinary(info *SystemInfo, installDir, target string) error {
	if _, err := os.Stat("cmd/kerxt/main.go"); err != nil {
		return copyExecutable(target)
	}
	if !info.GoInstalled {
		fmt.Println("[!] Source code found, but the Go compiler is not installed.")
		printGoInstallRecommendation(info)
		return fmt.Errorf("Go compiler not found on this system")
	}
	return buildBinary(installDir, target)
}

func buildBinary(installDir, target string) error {
	fmt.Println("[>] Building the production binary (flags: -s -w)...")
	temporary := filepath.Join(installDir, "kerxt.tmp")
	if runtime.GOOS == "windows" {
		temporary += ".exe"
	}
	defer os.Remove(temporary)
	command := exec.Command("go", "build", "-ldflags=-s -w", "-o", temporary, "./cmd/kerxt")
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	if err := os.Rename(temporary, target); err == nil {
		return nil
	}
	if err := copyFile(temporary, target); err != nil {
		return fmt.Errorf("failed to install the binary: %w", err)
	}
	return nil
}

func copyExecutable(target string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate the current executable: %w", err)
	}
	if filepath.Clean(executable) == filepath.Clean(target) {
		return nil
	}
	fmt.Println("[>] Copying the binary to the installation directory...")
	if err := copyFile(executable, target); err != nil {
		return fmt.Errorf("failed to copy the binary to %s: %w", target, err)
	}
	return nil
}
