package doctor

import (
	"errors"
	"fmt"
	"net"
	"runtime"
	"strings"
	"syscall"
	"time"

	"kerykeion/pkg/config"
	"kerykeion/pkg/discovery"
)

type DiagnosticResult struct {
	OS               string
	Interfaces       []InterfaceDetail
	TCPPortStatus    string
	TCPPort          int
	TCPState         string
	TCPError         string
	UDPPortStatus    string
	BroadcastWorking bool
	PeersFound       int
	Suggestions      []string
}

type InterfaceDetail struct {
	Name      string
	IP        string
	Netmask   string
	Broadcast string
	IsUp      bool
}

func RunDiagnostics(cfg *config.Config, disc *discovery.Discovery) DiagnosticResult {
	return runDiagnostics(cfg, disc, 0)
}

func RunDiagnosticsForApp(cfg *config.Config, disc *discovery.Discovery, listeningTCPPort int) DiagnosticResult {
	return runDiagnostics(cfg, disc, listeningTCPPort)
}

func runDiagnostics(cfg *config.Config, disc *discovery.Discovery, listeningTCPPort int) DiagnosticResult {
	res := DiagnosticResult{OS: runtime.GOOS, TCPPort: cfg.TCPPort, Interfaces: inspectInterfaces()}
	inspectTCPPort(cfg.TCPPort, listeningTCPPort, &res)
	inspectUDPLoopback(&res)
	if disc != nil {
		res.PeersFound = len(disc.GetPeers())
	}
	appendNetworkSuggestions(cfg, &res)
	return res
}

func inspectInterfaces() []InterfaceDetail {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var details []InterfaceDetail
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			details = append(details, inspectInterface(iface)...)
		}
	}
	return details
}

func inspectInterface(iface net.Interface) []InterfaceDetail {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}
	var details []InterfaceDetail
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil {
			continue
		}
		ip4 := ipnet.IP.To4()
		mask := ipnet.Mask
		broadcast := ""
		if len(mask) == 4 {
			broadcast = net.IPv4(ip4[0]|^mask[0], ip4[1]|^mask[1], ip4[2]|^mask[2], ip4[3]|^mask[3]).String()
		}
		details = append(details, InterfaceDetail{
			Name: iface.Name, IP: ip4.String(), Netmask: net.IP(mask).String(), Broadcast: broadcast,
			IsUp: iface.Flags&net.FlagUp != 0,
		})
	}
	return details
}

func inspectTCPPort(port, listeningPort int, res *DiagnosticResult) {
	if listeningPort > 0 && listeningPort == port {
		res.TCPState = "active"
		res.TCPPortStatus = fmt.Sprintf("Port %d TCP AKTIF (digunakan aplikasi ini)", port)
		return
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf(":%d", port))
	if err == nil {
		res.TCPState = "available"
		res.TCPPortStatus = fmt.Sprintf("Port %d TCP TERSEDIA", port)
		_ = listener.Close()
		return
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		res.TCPState = "occupied"
		res.TCPPortStatus = fmt.Sprintf("Port %d TCP TERPAKAI proses lain atau instance Kerykeion lain", port)
		res.Suggestions = append(res.Suggestions, "Port terpakai bukan berarti diblokir firewall. Tutup instance lain atau pilih port berbeda dengan --port.")
		return
	}
	res.TCPState = "error"
	res.TCPError = err.Error()
	res.TCPPortStatus = fmt.Sprintf("Gagal membuka port %d TCP: %v", port, err)
}

func inspectUDPLoopback(res *DiagnosticResult) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0, IP: net.IPv4zero})
	if err != nil {
		res.UDPPortStatus = fmt.Sprintf("Gagal membuka port UDP: %v", err)
		return
	}
	defer conn.Close()
	res.UDPPortStatus = "Port UDP TERSEDIA"
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	target := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: conn.LocalAddr().(*net.UDPAddr).Port}
	_, _ = conn.WriteToUDP([]byte("DOCTOR_PING"), target)
	buf := make([]byte, 64)
	n, _, err := conn.ReadFromUDP(buf)
	res.BroadcastWorking = err == nil && string(buf[:n]) == "DOCTOR_PING"
}

func appendNetworkSuggestions(cfg *config.Config, res *DiagnosticResult) {
	if len(res.Interfaces) == 0 {
		res.Suggestions = append(res.Suggestions, "Tidak ada interface jaringan aktif. Pastikan laptop terhubung ke WiFi atau kabel LAN kantor.")
	} else {
		appendFirewallSuggestions(cfg, res)
	}
	if res.PeersFound == 0 {
		res.Suggestions = append(res.Suggestions,
			"Belum ada rekan yang terdeteksi di jaringan.",
			"Tips: 1) Pastikan kedua laptop terhubung ke WiFi kantor yang sama.",
			"Tips: 2) Jika broadcast terisolasi oleh Access Point, gunakan perintah '/add <ip_rekan>:42100' untuk koneksi langsung.",
		)
	}
}

func appendFirewallSuggestions(cfg *config.Config, res *DiagnosticResult) {
	switch runtime.GOOS {
	case "windows":
		res.Suggestions = append(res.Suggestions, "Windows Firewall: Pastikan profil jaringan disetel ke 'Private network' (bukan Public).")
		res.Suggestions = append(res.Suggestions, fmt.Sprintf("Bila rekan tidak terlihat, pastikan inbound port TCP %d dan UDP %d diizinkan di Windows Firewall.", cfg.TCPPort, cfg.UDPPort))
	case "darwin":
		res.Suggestions = append(res.Suggestions, "macOS: Pastikan izin 'Local Network' aktif untuk aplikasi terminal/Kerykeion Terminal di System Settings > Privacy & Security.")
	case "linux":
		res.Suggestions = append(res.Suggestions, fmt.Sprintf("Linux: Jika rekan tidak bisa terhubung dan UFW aktif, izinkan port: 'sudo ufw allow %d/tcp && sudo ufw allow %d/udp'.", cfg.TCPPort, cfg.UDPPort))
	}
}

func FormatReport(res DiagnosticResult) string {
	var sb strings.Builder
	sb.WriteString("=== HASIL DIAGNOSIS JARINGAN KERYKEION TERMINAL ===\n\n")
	sb.WriteString(fmt.Sprintf("Sistem Operasi: %s\n\n", res.OS))

	sb.WriteString("Interface Jaringan Aktif:\n")
	if len(res.Interfaces) == 0 {
		sb.WriteString("  [!] Tidak ada interface jaringan aktif ditemukan.\n")
	}
	for _, iface := range res.Interfaces {
		sb.WriteString(fmt.Sprintf("  - %s: IP %s (Netmask: %s, Broadcast: %s)\n", iface.Name, iface.IP, iface.Netmask, iface.Broadcast))
	}
	sb.WriteString("\n")

	sb.WriteString("Status Port:\n")
	sb.WriteString(fmt.Sprintf("  - TCP: %s\n", res.TCPPortStatus))
	sb.WriteString(fmt.Sprintf("  - UDP: %s\n", res.UDPPortStatus))
	sb.WriteString("\n")

	sb.WriteString("Deteksi Rekan (P2P):\n")
	sb.WriteString(fmt.Sprintf("  - Jumlah rekan terdeteksi saat ini: %d peer\n\n", res.PeersFound))

	if len(res.Suggestions) > 0 {
		sb.WriteString("Saran & Rekomendasi:\n")
		for _, s := range res.Suggestions {
			sb.WriteString(fmt.Sprintf("  * %s\n", s))
		}
	}
	sb.WriteString("\n===========================================\n")
	return sb.String()
}

func FormatReportEnglish(res DiagnosticResult, udpPort int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "KERYKEION NETWORK DIAGNOSTICS\n\nOperating system: %s\n\nNetwork interfaces:\n", res.OS)
	if len(res.Interfaces) == 0 {
		out.WriteString("No network interfaces found. Connect to the office LAN or WiFi.\n")
	}
	for _, iface := range res.Interfaces {
		fmt.Fprintf(&out, "  %s: %s (netmask %s, broadcast %s)\n", iface.Name, iface.IP, iface.Netmask, iface.Broadcast)
	}
	fmt.Fprintf(&out, "\nTCP port %d: ", res.TCPPort)
	switch res.TCPState {
	case "active":
		out.WriteString("ACTIVE (used by this application)\n")
	case "available":
		out.WriteString("AVAILABLE\n")
	case "occupied":
		out.WriteString("IN USE by another process or Kerykeion instance. Close that instance or use --port.\n")
	default:
		fmt.Fprintf(&out, "Unable to bind: %s\n", res.TCPError)
	}
	if res.BroadcastWorking {
		out.WriteString("UDP loopback: OK (LAN broadcast reachability is not tested)\n")
	} else {
		out.WriteString("UDP loopback: failed\n")
	}
	fmt.Fprintf(&out, "\nDiscovered peers: %d\n", res.PeersFound)
	if res.PeersFound == 0 {
		out.WriteString("Make sure peers are running Kerykeion on the same LAN.\nIf discovery is restricted, use /add <peer_ip>:42100.\n")
	}
	fmt.Fprintf(&out, "Allow inbound TCP %d and UDP %d if a firewall blocks peer connections.\n", res.TCPPort, udpPort)
	if res.OS == "linux" {
		fmt.Fprintf(&out, "If UFW is active: sudo ufw allow %d/tcp && sudo ufw allow %d/udp\n", res.TCPPort, udpPort)
	}
	return out.String()
}
