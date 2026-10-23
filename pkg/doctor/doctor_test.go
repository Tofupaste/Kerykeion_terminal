package doctor

import (
	"net"
	"strings"
	"testing"

	"kerykeion/pkg/config"
)

func TestTCPDiagnosticsWithOccupiedPort(t *testing.T) {
	listener, err := net.Listen("tcp4", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	cfg := &config.Config{TCPPort: port, UDPPort: 42101}

	t.Run("application listener", func(t *testing.T) {
		report := RunDiagnosticsForApp(cfg, nil, port)
		if !strings.Contains(report.TCPPortStatus, "AKTIF (digunakan aplikasi ini)") {
			t.Fatalf("own listener reported incorrectly: %s", report.TCPPortStatus)
		}
		english := FormatReportEnglish(report, cfg.UDPPort)
		if !strings.Contains(english, "ACTIVE (used by this application)") || !strings.Contains(english, "LAN broadcast reachability is not tested") {
			t.Fatalf("English diagnosis lost listener or UDP test context: %s", english)
		}
	})
	t.Run("standalone diagnosis", func(t *testing.T) {
		report := RunDiagnostics(cfg, nil)
		if !strings.Contains(report.TCPPortStatus, "TERPAKAI") || strings.Contains(report.TCPPortStatus, "DIBLOKIR") {
			t.Fatalf("occupied port reported incorrectly: %s", report.TCPPortStatus)
		}
	})
	t.Run("different application port", func(t *testing.T) {
		report := RunDiagnosticsForApp(cfg, nil, port+1)
		if !strings.Contains(report.TCPPortStatus, "TERPAKAI") {
			t.Fatalf("another listener mistaken for own listener: %s", report.TCPPortStatus)
		}
	})
}

func TestTCPDiagnosticsWithAvailablePort(t *testing.T) {
	report := RunDiagnostics(&config.Config{TCPPort: 0}, nil)
	if !strings.Contains(report.TCPPortStatus, "TERSEDIA") {
		t.Fatalf("available port reported incorrectly: %s", report.TCPPortStatus)
	}
}
