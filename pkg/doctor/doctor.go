package doctor

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	"kantorchat/pkg/config"
	"kantorchat/pkg/discovery"
)

type DiagnosticResult struct {
	OS               string
	Interfaces       []InterfaceDetail
	TCPPortStatus    string
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
	res := DiagnosticResult{
		OS: runtime.GOOS,
	}

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if !ok || ipnet.IP.To4() == nil {
					continue
				}
				ip4 := ipnet.IP.To4()
				mask := ipnet.Mask
				bcast := ""
				if len(mask) == 4 {
					bcast = net.IPv4(
						ip4[0]|^mask[0],
						ip4[1]|^mask[1],
						ip4[2]|^mask[2],
						ip4[3]|^mask[3],
					).String()
				}
				res.Interfaces = append(res.Interfaces, InterfaceDetail{
					Name:      iface.Name,
					IP:        ip4.String(),
					Netmask:   net.IP(mask).String(),
					Broadcast: bcast,
					IsUp:      iface.Flags&net.FlagUp != 0,
				})
			}
		}
	}

	// Test TCP port bind
	tcpLn, err := net.Listen("tcp4", fmt.Sprintf(":%d", cfg.TCPPort))
	if err == nil {
		res.TCPPortStatus = fmt.Sprintf("Port %d TCP TERSEDIA", cfg.TCPPort)
		_ = tcpLn.Close()
	} else {
		res.TCPPortStatus = fmt.Sprintf("Port %d TCP TERPAKAI atau DIBLOKIR: %v", cfg.TCPPort, err)
	}

	// Test UDP port bind & broadcast
	udpAddr := &net.UDPAddr{Port: 0, IP: net.IPv4zero}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err == nil {
		res.UDPPortStatus = fmt.Sprintf("Port UDP TERSEDIA")
		_ = udpConn.SetDeadline(time.Now().Add(500 * time.Millisecond))
		testTarget := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: udpConn.LocalAddr().(*net.UDPAddr).Port}
		_, _ = udpConn.WriteToUDP([]byte("DOCTOR_PING"), testTarget)

		buf := make([]byte, 64)
		n, _, err := udpConn.ReadFromUDP(buf)
		if err == nil && string(buf[:n]) == "DOCTOR_PING" {
			res.BroadcastWorking = true
		}
		_ = udpConn.Close()
	} else {
		res.UDPPortStatus = fmt.Sprintf("Gagal membuka port UDP: %v", err)
	}

	if disc != nil {
		peers := disc.GetPeers()
		res.PeersFound = len(peers)
	}

	// Generate helpful advice
	if len(res.Interfaces) == 0 {
		res.Suggestions = append(res.Suggestions, "Tidak ada interface jaringan aktif. Pastikan laptop terhubung ke WiFi atau kabel LAN kantor.")
	} else {
		switch runtime.GOOS {
		case "windows":
			res.Suggestions = append(res.Suggestions, "Windows Firewall: Pastikan profil jaringan disetel ke 'Private network' (bukan Public).")
			res.Suggestions = append(res.Suggestions, fmt.Sprintf("Bila rekan tidak terlihat, pastikan inbound port TCP %d dan UDP %d diizinkan di Windows Firewall.", cfg.TCPPort, cfg.UDPPort))
		case "darwin":
			res.Suggestions = append(res.Suggestions, "macOS: Pastikan izin 'Local Network' aktif untuk aplikasi terminal/KantorChat di System Settings > Privacy & Security.")
		case "linux":
			res.Suggestions = append(res.Suggestions, fmt.Sprintf("Linux: Jika memakai ufw/iptables, izinkan port: 'sudo ufw allow %d/tcp && sudo ufw allow %d/udp'.", cfg.TCPPort, cfg.UDPPort))
		}
	}

	if res.PeersFound == 0 {
		res.Suggestions = append(res.Suggestions, "Belum ada rekan yang terdeteksi di jaringan.")
		res.Suggestions = append(res.Suggestions, "Tips: 1) Pastikan kedua laptop terhubung ke WiFi kantor yang sama.")
		res.Suggestions = append(res.Suggestions, "Tips: 2) Jika broadcast terisolasi oleh Access Point, gunakan perintah '/add <ip_rekan>:42100' untuk koneksi langsung.")
	}

	return res
}

func FormatReport(res DiagnosticResult) string {
	var sb strings.Builder
	sb.WriteString("=== HASIL DIAGNOSIS JARINGAN KANTORCHAT ===\n\n")
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
