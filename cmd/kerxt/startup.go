package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"kerykeion/pkg/config"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/installer"
)

type startupOptions struct {
	initialRoom string
	repl        bool
	debug       bool
	handled     bool
}

func runStartupCommand(args []string, configDir string, options startupOptions) (startupOptions, error) {
	if len(args) == 0 {
		return options, nil
	}
	switch strings.ToLower(args[0]) {
	case "install":
		options.handled = true
		target := ""
		if len(args) > 1 {
			target = args[1]
		}
		if err := installer.RunInstall(installer.InstallOptions{TargetDir: target}); err != nil {
			return options, fmt.Errorf("Installation failed: %w", err)
		}
	case "uninstall":
		options.handled = true
		if err := installer.RunUninstall(hasPurgeFlag(args[1:])); err != nil {
			return options, fmt.Errorf("Uninstallation failed: %w", err)
		}
	case "doctor":
		options.handled = true
		cfg, err := config.LoadConfig(configDir)
		if err != nil {
			return options, fmt.Errorf("Failed to load configuration: %w", err)
		}
		report := doctor.RunDiagnostics(cfg, nil)
		fmt.Println(doctor.FormatReportEnglish(report, cfg.UDPPort))
	case "join":
		options.parseJoin(args[1:])
	}
	return options, nil
}

func hasPurgeFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--purge" || arg == "-purge" {
			return true
		}
	}
	return false
}

func (o *startupOptions) parseJoin(args []string) {
	for _, arg := range args {
		switch arg {
		case "--repl", "-repl":
			o.repl = true
		case "--debug", "-debug":
			o.debug = true
		default:
			if !strings.HasPrefix(arg, "-") && o.initialRoom == "" {
				o.initialRoom = arg
			}
		}
	}
}

func runFirstRunWizard(cfg *config.Config) {
	// First run wizard if display name is empty
	if cfg.DisplayName == "" {
		fmt.Println("==================================================")
		fmt.Println("     Selamat Datang di Kerykeion Terminal (P2P)   ")
		fmt.Println("==================================================")
		fmt.Println("Aplikasi chat & file sharing langsung antar-laptop")
		fmt.Println("tanpa internet dan tanpa server pusat.")
		fmt.Println()
		fmt.Print("Masukkan nama tampilan kamu: ")
		reader := bufio.NewReader(os.Stdin)
		inputName, _ := reader.ReadString('\n')
		inputName = strings.TrimSpace(inputName)
		if inputName == "" {
			inputName = "anon"
		}
		cfg.DisplayName = inputName
		_ = cfg.Save()
		fmt.Printf("\nNama kamu disetel sebagai: %s\n", inputName)
		fmt.Println("Panduan Cepat:")
		fmt.Println(" 1. Ketik /who untuk melihat siapa saja yang online di WiFi.")
		fmt.Println(" 2. Ketik /join <nama_room> untuk mengobrol bersama.")
		fmt.Println(" 3. Ketik /send <file> untuk mengirim berkas.")
		fmt.Println("--------------------------------------------------")
	}
}
