package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/doctor"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
)

type AppModel struct {
	cfg        *config.Config
	idMgr      *identity.Manager
	store      *db.Store
	discovery  *discovery.Discovery
	messaging  *messaging.Service
	roomMgr    *room.Manager
	transfer   *transfer.Manager

	width      int
	height     int
	viewport   viewport.Model
	textInput  textinput.Model

	activeTarget string // room name (e.g. "ngobs") or peerID (e.g. "abc...")
	isRoom       bool
	initialRoom  string

	messages   []db.MessageRecord
	peers      []discovery.PeerInfo
	rooms      []db.RoomRecord
	fileAnnounceMap map[int]protocol.FileAnnouncePayload

	systemLogs []string
	statusText string
}

type MsgNewMessage db.MessageRecord
type MsgStatusChange struct {
	MsgID  string
	Status string
}
type MsgProgress transfer.TransferProgress
type MsgAnnounce protocol.FileAnnouncePayload
type MsgPeerDiscovered discovery.PeerInfo
type MsgPeerUpdated discovery.PeerInfo
type MsgRoomEvent struct {
	Event  string
	Room   string
	Detail string
}
type MsgRoomJoined struct {
	Room       *db.RoomRecord
	WasCreated bool
	Err        error
}

func NewTUI(
	cfg *config.Config,
	idMgr *identity.Manager,
	store *db.Store,
	disc *discovery.Discovery,
	msgSvc *messaging.Service,
	rMgr *room.Manager,
	tMgr *transfer.Manager,
	initialRoom string,
) *AppModel {
	ti := textinput.New()
	ti.Placeholder = "Ketik pesan atau /help untuk bantuan..."
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 60

	vp := viewport.New(60, 20)

	m := &AppModel{
		cfg:             cfg,
		idMgr:           idMgr,
		store:           store,
		discovery:       disc,
		messaging:       msgSvc,
		roomMgr:         rMgr,
		transfer:        tMgr,
		textInput:       ti,
		viewport:        vp,
		initialRoom:     initialRoom,
		fileAnnounceMap: make(map[int]protocol.FileAnnouncePayload),
		statusText:      "Ketik /help untuk panduan",
	}

	m.refreshRoomsAndPeers()
	return m
}

func (m *AppModel) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, textinput.Blink)
	if m.initialRoom != "" {
		target := m.initialRoom
		cmds = append(cmds, func() tea.Msg {
			rec, joined, err := m.roomMgr.JoinOrCreateRoom(target, m.cfg.TCPPort)
			return MsgRoomJoined{
				Room:       rec,
				WasCreated: !joined,
				Err:        err,
			}
		})
	}
	return tea.Batch(cmds...)
}

func (m *AppModel) refreshRoomsAndPeers() {
	if m.discovery != nil {
		m.peers = m.discovery.GetPeers()
	}
	if m.store != nil {
		rooms, err := m.store.GetAllRooms()
		if err == nil {
			var active []db.RoomRecord
			for _, r := range rooms {
				if r.State == "active" {
					active = append(active, r)
				}
			}
			m.rooms = active
		}
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit

		case tea.KeyTab:
			m.handleTabCompletion()
			return m, nil

		case tea.KeyEnter:
			val := strings.TrimSpace(m.textInput.Value())
			if val != "" {
				cmd := m.handleUserInput(val)
				m.textInput.SetValue("")
				m.textInput.Reset()
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return m, tea.Batch(cmds...)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		sidebarWidth := 26
		contentWidth := m.width - sidebarWidth - 4
		if contentWidth < 30 {
			contentWidth = 30
		}
		contentHeight := m.height - 5
		if contentHeight < 5 {
			contentHeight = 5
		}

		m.viewport.Width = contentWidth
		m.viewport.Height = contentHeight
		m.textInput.Width = contentWidth - 4
		m.updateViewportContent()

	case MsgNewMessage:
		rec := db.MessageRecord(msg)
		if rec.ConversationID == m.activeTarget {
			m.messages = append(m.messages, rec)
			m.updateViewportContent()
			m.viewport.GotoBottom()
		}

	case MsgStatusChange:
		for i, existing := range m.messages {
			if existing.MsgID == msg.MsgID {
				m.messages[i].Status = msg.Status
				m.updateViewportContent()
				break
			}
		}

	case MsgProgress:
		if msg.ErrorMessage != "" {
			m.addSystemLog(fmt.Sprintf("[Galat Transfer] %s: %s", msg.Filename, msg.ErrorMessage))
		} else if msg.IsCompleted {
			dir := "Terkirim"
			if msg.IsDownload {
				dir = "Selesai diunduh ke downloads/"
			}
			m.addSystemLog(fmt.Sprintf("[Transfer] %s %s", msg.Filename, dir))
		} else if msg.SpeedBps > 0 {
			pct := int((msg.SentBytes * 100) / msg.TotalBytes)
			m.statusText = fmt.Sprintf("Transfer %s: %d%% (%s/s)", msg.Filename, pct, formatBytes(msg.SpeedBps))
		}

	case MsgAnnounce:
		m.fileAnnounceMap[msg.AnnounceNo] = protocol.FileAnnouncePayload(msg)
		announcementText := fmt.Sprintf("[#%d] %s (%s) dari %s", msg.AnnounceNo, msg.Filename, formatBytes(msg.Size), msg.SenderID)
		m.addSystemLog(fmt.Sprintf("[Pengumuman File] %s. Ketik /get %d untuk unduh.", announcementText, msg.AnnounceNo))

	case MsgPeerDiscovered, MsgPeerUpdated:
		m.refreshRoomsAndPeers()

	case MsgRoomEvent:
		m.addSystemLog(fmt.Sprintf("[%s] %s: %s", msg.Event, msg.Room, msg.Detail))
		m.refreshRoomsAndPeers()

	case MsgRoomJoined:
		if msg.Err != nil {
			m.addSystemLog(fmt.Sprintf("[Galat] %v", msg.Err))
		} else if msg.Room != nil {
			m.activeTarget = msg.Room.Name
			m.isRoom = true
			m.loadMessagesForTarget()
			m.refreshRoomsAndPeers()
			m.statusText = fmt.Sprintf("Room: #%s", msg.Room.Name)
			if msg.WasCreated {
				m.addSystemLog(fmt.Sprintf("Room #%s berhasil dibuat sebagai master!", msg.Room.Name))
			} else {
				m.addSystemLog(fmt.Sprintf("Berhasil bergabung ke room #%s!", msg.Room.Name))
			}
		}
		m.updateViewportContent()
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

func (m *AppModel) handleTabCompletion() {
	val := m.textInput.Value()
	if strings.HasPrefix(val, "/") {
		parts := strings.Fields(val)
		if len(parts) == 1 {
			prefix := strings.TrimPrefix(parts[0], "/")
			for cmd := range CommandCatalog {
				if strings.HasPrefix(cmd, prefix) {
					m.textInput.SetValue("/" + cmd + " ")
					m.textInput.SetCursor(len("/" + cmd + " "))
					return
				}
			}
		} else if len(parts) == 2 && (parts[0] == "/join" || parts[0] == "/go") {
			prefix := parts[1]
			for _, r := range m.rooms {
				if strings.HasPrefix(r.Name, prefix) {
					m.textInput.SetValue(fmt.Sprintf("%s %s", parts[0], r.Name))
					m.textInput.SetCursor(len(m.textInput.Value()))
					return
				}
			}
		}
	}
}

func (m *AppModel) handleUserInput(input string) tea.Cmd {
	if strings.HasPrefix(input, "/") {
		return m.executeCommand(input)
	}

	if m.activeTarget == "" {
		m.addSystemLog("Belum ada percakapan aktif. Ketik /join <nama_room> atau /dm <nama_rekan>.")
		return nil
	}

	if m.isRoom {
		_, _ = m.messaging.SendRoomMessage(m.activeTarget, input, "")
	} else {
		_, _ = m.messaging.SendDM(m.activeTarget, input)
	}
	return nil
}

func (m *AppModel) executeCommand(raw string) tea.Cmd {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		if len(parts) > 1 {
			m.addSystemLog(FormatDetailHelp(strings.Join(parts[1:], " ")))
		} else {
			m.addSystemLog(FormatQuickHelp())
		}
		return nil

	case "/who":
		m.refreshRoomsAndPeers()
		var sb strings.Builder
		sb.WriteString("=== DAFTAR REKAN DI LAN ===\n")
		if len(m.peers) == 0 {
			sb.WriteString("Belum ada rekan terdeteksi. Pastikan rekan sudah membuka aplikasi dan satu WiFi.\n")
		}
		for _, p := range m.peers {
			status := "🟢 online"
			if !p.IsOnline {
				status = "⚪ stale"
			}
			sb.WriteString(fmt.Sprintf("- %s (%s) IP %s:%d\n", p.DisplayName, status, p.IP, p.Port))
		}
		m.addSystemLog(sb.String())
		return nil

	case "/dm":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /dm <nama_rekan>")
			return nil
		}
		target := parts[1]
		peer, ok := m.discovery.FindPeerByName(target)
		if !ok {
			for _, p := range m.peers {
				if strings.HasPrefix(p.PeerID, target) {
					peer = &p
					ok = true
					break
				}
			}
		}
		if !ok {
			m.addSystemLog(fmt.Sprintf("Rekan '%s' tidak ditemukan. Ketik /who untuk melihat daftar.", target))
			return nil
		}
		m.activeTarget = peer.PeerID
		m.isRoom = false
		m.loadMessagesForTarget()
		m.statusText = fmt.Sprintf("DM dengan %s", peer.DisplayName)
		m.addSystemLog(fmt.Sprintf("Membuka percakapan pribadi dengan %s", peer.DisplayName))
		return nil

	case "/join":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /join <nama_room>")
			return nil
		}
		roomName := parts[1]
		cleanName := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(roomName)), "#")
		m.addSystemLog(fmt.Sprintf("Mencari atau membuat room '#%s'...", cleanName))
		return func() tea.Msg {
			rec, joined, err := m.roomMgr.JoinOrCreateRoom(cleanName, m.cfg.TCPPort)
			return MsgRoomJoined{
				Room:       rec,
				WasCreated: !joined,
				Err:        err,
			}
		}

	case "/room":
		if len(parts) < 2 {
			m.addSystemLog("Perintah room yang tersedia:\n• /room new <nama> - Buat room baru\n• /room list - Daftar room yang diikuti\n• /room info [nama] - Detail dan anggota room\n• /room transfer <nama> <rekan> - Pindahkan master")
			return nil
		}
		sub := strings.ToLower(parts[1])
		switch sub {
		case "new":
			if len(parts) < 3 {
				m.addSystemLog("Gunakan: /room new <nama_room>")
				return nil
			}
			rec, err := m.roomMgr.CreateRoom(parts[2])
			if err != nil {
				m.addSystemLog(fmt.Sprintf("[Galat] %v", err))
				return nil
			}
			m.activeTarget = rec.Name
			m.isRoom = true
			m.loadMessagesForTarget()
			m.refreshRoomsAndPeers()
			m.statusText = fmt.Sprintf("Room: #%s", rec.Name)
			m.addSystemLog(fmt.Sprintf("Room #%s berhasil dibuat!", rec.Name))
			return nil

		case "list":
			m.refreshRoomsAndPeers()
			var sb strings.Builder
			sb.WriteString("=== DAFTAR ROOM YANG DIIKUTI ===\n")
			for _, r := range m.rooms {
				role := "guest"
				if r.MyRole == "master" {
					role = "👑 master"
				}
				sb.WriteString(fmt.Sprintf("- #%s [%s]\n", r.Name, role))
			}
			m.addSystemLog(sb.String())
			return nil

		case "info":
			name := m.activeTarget
			if len(parts) >= 3 {
				name = parts[2]
			}
			rec, err := m.store.GetRoom(name)
			if err != nil || rec == nil {
				m.addSystemLog("Room tidak ditemukan.")
				return nil
			}
			members, _ := m.store.GetRoomMembers(name)
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("=== INFO ROOM: #%s ===\n", rec.Name))
			sb.WriteString(fmt.Sprintf("Master : %s\n", rec.MasterID))
			sb.WriteString(fmt.Sprintf("Peran  : %s\n", rec.MyRole))
			sb.WriteString(fmt.Sprintf("Dibuat : %s\n", rec.CreatedAt.Format("02 Jan 2006 15:04")))
			sb.WriteString(fmt.Sprintf("Anggota (%d):\n", len(members)))
			for _, mID := range members {
				name := mID[:6]
				if p, ok := m.discovery.GetPeer(mID); ok {
					name = p.DisplayName
				}
				sb.WriteString(fmt.Sprintf("  - %s\n", name))
			}
			m.addSystemLog(sb.String())

		case "transfer":
			if len(parts) < 4 {
				m.addSystemLog("Gunakan: /room transfer <nama_room> <rekan>")
				return nil
			}
			targetPeer, ok := m.discovery.FindPeerByName(parts[3])
			if !ok {
				m.addSystemLog(fmt.Sprintf("Rekan '%s' tidak ditemukan.", parts[3]))
				return nil
			}
			err := m.roomMgr.TransferMaster(parts[2], targetPeer.PeerID)
			if err != nil {
				m.addSystemLog(fmt.Sprintf("[Galat] %v", err))
				return nil
			}
			m.addSystemLog(fmt.Sprintf("Kepemilikan room #%s diserahkan ke %s", parts[2], targetPeer.DisplayName))
			return nil
		}
		return nil

	case "/go":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /go <nama_room_atau_rekan>")
			return nil
		}
		target := strings.TrimPrefix(parts[1], "#")
		for _, r := range m.rooms {
			if r.Name == target {
				m.activeTarget = r.Name
				m.isRoom = true
				m.loadMessagesForTarget()
				m.statusText = fmt.Sprintf("Room: #%s", r.Name)
				return nil
			}
		}
		if p, ok := m.discovery.FindPeerByName(target); ok {
			m.activeTarget = p.PeerID
			m.isRoom = false
			m.loadMessagesForTarget()
			m.statusText = fmt.Sprintf("DM dengan %s", p.DisplayName)
			return nil
		}
		m.addSystemLog(fmt.Sprintf("Percakapan '%s' tidak ditemukan.", target))
		return nil

	case "/send":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /send <path_file>")
			return nil
		}
		filePath := parts[1]
		if m.activeTarget == "" {
			m.addSystemLog("Pilih percakapan aktif terlebih dahulu.")
			return nil
		}
		go func() {
			if m.isRoom {
				_, err := m.transfer.SendFile(filePath, "", m.activeTarget)
				if err != nil {
					m.addSystemLog(fmt.Sprintf("[Galat Send] %v", err))
				}
			} else {
				_, err := m.transfer.SendFile(filePath, m.activeTarget, "")
				if err != nil {
					m.addSystemLog(fmt.Sprintf("[Galat Send] %v", err))
				}
			}
		}()
		return nil

	case "/get":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /get <nomor_pengumuman>")
			return nil
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			m.addSystemLog("Nomor pengumuman tidak valid.")
			return nil
		}
		go func() {
			err := m.transfer.PullAnnouncedFile(num)
			if err != nil {
				m.addSystemLog(fmt.Sprintf("[Galat Get] %v", err))
			}
		}()
		return nil

	case "/leave":
		target := m.activeTarget
		if len(parts) >= 2 {
			target = parts[1]
		}
		err := m.roomMgr.LeaveRoom(target)
		if err != nil {
			m.addSystemLog(fmt.Sprintf("[Galat Leave] %v", err))
			return nil
		}
		m.activeTarget = ""
		m.messages = nil
		m.updateViewportContent()
		m.refreshRoomsAndPeers()
		return nil

	case "/disband":
		target := m.activeTarget
		if len(parts) >= 2 {
			target = parts[1]
		}
		err := m.roomMgr.DisbandRoom(target)
		if err != nil {
			m.addSystemLog(fmt.Sprintf("[Galat Disband] %v", err))
			return nil
		}
		m.activeTarget = ""
		m.messages = nil
		m.updateViewportContent()
		m.refreshRoomsAndPeers()
		return nil

	case "/name":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /name <nama_baru>")
			return nil
		}
		newName := parts[1]
		m.idMgr.SetDisplayName(newName)
		m.cfg.DisplayName = newName
		_ = m.cfg.Save()
		_ = m.store.SaveIdentity(m.idMgr.PeerID(), newName)
		m.addSystemLog(fmt.Sprintf("Nama tampilan diubah menjadi: %s", newName))
		return nil

	case "/open":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /open <nomor_link>")
			return nil
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			m.addSystemLog("Nomor link tidak valid.")
			return nil
		}
		link, ok := m.messaging.GetLink(num)
		if !ok {
			m.addSystemLog(fmt.Sprintf("Link #%d tidak ditemukan.", num))
			return nil
		}
		openBrowser(link)
		m.addSystemLog(fmt.Sprintf("Membuka link: %s", link))
		return nil

	case "/doctor":
		report := doctor.RunDiagnostics(m.cfg, m.discovery)
		m.addSystemLog(doctor.FormatReport(report))
		return nil

	case "/add":
		if len(parts) < 2 {
			m.addSystemLog("Gunakan: /add <ip:port>")
			return nil
		}
		_, err := m.discovery.AddManualPeer(parts[1])
		if err != nil {
			m.addSystemLog(fmt.Sprintf("[Galat Add] %v", err))
		} else {
			m.addSystemLog(fmt.Sprintf("Mengirim broadcast langsung ke %s", parts[1]))
		}
		return nil

	case "/quit":
		return tea.Quit

	default:
		sugg := SuggestCommand(cmd)
		if sugg != "" {
			m.addSystemLog(fmt.Sprintf("Perintah '%s' tidak dikenal. Maksud kamu %s?", cmd, sugg))
		} else {
			m.addSystemLog(fmt.Sprintf("Perintah '%s' tidak dikenal. Ketik /help untuk panduan.", cmd))
		}
		return nil
	}
}

func (m *AppModel) loadMessagesForTarget() {
	if m.activeTarget == "" {
		m.messages = nil
		m.updateViewportContent()
		return
	}
	msgs, err := m.store.GetMessages(m.activeTarget, 100)
	if err == nil {
		m.messages = msgs
	} else {
		m.messages = nil
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *AppModel) addSystemLog(logMsg string) {
	m.systemLogs = append(m.systemLogs, logMsg)
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *AppModel) updateViewportContent() {
	var sb strings.Builder

	if m.activeTarget == "" {
		sb.WriteString("Selamat datang di Kerykeion Terminal (P2P LAN)!\n\n")
		sb.WriteString("Ketik /join <nama_room> untuk masuk atau buat room.\n")
		sb.WriteString("Ketik /who untuk melihat rekan yang aktif.\n")
		sb.WriteString("Ketik /help untuk panduan lengkap perintah.\n\n")
	} else {
		sb.WriteString(fmt.Sprintf("=== PERCAKAPAN: %s ===\n\n", m.activeTarget))
		for _, msg := range m.messages {
			tStr := time.Unix(msg.Timestamp, 0).Format("15:04")
			authorName := msg.AuthorID[:6]
			if msg.AuthorID == m.idMgr.PeerID() {
				authorName = "Kamu"
			} else if p, ok := m.discovery.GetPeer(msg.AuthorID); ok {
				authorName = p.DisplayName
			}

			statusMark := ""
			if msg.AuthorID == m.idMgr.PeerID() {
				switch msg.Status {
				case "queued":
					statusMark = " ⏳"
				case "sent":
					statusMark = " ✓"
				case "delivered":
					statusMark = " ✓✓"
				}
			}

			sb.WriteString(fmt.Sprintf("[%s] %s%s: %s\n", tStr, authorName, statusMark, msg.Body))
		}
	}

	if len(m.systemLogs) > 0 {
		sb.WriteString("\n--- Pesan Sistem ---\n")
		start := 0
		if len(m.systemLogs) > 10 {
			start = len(m.systemLogs) - 10
		}
		for _, log := range m.systemLogs[start:] {
			sb.WriteString(fmt.Sprintf("• %s\n", log))
		}
	}

	m.viewport.SetContent(sb.String())
}

func (m *AppModel) View() string {
	sidebarWidth := 26
	mainWidth := m.width - sidebarWidth - 4
	if mainWidth < 20 {
		mainWidth = 20
	}

	sidebarStyle := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(m.height - 4).
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderForeground(lipgloss.Color("63"))

	mainStyle := lipgloss.NewStyle().
		Width(mainWidth).
		Height(m.height - 4)

	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("KERYKEION") + "\n")
	sb.WriteString(fmt.Sprintf("ID: %s\n", m.idMgr.DisplayName()) + "\n")

	sb.WriteString(lipgloss.NewStyle().Bold(true).Render("ROOMS:") + "\n")
	if len(m.rooms) == 0 {
		sb.WriteString(" (belum ada)\n")
	}
	for _, r := range m.rooms {
		prefix := "  #"
		if m.isRoom && m.activeTarget == r.Name {
			prefix = " >#"
		}
		crown := ""
		if r.MyRole == "master" {
			crown = " 👑"
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\n", prefix, r.Name, crown))
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("REKAN:") + "\n")
	if len(m.peers) == 0 {
		sb.WriteString(" (belum ada)\n")
	}
	for _, p := range m.peers {
		dot := "🟢"
		if !p.IsOnline {
			dot = "⚪"
		}
		prefix := "  "
		if !m.isRoom && m.activeTarget == p.PeerID {
			prefix = " >"
		}
		sb.WriteString(fmt.Sprintf("%s%s %s\n", prefix, dot, p.DisplayName))
	}

	sidebarView := sidebarStyle.Render(sb.String())
	mainView := mainStyle.Render(m.viewport.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, " ", mainView)

	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Width(m.width).
		Render(fmt.Sprintf(" %s | Bantuan: /help | Keluar: Ctrl+C", m.statusText))

	inputView := m.textInput.View()

	return lipgloss.JoinVertical(lipgloss.Left, content, statusStyle, inputView)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	_ = exec.Command(cmd, args...).Start()
}
