package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/doctor"
)

func (m *AppModel) commandLang(parts []string) tea.Cmd {
	if len(parts) != 2 {
		m.addSystemLog(m.tr("Gunakan: /lang id atau /lang en"))
	} else {
		m.changeLanguage(strings.ToLower(parts[1]))
	}
	return nil
}

func (m *AppModel) commandHelp(parts []string) tea.Cmd {
	if len(parts) > 1 {
		m.addSystemLog(detailHelp(m.cfg.Language, strings.Join(parts[1:], " ")))
	} else {
		m.addSystemLog(quickHelp(m.cfg.Language))
	}
	return nil
}

func (m *AppModel) commandWho(parts []string) tea.Cmd {
	m.refreshRoomsAndPeers()
	var sb strings.Builder
	sb.WriteString(m.tr("=== DAFTAR REKAN DI LAN ===\n"))
	sb.WriteString(m.tr("Identitas kamu: %s\nID: %s\n", m.idMgr.DisplayName(), m.idMgr.PeerID()))
	if len(m.peers) == 0 {
		sb.WriteString(m.tr("Belum ada rekan terdeteksi. Pastikan rekan sudah membuka aplikasi dan satu WiFi.\n"))
	}
	for _, p := range m.peers {
		status := "online"
		if !p.IsOnline {
			status = "stale"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s) IP %s:%d\n  ID: %s\n", peerLabel(p, m.peers), status, p.IP, p.Port, p.PeerID))
	}
	m.addSystemLog(sb.String())
	return nil
}

func (m *AppModel) commandDm(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /dm <nama|ID|nama#ID>"))
		return nil
	}
	target := parts[1]
	peer, ok := m.lookupPeer(target)
	if !ok {
		return nil
	}
	m.activeTarget = peer.PeerID
	m.isRoom = false
	m.showChat()
	m.loadMessagesForTarget()
	m.statusText = m.tr("DM dengan %s", peerLabel(*peer, m.peers))
	m.addSystemLog(m.tr("Membuka percakapan pribadi dengan %s", peerLabel(*peer, m.peers)))
	return nil
}

func (m *AppModel) commandJoin(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /join <nama_room>"))
		return nil
	}
	roomName := parts[1]
	cleanName := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(roomName)), "#")
	m.addSystemLog(m.tr("Mencari atau membuat room '#%s'...", cleanName))
	return func() tea.Msg {
		rec, joined, err := m.roomMgr.JoinOrCreateRoom(cleanName, m.cfg.TCPPort)
		return MsgRoomJoined{
			Room:       rec,
			WasCreated: !joined,
			Err:        err,
		}
	}
}

func (m *AppModel) commandRoom(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Perintah room yang tersedia:\n• /room new <nama> - Buat room baru\n• /room list - Daftar room yang diikuti\n• /room info [nama] - Detail dan anggota room\n• /room transfer <nama> <rekan> - Pindahkan master"))
		return nil
	}
	sub := strings.ToLower(parts[1])
	handlers := map[string]func([]string) tea.Cmd{
		"new":      m.commandRoomNew,
		"list":     m.commandRoomList,
		"info":     m.commandRoomInfo,
		"transfer": m.commandRoomTransfer,
	}
	if handler, ok := handlers[sub]; ok {
		return handler(parts)
	}

	return nil
}

func (m *AppModel) commandGo(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /go <nama_room_atau_rekan>"))
		return nil
	}
	target := strings.TrimPrefix(parts[1], "#")
	for _, r := range m.rooms {
		if r.Name == target {
			m.activeTarget = r.Name
			m.isRoom = true
			m.showChat()
			m.loadMessagesForTarget()
			m.statusText = m.tr("Room: #%s", r.Name)
			return nil
		}
	}
	if p, ok := m.lookupPeer(target); ok {
		m.activeTarget = p.PeerID
		m.isRoom = false
		m.showChat()
		m.loadMessagesForTarget()
		m.statusText = m.tr("DM dengan %s", peerLabel(*p, m.peers))
		return nil
	}
	return nil
}

func (m *AppModel) commandSend(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /send <path_file>"))
		return nil
	}
	filePath := parts[1]
	if m.activeTarget == "" {
		m.addSystemLog(m.tr("Pilih percakapan aktif terlebih dahulu."))
		return nil
	}
	return m.sendAttachment(filePath, m.activeTarget, m.isRoom)
}

func (m *AppModel) commandGet(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /get <nomor_pengumuman>"))
		return nil
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		m.addSystemLog(m.tr("Nomor pengumuman tidak valid."))
		return nil
	}
	m.addSystemLog(m.tr("Meminta file #%d...", num))
	return func() tea.Msg {
		return MsgCommandResult{Err: m.transfer.PullAnnouncedFile(num)}
	}
}

func (m *AppModel) commandLeave(parts []string) tea.Cmd {
	target := m.activeTarget
	if len(parts) >= 2 {
		target = parts[1]
	}
	return func() tea.Msg {
		return MsgCommandResult{Err: m.roomMgr.LeaveRoom(target), ClearTarget: target}
	}
}

func (m *AppModel) commandDisband(parts []string) tea.Cmd {
	target := m.activeTarget
	if len(parts) >= 2 {
		target = parts[1]
	}
	return func() tea.Msg {
		return MsgCommandResult{Err: m.roomMgr.DisbandRoom(target), ClearTarget: target}
	}
}

func (m *AppModel) commandName(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /name <nama_baru>"))
		return nil
	}
	newName := parts[1]
	m.idMgr.SetDisplayName(newName)
	m.cfg.DisplayName = newName
	_ = m.cfg.Save()
	_ = m.store.SaveIdentity(m.idMgr.PeerID(), newName)
	m.addSystemLog(m.tr("Nama tampilan diubah menjadi: %s", newName))
	return nil
}

func (m *AppModel) commandOpen(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /open <nomor_link>"))
		return nil
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		m.addSystemLog(m.tr("Nomor link tidak valid."))
		return nil
	}
	link, ok := m.messaging.GetLink(num)
	if !ok {
		m.addSystemLog(m.tr("Link #%d tidak ditemukan.", num))
		return nil
	}
	openBrowser(link)
	m.addSystemLog(m.tr("Membuka link: %s", link))
	return nil
}

func (m *AppModel) commandDoctor(parts []string) tea.Cmd {
	cfg := *m.cfg
	return func() tea.Msg {
		report := doctor.RunDiagnosticsForApp(&cfg, m.discovery, cfg.TCPPort)
		text := doctor.FormatReport(report)
		if cfg.Language == "en" {
			text = doctor.FormatReportEnglish(report, cfg.UDPPort)
		}
		return MsgCommandResult{Log: text}
	}
}

func (m *AppModel) commandAdd(parts []string) tea.Cmd {
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Gunakan: /add <ip:port>"))
		return nil
	}
	_, err := m.discovery.AddManualPeer(parts[1])
	if err != nil {
		m.addSystemLog(m.tr("[Galat Add] %v", err))
	} else {
		m.addSystemLog(m.tr("Mengirim broadcast langsung ke %s", parts[1]))
	}
	return nil
}

func (m *AppModel) commandQuit(parts []string) tea.Cmd {
	return tea.Quit
}
