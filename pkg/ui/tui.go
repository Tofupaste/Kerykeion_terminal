package ui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/ai"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

type AppModel struct {
	cfg       *config.Config
	idMgr     *identity.Manager
	store     *db.Store
	discovery *discovery.Discovery
	messaging *messaging.Service
	roomMgr   *room.Manager
	transfer  *transfer.Manager
	transport *transport.Manager
	aiSvc     *ai.Service

	width          int
	height         int
	viewport       viewport.Model
	systemViewport viewport.Model
	textInput      textinput.Model
	chatVisible    bool
	systemFocused  bool
	unread         int

	activeTarget string // room name (e.g. "ngobs") or peerID (e.g. "abc...")
	isRoom       bool
	initialRoom  string

	messages        []db.MessageRecord
	peers           []discovery.PeerInfo
	rooms           []db.RoomRecord
	fileAnnounceMap map[int]protocol.FileAnnouncePayload

	systemLogs       []string
	statusText       string
	picker           *attachmentPicker
	pickerDirectory  string
	pickerGeneration uint64
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
type MsgCommandResult struct {
	Err         error
	Log         string
	ClearTarget string
}

type MsgRoomJoined struct {
	Room       *db.RoomRecord
	WasCreated bool
	Err        error
}

func NewTUI(
	services Services,
	initialRoom string,
) *AppModel {
	ti := textinput.New()
	ti.Placeholder = localized(services.Config.Language, "Ketik perintah atau /help untuk bantuan...")
	ti.Focus()
	ti.CharLimit = 1000
	ti.Width = 60

	vp := viewport.New(60, 20)

	m := &AppModel{
		cfg:             services.Config,
		idMgr:           services.Identity,
		store:           services.Store,
		discovery:       services.Discovery,
		messaging:       services.Messaging,
		roomMgr:         services.Rooms,
		transfer:        services.Transfers,
		transport:       services.Transport,
		aiSvc:           services.AI,
		textInput:       ti,
		viewport:        vp,
		systemViewport:  viewport.New(60, 20),
		width:           80,
		height:          24,
		systemFocused:   true,
		initialRoom:     initialRoom,
		fileAnnounceMap: make(map[int]protocol.FileAnnouncePayload),
		statusText:      localized(services.Config.Language, defaultStatusText),
	}

	m.refreshRoomsAndPeers()
	m.resizePanels()
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

func (m *AppModel) handleTabCompletion() {
	if m.chatVisible && !m.systemFocused {
		return
	}
	val := m.textInput.Value()
	if !strings.HasPrefix(val, "/") {
		return
	}
	parts := strings.Fields(val)
	if len(parts) == 1 {
		m.completeCommand(parts[0])
		return
	}
	if len(parts) == 2 && (parts[0] == "/join" || parts[0] == "/go") {
		m.completeRoom(parts[0], parts[1])
	}
}

func (m *AppModel) setCompletion(value string) {
	m.textInput.SetValue(value)
	m.textInput.SetCursor(len(value))
}

func (m *AppModel) completeCommand(input string) {
	prefix := strings.TrimPrefix(input, "/")
	for command := range CommandCatalog {
		if strings.HasPrefix(command, prefix) {
			m.setCompletion("/" + command + " ")
			return
		}
	}
}

func (m *AppModel) completeRoom(command, prefix string) {
	for _, room := range m.rooms {
		if strings.HasPrefix(room.Name, prefix) {
			m.setCompletion(command + " " + room.Name)
			return
		}
	}
}

func (m *AppModel) handleUserInput(input string) tea.Cmd {
	if m.systemFocused || !m.chatVisible {
		if strings.HasPrefix(input, "/") {
			return m.executeCommand(input)
		}
		m.addSystemLog(m.tr("Panel sistem menerima perintah /... saja. Tekan F6 untuk fokus chat, atau F4 jika chat diminimalkan."))
		return nil
	}

	if m.activeTarget == "" {
		m.addSystemLog(m.tr("Belum ada percakapan aktif. Ketik /join <nama_room> atau /dm <nama_rekan>."))
		return nil
	}

	target, isRoom := m.activeTarget, m.isRoom
	return func() tea.Msg {
		var err error
		if isRoom {
			_, err = m.messaging.SendRoomMessage(target, input, "")
		} else {
			_, err = m.messaging.SendDM(target, input)
		}
		if err != nil {
			return MsgCommandResult{Err: err}
		}

		if strings.HasPrefix(input, "@ai ") && m.aiSvc != nil {
			prompt := strings.TrimSpace(strings.TrimPrefix(input, "@ai "))
			go func() {
				ans, askErr := m.aiSvc.Ask(target, prompt)
				if askErr == nil {
					if isRoom {
						_, _ = m.messaging.SendRoomMessage(target, ans, "")
					} else {
						_, _ = m.messaging.SendDM(target, ans)
					}
				}
			}()
		}

		return MsgCommandResult{}
	}
}

func (m *AppModel) executeCommand(raw string) tea.Cmd {
	parts := commandParts(raw)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])

	handlers := map[string]func([]string) tea.Cmd{
		"/lang":     m.commandLang,
		"/language": m.commandLang,
		"/help":     m.commandHelp,
		"/who":      m.commandWho,
		"/dm":       m.commandDm,
		"/join":     m.commandJoin,
		"/room":     m.commandRoom,
		"/go":       m.commandGo,
		"/send":     m.commandSend,
		"/get":      m.commandGet,
		"/leave":    m.commandLeave,
		"/disband":  m.commandDisband,
		"/name":     m.commandName,
		"/open":     m.commandOpen,
		"/doctor":   m.commandDoctor,
		"/add":      m.commandAdd,
		"/ai":       m.commandAI,
		"/quit":     m.commandQuit,
	}
	if handler, ok := handlers[cmd]; ok {
		return handler(parts)
	}

	sugg := SuggestCommand(cmd)
	if sugg != "" {
		m.addSystemLog(m.tr("Perintah '%s' tidak dikenal. Maksud kamu %s?", cmd, sugg))
	} else {
		m.addSystemLog(m.tr("Perintah '%s' tidak dikenal. Ketik /help untuk panduan.", cmd))
	}
	return nil
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
	if len(m.systemLogs) > 500 {
		m.systemLogs = m.systemLogs[len(m.systemLogs)-500:]
	}
	m.updateViewportContent()
	m.systemViewport.GotoBottom()
}

func (m *AppModel) updateViewportContent() {
	var sb strings.Builder

	if m.activeTarget != "" {
		for _, msg := range m.messages {
			tStr := time.Unix(msg.Timestamp, 0).Format("15:04")
			authorName := m.messageAuthor(msg.AuthorID)
			statusMark := m.messageStatus(msg)

			sb.WriteString(fmt.Sprintf("[%s] %s%s: %s\n", tStr, authorName, statusMark, msg.Body))
		}
	}

	if sb.Len() == 0 {
		sb.WriteString(m.tr("Belum ada pesan. Ketik pesan lalu Enter untuk mengirim."))
	}
	m.viewport.SetContent(wrapTerminalText(sb.String(), m.viewport.Width))
	systemText := m.tr("Kerykeion Terminal siap.\n/join <nama> untuk membuka room, /dm <rekan> untuk pesan pribadi.\n/help untuk panduan. F2 untuk mengganti bahasa.")
	if len(m.systemLogs) > 0 {
		systemText += "\n\n" + strings.Join(m.systemLogs, "\n\n")
	}
	m.systemViewport.SetContent(wrapTerminalText(systemText, m.systemViewport.Width))
}

func (m *AppModel) messageAuthor(authorID string) string {
	if authorID == m.idMgr.PeerID() {
		return m.tr("Kamu")
	}
	if m.discovery != nil {
		if peer, ok := m.discovery.GetPeer(authorID); ok {
			return peerLabel(*peer, m.peers)
		}
	}
	return shortPeerID(authorID)
}

func (m *AppModel) messageStatus(message db.MessageRecord) string {
	if message.AuthorID != m.idMgr.PeerID() {
		return ""
	}
	switch message.Status {
	case "queued":
		return m.tr(" [antre]")
	case "sent":
		return " ✓"
	case "delivered":
		return " ✓✓"
	default:
		return ""
	}
}

func (m *AppModel) View() string {
	if m.picker != nil {
		return m.renderAttachmentPicker()
	}
	return m.renderPanels()
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
