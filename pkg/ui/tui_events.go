package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/db"
	"kerykeion/pkg/protocol"
)

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(clipboardResult); ok {
		m.applyClipboardResult(result)
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.copyIDs != nil {
		return m, m.handleCopyKey(key)
	}
	if result, ok := msg.(attachmentDirectoryResult); ok {
		m.applyAttachmentDirectory(result)
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && m.picker != nil {
		return m, m.handleAttachmentKey(key)
	}
	switch msg := msg.(type) {
	case roomCatalogTick:
		m.refreshRoomCatalog()
		return m, roomCatalogTickCommand()
	case tea.KeyMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizePanels()
	case MsgNewMessage:
		m.receiveMessage(db.MessageRecord(msg))
	case MsgStatusChange:
		m.updateMessageStatus(msg)
	case MsgProgress:
		m.updateTransferProgress(msg)
	case MsgAnnounce:
		m.receiveAnnouncement(msg)
	case MsgPeerDiscovered, MsgPeerUpdated:
		m.refreshRoomsAndPeers()
	case MsgRoomEvent:
		m.applyRoomEvent(msg)
	case MsgCommandResult:
		m.applyCommandResult(msg)
	case MsgRoomJoined:
		m.applyRoomJoined(msg)
	}
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m *AppModel) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyCtrlC:
		return tea.Quit, true
	case tea.KeyCtrlY:
		m.openCopyIDs()
		return nil, true
	case tea.KeyCtrlO:
		return m.openAttachmentPicker(), true
	case tea.KeyEsc:
		m.minimizeChat()
	case tea.KeyF2:
		language := "en"
		if m.cfg.Language == "en" {
			language = "id"
		}
		m.changeLanguage(language)
	case tea.KeyF4:
		m.restoreChat()
	case tea.KeyF6:
		if m.chatVisible {
			m.systemFocused = !m.systemFocused
			m.resizePanels()
		}
	case tea.KeyPgUp, tea.KeyPgDown, tea.KeyCtrlHome, tea.KeyCtrlEnd:
		return m.scrollFocusedPanel(msg), true
	case tea.KeyTab:
		m.handleTabCompletion()
	case tea.KeyEnter:
		return m.submitInput(), true
	default:
		return nil, false
	}
	return nil, true
}

func (m *AppModel) submitInput() tea.Cmd {
	value := strings.TrimSpace(m.textInput.Value())
	if value == "" {
		return nil
	}
	cmd := m.handleUserInput(value)
	m.textInput.SetValue("")
	m.textInput.Reset()
	return cmd
}

func (m *AppModel) scrollFocusedPanel(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.systemFocused || !m.chatVisible {
		m.systemViewport, cmd = m.systemViewport.Update(msg)
	} else {
		m.viewport, cmd = m.viewport.Update(msg)
	}
	return cmd
}

func (m *AppModel) minimizeChat() {
	if !m.chatVisible {
		return
	}
	m.chatVisible = false
	m.systemFocused = true
	m.resizePanels()
}

func (m *AppModel) restoreChat() {
	if m.activeTarget == "" {
		m.addSystemLog(m.tr("Tidak ada chat aktif. Gunakan /join <nama> atau /dm <rekan>."))
		return
	}
	if !m.chatVisible {
		m.showChat()
	}
}

func (m *AppModel) clearChat() {
	m.picker = nil
	m.activeTarget = ""
	m.messages = nil
	m.chatVisible = false
	m.systemFocused = true
	m.statusText = m.tr(defaultStatusText)
}

func (m *AppModel) receiveMessage(record db.MessageRecord) {
	if record.ConversationID != m.activeTarget {
		return
	}
	m.messages = append(m.messages, record)
	if !m.chatVisible {
		m.unread++
	}
	m.updateViewportContent()
	m.viewport.GotoBottom()
}

func (m *AppModel) updateMessageStatus(status MsgStatusChange) {
	for i, message := range m.messages {
		if message.MsgID == status.MsgID {
			m.messages[i].Status = status.Status
			m.updateViewportContent()
			return
		}
	}
}

func (m *AppModel) updateTransferProgress(progress MsgProgress) {
	if progress.ErrorMessage != "" {
		m.statusText = m.tr("Transfer gagal: %s", progress.Filename)
		m.addSystemLog(m.tr("[Galat Transfer] %s: %s", progress.Filename, localized(m.cfg.Language, progress.ErrorMessage)))
		return
	}
	if progress.IsCompleted {
		m.statusText = m.tr("Transfer selesai: %s", progress.Filename)
		if progress.IsDownload && progress.LocalPath != "" {
			m.addSystemLog(m.tr("[Transfer] %s disimpan di %s", progress.Filename, progress.LocalPath))
			return
		}
		direction := m.tr("Terkirim")
		if progress.IsDownload {
			direction = m.tr("Selesai diunduh ke downloads/")
		}
		m.addSystemLog(fmt.Sprintf("[Transfer] %s %s", progress.Filename, direction))
		return
	}
	if progress.TotalBytes > 0 {
		percent := int(progress.SentBytes * 100 / progress.TotalBytes)
		m.statusText = fmt.Sprintf("Transfer %s: %d%%", progress.Filename, percent)
		if progress.SpeedBps > 0 {
			m.statusText += fmt.Sprintf(" (%s/s)", formatBytes(progress.SpeedBps))
		}
	}
}

func (m *AppModel) receiveAnnouncement(announcement MsgAnnounce) {
	m.fileAnnounceMap[announcement.AnnounceNo] = protocol.FileAnnouncePayload(announcement)
	text := m.tr("[#%d] %s (%s) dari %s", announcement.AnnounceNo, announcement.Filename, formatBytes(announcement.Size), announcement.SenderID)
	m.addSystemLog(m.tr("[Pengumuman File] %s. Ketik /get %d untuk unduh.", text, announcement.AnnounceNo))
}

func (m *AppModel) applyRoomEvent(event MsgRoomEvent) {
	m.addSystemLog(m.roomEventText(event))
	if event.Event == "ROOM_HISTORY_SYNCED" && m.isRoom && m.activeTarget == event.Room {
		m.loadMessagesForTarget()
		m.updateViewportContent()
	}
	if event.Event == "ROOM_DISBANDED" && m.isRoom && m.activeTarget == event.Room {
		m.clearChat()
		m.resizePanels()
	}
	m.refreshRoomsAndPeers()
}

func (m *AppModel) applyCommandResult(result MsgCommandResult) {
	if result.Err != nil {
		m.addSystemLog(m.tr("[Galat] %v", localized(m.cfg.Language, result.Err.Error())))
		return
	}
	if result.ClearTarget != "" && m.activeTarget == result.ClearTarget {
		m.clearChat()
	}
	if result.Log != "" {
		m.addSystemLog(result.Log)
	}
	m.refreshRoomsAndPeers()
	m.resizePanels()
}

func (m *AppModel) applyRoomJoined(result MsgRoomJoined) {
	if result.Err != nil {
		m.addSystemLog(m.tr("[Galat] %v", localized(m.cfg.Language, result.Err.Error())))
	} else if result.Room != nil {
		m.activeTarget = result.Room.Name
		m.isRoom = true
		m.showChat()
		m.loadMessagesForTarget()
		m.refreshRoomsAndPeers()
		m.statusText = m.tr("Room: #%s", result.Room.Name)
		text := "Berhasil bergabung ke room #%s!"
		if result.WasCreated {
			text = "Room #%s berhasil dibuat sebagai master!"
		}
		m.addSystemLog(m.tr(text, result.Room.Name))
	}
	m.updateViewportContent()
}
