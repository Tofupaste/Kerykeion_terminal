package installer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// SystemInfo stores detected hardware and operating system details.
type SystemInfo struct {
	OS             string
	Arch           string
	Distro         string
	PackageManager string
	GoInstalled    bool
	GoVersion      string
	UserHome       string
	Shell          string
	ShellConfigFile string
}

// InstallOptions defines settings for the installation process.
type InstallOptions struct {
	TargetDir string
	Force     bool
}

// DetectSystem gathers operating system, distro, and toolchain information.
func DetectSystem() *SystemInfo {
	info := &SystemInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	home, err := os.UserHomeDir()
	if err == nil {
		info.UserHome = home
	}

	info.Shell = os.Getenv("SHELL")
	info.ShellConfigFile = detectShellConfig(info.UserHome, info.Shell)

	// Check Go compiler
	goPath, err := exec.LookPath("go")
	if err == nil && goPath != "" {
		info.GoInstalled = true
		out, err := exec.Command("go", "version").Output()
		if err == nil {
			info.GoVersion = strings.TrimSpace(string(out))
		}
	}

	// Detect Linux Distro & Package Manager
	if runtime.GOOS == "linux" {
		detectLinuxDistro(info)
	} else if runtime.GOOS == "darwin" {
		info.Distro = "macOS"
		if _, err := exec.LookPath("brew"); err == nil {
			info.PackageManager = "brew"
		}
	} else if runtime.GOOS == "windows" {
		info.Distro = "Windows"
		if _, err := exec.LookPath("winget"); err == nil {
			info.PackageManager = "winget"
		} else if _, err := exec.LookPath("choco"); err == nil {
			info.PackageManager = "choco"
		}
	}

	return info
}

func detectLinuxDistro(info *SystemInfo) {
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				info.Distro = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
			if strings.HasPrefix(line, "NAME=") && info.Distro == "" {
				info.Distro = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
			}
		}
	}
	if info.Distro == "" {
		info.Distro = "Linux"
	}

	// Check package managers
	managers := []string{"pacman", "yay", "paru", "dnf", "apt", "zypper", "apk"}
	for _, m := range managers {
		if _, err := exec.LookPath(m); err == nil {
			info.PackageManager = m
			break
		}
	}
}

func detectShellConfig(home, shellPath string) string {
	shellName := filepath.Base(shellPath)
	switch shellName {
	case "fish":
		fishConf := filepath.Join(home, ".config", "fish", "config.fish")
		return fishConf
	case "zsh":
		return filepath.Join(home, ".zshrc")
	default:
		return filepath.Join(home, ".bashrc")
	}
}

// DefaultInstallDir returns the appropriate standard installation path.
func DefaultInstallDir(info *SystemInfo) string {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" {
			return filepath.Join(localAppData, "Programs", "kerxt")
		}
		return filepath.Join(info.UserHome, "bin")
	}
	return filepath.Join(info.UserHome, ".local", "bin")
}

// RunInstall handles full automated compilation, binary placement, and PATH validation.
func RunInstall(opts InstallOptions) error {
	fmt.Println("==================================================")
	fmt.Println("       Kerykeion Terminal - Automated Installer   ")
	fmt.Println("==================================================")

	info := DetectSystem()
	fmt.Printf("[i] Perangkat: %s (%s)\n", info.Distro, info.Arch)
	if info.GoInstalled {
		fmt.Printf("[i] Go Compiler: %s\n", info.GoVersion)
	}

	installDir := opts.TargetDir
	if installDir == "" {
		installDir = DefaultInstallDir(info)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("gagal membuat direktori tujuan %s: %w", installDir, err)
	}

	targetBinary := filepath.Join(installDir, "kerxt")
	if runtime.GOOS == "windows" {
		targetBinary += ".exe"
	}

	// Determine if running within repository source
	hasSource := false
	if _, err := os.Stat("cmd/kerxt/main.go"); err == nil {
		hasSource = true
	}

	if hasSource {
		if !info.GoInstalled {
			fmt.Println("[!] Kode sumber terdeteksi, namun Go compiler belum terpasang.")
			printGoInstallRecommendation(info)
			return fmt.Errorf("go compiler tidak ditemukan di sistem")
		}

		fmt.Println("[>] Mengompilasi biner produksi (flags: -s -w)...")
		tempOut := filepath.Join(installDir, "kerxt.tmp")
		if runtime.GOOS == "windows" {
			tempOut += ".exe"
		}

		buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", tempOut, "./cmd/kerxt")
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			_ = os.Remove(tempOut)
			return fmt.Errorf("proses kompilasi gagal: %w", err)
		}

		if err := os.Rename(tempOut, targetBinary); err != nil {
			// On Windows or across partitions, fallback to copy
			if errCopy := copyFile(tempOut, targetBinary); errCopy != nil {
				_ = os.Remove(tempOut)
				return fmt.Errorf("gagal memasang biner: %w", errCopy)
			}
			_ = os.Remove(tempOut)
		}
	} else {
		// Self-install: copy current running executable to target destination
		currentExe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("gagal mendeteksi lokasi biner saat ini: %w", err)
		}

		// Avoid copying onto itself
		if filepath.Clean(currentExe) != filepath.Clean(targetBinary) {
			fmt.Println("[>] Memasang biner ke direktori tujuan...")
			if err := copyFile(currentExe, targetBinary); err != nil {
				return fmt.Errorf("gagal menyalin biner ke %s: %w", targetBinary, err)
			}
		}
	}

	_ = os.Chmod(targetBinary, 0755)
	fmt.Printf("[✓] Biner berhasil dipasang di: %s\n", targetBinary)

	// Verify and configure PATH
	ensurePathConfigured(info, installDir)

	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("         Instalasi Berhasil Diselesaikan!         ")
	fmt.Println("==================================================")
	fmt.Println("Ketik perintah berikut di terminal untuk memulai:")
	fmt.Println("  kerxt")
	fmt.Println()
	fmt.Println("Untuk diagnosa jaringan dan firewall:")
	fmt.Println("  kerxt doctor")
	fmt.Println("==================================================")

	return nil
}

// RunUninstall cleanly removes the binary and optionally purges data and cache.
func RunUninstall(purgeData bool) error {
	fmt.Println("==================================================")
	fmt.Println("      Kerykeion Terminal - Uninstaller            ")
	fmt.Println("==================================================")

	info := DetectSystem()
	defaultDir := DefaultInstallDir(info)
	binName := "kerxt"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	targetBinary := filepath.Join(defaultDir, binName)

	removedAny := false
	if _, err := os.Stat(targetBinary); err == nil {
		if err := os.Remove(targetBinary); err != nil {
			fmt.Printf("[!] Gagal menghapus biner di %s: %v\n", targetBinary, err)
		} else {
			fmt.Printf("[✓] Biner berhasil dihapus: %s\n", targetBinary)
			removedAny = true
		}
	}

	// Check if installed in /usr/local/bin
	if runtime.GOOS != "windows" {
		sysBin := filepath.Join("/usr/local/bin", binName)
		if _, err := os.Stat(sysBin); err == nil {
			fmt.Printf("[i] Ditemukan juga biner sistem di %s. Jalankan 'sudo rm %s' jika ingin menghapusnya.\n", sysBin, sysBin)
		}
	}

	if purgeData {
		configDir, _ := os.UserConfigDir()
		cacheDir, _ := os.UserCacheDir()

		keryConfig := filepath.Join(configDir, "kerykeion")
		keryCache := filepath.Join(cacheDir, "kerykeion")

		if _, err := os.Stat(keryConfig); err == nil {
			_ = os.RemoveAll(keryConfig)
			fmt.Printf("[✓] Direktori konfigurasi dan riwayat dihapus: %s\n", keryConfig)
		}
		if _, err := os.Stat(keryCache); err == nil {
			_ = os.RemoveAll(keryCache)
			fmt.Printf("[✓] Direktori cache dan unduhan dihapus: %s\n", keryCache)
		}

		// Also check leftover legacy directories
		legacyConfig := filepath.Join(configDir, "kantorchat")
		legacyCache := filepath.Join(cacheDir, "kantorchat")
		if _, err := os.Stat(legacyConfig); err == nil {
			_ = os.RemoveAll(legacyConfig)
			fmt.Printf("[✓] Direktori konfigurasi lama (kantorchat) dibersihkan: %s\n", legacyConfig)
		}
		if _, err := os.Stat(legacyCache); err == nil {
			_ = os.RemoveAll(legacyCache)
			fmt.Printf("[✓] Direktori cache lama (kantorchat) dibersihkan: %s\n", legacyCache)
		}
	} else {
		fmt.Println("[i] Data riwayat dan konfigurasi dipertahankan. Gunakan 'kerxt uninstall --purge' bila ingin menghapus seluruh data.")
	}

	if removedAny {
		fmt.Println("[✓] Proses penghapusan selesai.")
	} else {
		fmt.Println("[i] Tidak ditemukan biner aktif di lokasi default.")
	}
	return nil
}

func ensurePathConfigured(info *SystemInfo, installDir string) {
	pathEnv := os.Getenv("PATH")
	paths := filepath.SplitList(pathEnv)
	cleanInstallDir := filepath.Clean(installDir)

	for _, p := range paths {
		if filepath.Clean(p) == cleanInstallDir {
			fmt.Printf("[✓] Status PATH: %s sudah terdaftar di PATH.\n", installDir)
			return
		}
	}

	fmt.Printf("[!] Direktori %s belum terdaftar di variabel PATH.\n", installDir)
	if info.ShellConfigFile == "" {
		return
	}

	// Try auto-adding to shell configuration
	shellBase := filepath.Base(info.Shell)
	var appendLine string
	if shellBase == "fish" {
		appendLine = fmt.Sprintf("\n# Kerykeion Terminal PATH\nfish_add_path %s\n", installDir)
	} else {
		appendLine = fmt.Sprintf("\n# Kerykeion Terminal PATH\nexport PATH=\"%s:$PATH\"\n", installDir)
	}

	// Read existing config to avoid duplicate additions
	existing, _ := os.ReadFile(info.ShellConfigFile)
	if strings.Contains(string(existing), installDir) {
		fmt.Printf("[i] Konfigurasi PATH sudah ada di %s. Muat ulang terminal dengan 'source %s'.\n", info.ShellConfigFile, info.ShellConfigFile)
		return
	}

	f, err := os.OpenFile(info.ShellConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		_, _ = f.WriteString(appendLine)
		fmt.Printf("[✓] Berhasil menambahkan PATH ke file konfigurasi shell: %s\n", info.ShellConfigFile)
		fmt.Printf("[i] Jalankan 'source %s' atau buka terminal baru agar aktif.\n", info.ShellConfigFile)
	} else {
		fmt.Printf("[i] Tambahkan baris ini ke file konfigurasi shell (%s):\n", info.ShellConfigFile)
		fmt.Printf("    export PATH=\"%s:$PATH\"\n", installDir)
	}
}

func printGoInstallRecommendation(info *SystemInfo) {
	fmt.Println()
	fmt.Println("Untuk memasang Go compiler di sistem kamu:")
	switch info.PackageManager {
	case "pacman", "yay", "paru":
		fmt.Println("  sudo pacman -S go git")
	case "dnf":
		fmt.Println("  sudo dnf install -y golang git")
	case "apt":
		fmt.Println("  sudo apt update && sudo apt install -y golang git")
	case "brew":
		fmt.Println("  brew install go git")
	default:
		fmt.Println("  Kunjungi https://go.dev/dl/ untuk mengunduh Go.")
	}
	fmt.Println()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Write to temporary file first then replace
	tmpDst := dst + ".install.tmp"
	out, err := os.OpenFile(tmpDst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	buf := bufio.NewReader(in)
	if _, err := io.Copy(out, buf); err != nil {
		out.Close()
		_ = os.Remove(tmpDst)
		return err
	}
	out.Close()

	return os.Rename(tmpDst, dst)
}
