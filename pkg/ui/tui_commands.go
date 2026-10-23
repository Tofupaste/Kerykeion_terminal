package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transfer"
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
		m.addSystemLog(m.tr("Gunakan: /get <nomor|ID_file>"))
		return nil
	}
	reference := parts[1]
	if _, _, err := transfer.ParseFileReference(reference); err != nil {
		m.addSystemLog(m.tr("Gunakan: /get <nomor|ID_file>"))
		return nil
	}

	m.addSystemLog(m.tr("Meminta file %s...", reference))
	return func() tea.Msg {
		return MsgCommandResult{Err: m.transfer.PullFile(reference)}
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

func (m *AppModel) commandAI(parts []string) tea.Cmd {
	if m.aiSvc == nil {
		m.addSystemLog(m.tr("Layanan AI belum dikonfigurasi."))
		return nil
	}
	if len(parts) < 2 {
		m.addSystemLog(m.tr("Penggunaan /ai:\n  /ai <pertanyaan>            - Tanya model AI kantor\n  /ai model                   - Cek model aktif & daftar model server\n  /ai model <nama>            - Ganti model AI\n  /ai handoff <nama_rekan>    - Estafet sesi AI + Git Diff ke rekan\n  /ai ask @<rekan> <tanya>    - Pinjam/delegasi inferensi ke model rekan\n  /ai share                   - Bagikan riwayat sesi ke percakapan\n  /ai clear                   - Bersihkan riwayat percakapan sesi aktif"))
		return nil
	}

	sub := strings.ToLower(parts[1])
	switch sub {
	case "model":
		if len(parts) >= 3 {
			newModel := parts[2]
			m.aiSvc.SetModel(newModel)
			m.cfg.AIModel = newModel
			_ = m.cfg.Save()
			m.addSystemLog(m.tr("Model AI diubah ke: %s", newModel))
			return nil
		}
		aiSvc := m.aiSvc
		activeModel := aiSvc.Model()
		return func() tea.Msg {
			models, err := aiSvc.ListModels()
			if err != nil {
				return MsgCommandResult{Log: fmt.Sprintf("Gagal memuat model server: %v", err)}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("=== MODEL AI KANTOR (Aktif: %s) ===\n", activeModel))
			for _, mdl := range models {
				if mdl == activeModel {
					sb.WriteString(fmt.Sprintf("⭐ %s (sedang digunakan)\n", mdl))
				} else {
					sb.WriteString(fmt.Sprintf("- %s\n", mdl))
				}
			}
			sb.WriteString("\nGunakan: /ai model <nama_model> untuk mengganti.")
			return MsgCommandResult{Log: sb.String()}
		}
	case "clear":
		target := m.activeTarget
		if target == "" {
			target = "system"
		}
		m.aiSvc.ClearHistory(target)
		m.addSystemLog(m.tr("Riwayat percakapan AI untuk '%s' telah dibersihkan.", target))
		return nil
	case "share":
		target := m.activeTarget
		if target == "" {
			target = "system"
		}
		data, err := m.aiSvc.ExportHistory(target)
		if err != nil || m.aiSvc.HistoryCount(target) == 0 {
			m.addSystemLog(m.tr("Belum ada riwayat AI untuk dibagikan pada percakapan ini."))
			return nil
		}
		summary := fmt.Sprintf("[AI Session Context - %s] Riwayat percakapan (%d pesan):\n%s", m.aiSvc.Model(), m.aiSvc.HistoryCount(target), string(data))
		if m.activeTarget != "" {
			if m.isRoom {
				_, _ = m.messaging.SendRoomMessage(m.activeTarget, summary, "")
			} else {
				_, _ = m.messaging.SendDM(m.activeTarget, summary)
			}
			m.addSystemLog(m.tr("Riwayat sesi AI berhasil dibagikan ke percakapan aktif!"))
		} else {
			m.addSystemLog(summary)
		}
		return nil
	case "handoff":
		if len(parts) < 3 {
			m.addSystemLog(m.tr("Gunakan: /ai handoff <nama_rekan>"))
			return nil
		}
		targetPeer := strings.TrimPrefix(parts[2], "@")
		peer, ok := m.lookupPeer(targetPeer)
		if !ok {
			return nil
		}
		targetConv := m.activeTarget
		if targetConv == "" {
			targetConv = "project"
		}
		payload, err := m.aiSvc.CreateHandoffPayload(m.idMgr.PeerID(), targetConv)
		if err != nil {
			m.addSystemLog(m.tr("[Galat Handoff] %v", err))
			return nil
		}
		data, err := json.Marshal(payload)
		if err != nil {
			m.addSystemLog(m.tr("[Galat Handoff] %v", err))
			return nil
		}
		if m.transport != nil {
			if err := m.transport.SendToPeer(peer.PeerID, protocol.TypeAISessionSync, data); err != nil {
				m.addSystemLog(m.tr("Gagal mengirim handoff ke %s: %v", peer.DisplayName, err))
				return nil
			}
			diffInfo := ""
			if payload.GitDiff != "" {
				diffInfo = " beserta Git Diff terkini"
			}
			m.addSystemLog(m.tr("✓ Sesi AI (%d pesan)%s berhasil diserahkan ke @%s!", m.aiSvc.HistoryCount(targetConv), diffInfo, peer.DisplayName))
		}
		return nil
	case "ask":
		if len(parts) < 4 || !strings.HasPrefix(parts[2], "@") {
			m.addSystemLog(m.tr("Gunakan: /ai ask @<nama_rekan> [model] <pertanyaan>"))
			return nil
		}
		targetPeer := strings.TrimPrefix(parts[2], "@")
		peer, ok := m.lookupPeer(targetPeer)
		if !ok {
			return nil
		}

		targetModel := ""
		promptStart := 3
		candidateModel := strings.ToLower(parts[3])
		if len(parts) >= 5 && (strings.Contains(candidateModel, "claude") || strings.Contains(candidateModel, "qwen") || strings.Contains(candidateModel, "gemini") || strings.Contains(candidateModel, "codex") || strings.Contains(candidateModel, "gpt")) {
			targetModel = parts[3]
			promptStart = 4
		}
		prompt := strings.Join(parts[promptStart:], " ")
		reqID := uuid.NewString()

		if targetModel != "" {
			m.addSystemLog(m.tr("Meminta inferensi model [%s] dari @%s: \"%s\"...", targetModel, peer.DisplayName, prompt))
		} else {
			m.addSystemLog(m.tr("Meminta inferensi model dari @%s: \"%s\"...", peer.DisplayName, prompt))
		}

		return func() tea.Msg {
			reqPayload := protocol.AIQueryReqPayload{
				V:           1,
				ReqID:       reqID,
				FromPeerID:  m.idMgr.PeerID(),
				TargetModel: targetModel,
				Prompt:      prompt,
				TS:          time.Now().Unix(),
			}
			reqBytes, err := json.Marshal(reqPayload)
			if err != nil {
				return MsgCommandResult{Log: fmt.Sprintf("Gagal membentuk query: %v", err)}
			}
			ch := m.aiSvc.RegisterPendingQuery(reqID)
			defer m.aiSvc.UnregisterPendingQuery(reqID)

			if m.transport == nil {
				return MsgCommandResult{Log: "Transport belum siap"}
			}
			if err := m.transport.SendToPeer(peer.PeerID, protocol.TypeAIQueryReq, reqBytes); err != nil {
				return MsgCommandResult{Log: fmt.Sprintf("Gagal mengirim query ke @%s: %v", peer.DisplayName, err)}
			}

			select {
			case res := <-ch:
				if res.Error != "" {
					return MsgCommandResult{Log: fmt.Sprintf("[Galat AI dari @%s] %s", peer.DisplayName, res.Error)}
				}
				return MsgCommandResult{Log: fmt.Sprintf("[%s via @%s]: %s", res.Model, peer.DisplayName, res.Answer)}
			case <-time.After(60 * time.Second):
				return MsgCommandResult{Log: fmt.Sprintf("Batas waktu menunggu respons AI dari @%s habis.", peer.DisplayName)}
			}
		}
	default:
		prompt := strings.Join(parts[1:], " ")
		target := m.activeTarget
		if target == "" {
			target = "system"
		}
		aiSvc := m.aiSvc
		m.addSystemLog(m.tr("Mengirim pertanyaan ke model AI [%s]...", aiSvc.Model()))

		return func() tea.Msg {
			ans, err := aiSvc.Ask(target, prompt)
			if err != nil {
				return MsgCommandResult{Log: fmt.Sprintf("[Galat AI] %v", err)}
			}
			return MsgCommandResult{Log: ans}
		}
	}
}
