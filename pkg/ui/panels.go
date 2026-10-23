package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"kerykeion/pkg/discovery"
)

func shortPeerID(id string) string { return peerIDPrefix(id, nil) }

func wrapTerminalText(text string, width int) string {
	return ansi.Wrap(text, max(1, width), "")
}

func (m *AppModel) panelArea() (int, int, int) {
	sidebar := 0
	if m.width >= 100 {
		sidebar = 24
	}
	width := max(1, m.width-sidebar)
	return width, max(1, m.height-4), sidebar
}

func (m *AppModel) resizePanels() {
	width, height, _ := m.panelArea()
	m.systemViewport.Width = max(1, width-3)
	m.systemViewport.Height = max(1, height-3)
	m.viewport.Width = max(1, width-3)
	m.viewport.Height = max(1, height-3)
	if m.chatVisible {
		if width >= 100 {
			systemWidth := width * 2 / 5
			m.systemViewport.Width = systemWidth - 3
			m.viewport.Width = width - systemWidth - 3
		} else {
			systemHeight := max(4, height/3)
			m.systemViewport.Height = max(1, systemHeight-3)
			m.viewport.Height = max(1, height-systemHeight-3)
		}
	}
	m.textInput.Width = max(1, m.width-4)
	if m.chatVisible && !m.systemFocused {
		m.textInput.Placeholder = m.tr("Ketik pesan; Ctrl+O file; F6 perintah...")
	} else {
		m.textInput.Placeholder = m.tr("Ketik perintah atau /help untuk bantuan...")
	}
	m.updateViewportContent()
}

func (m *AppModel) showChat() {
	m.chatVisible = true
	m.systemFocused = false
	m.unread = 0
	m.resizePanels()
}

func panel(title, body string, width, height int, focused bool) string {
	style := lipgloss.NewStyle().Bold(true)
	if focused {
		style = style.Reverse(true)
		title = "> " + title
	}
	title = style.Render(ansi.Truncate(title, max(1, width), "…"))
	return lipgloss.NewStyle().Width(width).Height(height + 1).
		Border(lipgloss.NormalBorder()).Render(title + "\n" + body)
}

func scrollIndicator(height, totalLines, offset int) string {
	if height <= 0 {
		return ""
	}
	thumbSize, thumbStart := height, 0
	if totalLines > height {
		thumbSize = max(1, height*height/totalLines)
		maxOffset := totalLines - height
		offset = max(0, min(offset, maxOffset))
		thumbStart = (offset*(height-thumbSize) + maxOffset/2) / maxOffset
	}
	rows := make([]string, height)
	for row := range rows {
		rows[row] = "│"
		if row >= thumbStart && row < thumbStart+thumbSize {
			rows[row] = "┃"
		}
	}
	return strings.Join(rows, "\n")
}

func viewportWithScrollbar(vp viewport.Model) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, vp.View(), scrollIndicator(vp.Height, vp.TotalLineCount(), vp.YOffset))
}

func (m *AppModel) conversationTitle() string {
	if m.isRoom {
		return "#" + m.activeTarget
	}
	if m.discovery != nil {
		if peer, ok := m.discovery.GetPeer(m.activeTarget); ok {
			return "@" + peerLabel(*peer, m.peers)
		}
	}
	return "@" + shortPeerID(m.activeTarget)
}

func (m *AppModel) renderSidebar(height int) string {
	lines := m.roomSidebarLines()
	lines = append(lines, "", m.tr("Rekan"))
	if len(m.peers) == 0 {
		lines = append(lines, m.tr("Belum ada rekan terdeteksi. /doctor untuk diagnosis."))
	}
	for _, peer := range m.peers {
		mark := "  "
		if !m.isRoom && m.activeTarget == peer.PeerID {
			mark = "> "
		}
		status := "online"
		if !peer.IsOnline {
			status = "stale"
		}
		lines = append(lines, mark+peer.DisplayName, "  #"+peerIDPrefix(peer.PeerID, m.peers)+" ["+status+"]")
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], 22, "…")
	}
	return lipgloss.NewStyle().Width(22).Height(height).MaxHeight(height).
		BorderRight(true).BorderStyle(lipgloss.NormalBorder()).Render(strings.Join(lines, "\n")) + " "
}

func (m *AppModel) renderPanels() string {
	if m.width < 32 || m.height < 12 {
		return ansi.Truncate(m.tr("Perbesar terminal (minimal 32 × 12)."), max(1, m.width), "")
	}
	_, height, sidebar := m.panelArea()
	system := panel(m.tr("Pesan sistem"), viewportWithScrollbar(m.systemViewport), m.systemViewport.Width+1, m.systemViewport.Height, m.systemFocused)
	content := system
	if m.chatVisible {
		title := m.tr("Percakapan") + " · " + m.conversationTitle()
		chat := panel(title, viewportWithScrollbar(m.viewport), m.viewport.Width+1, m.viewport.Height, !m.systemFocused)
		width, _, _ := m.panelArea()
		if width >= 100 {
			content = lipgloss.JoinHorizontal(lipgloss.Top, system, chat)
		} else {
			content = lipgloss.JoinVertical(lipgloss.Left, system, chat)
		}
	}
	if sidebar > 0 {
		content = lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(height), content)
	}
	language := "Indonesia"
	if m.cfg.Language == "en" {
		language = "English"
	}
	header := lipgloss.NewStyle().Bold(true).Render("KERYKEION") + " · " + peerLabel(discovery.PeerInfo{DisplayName: m.idMgr.DisplayName(), PeerID: m.idMgr.PeerID()}, m.peers) + " · " + language
	status := m.statusText
	if m.activeTarget != "" && !m.chatVisible {
		status = m.tr("Chat diminimalkan: %s", m.conversationTitle())
		if m.unread > 0 {
			status += fmt.Sprintf(" (+%d)", m.unread)
		}
	} else if m.chatVisible {
		status += " · " + m.tr("Esc Minimalkan chat")
	}
	hints := m.panelHints()
	return strings.Join([]string{
		ansi.Truncate(header, m.width, "…"), content,
		ansi.Truncate(status, m.width, "…"), ansi.Truncate(hints, m.width, "…"),
		ansi.Truncate(m.textInput.View(), m.width, ""),
	}, "\n")
}

func (m *AppModel) panelHints() string {
	hints := m.tr("F2 Bahasa · F4 Chat · F6 Fokus · PgUp/PgDn Gulir · Ctrl+C Keluar")
	if m.chatVisible {
		hints = m.tr("F2 Bahasa · F6 Fokus · PgUp/PgDn Gulir · Ctrl+C Keluar")
		if !m.systemFocused {
			hints = "Ctrl+O " + m.tr("Lampirkan file") + " · " + hints
		}
	}
	return "Ctrl+Y " + m.tr("Salin ID") + " · " + hints
}

func (m *AppModel) roomSidebarLines() []string {
	var lines []string
	lines = append(lines, m.tr("Room"))
	if len(m.rooms) == 0 {
		lines = append(lines, m.tr("Belum ada room. /join <nama> untuk mulai."))
	}
	for _, room := range m.rooms {
		mark := "  "
		if m.isRoom && m.activeTarget == room.Name {
			mark = "> "
		}
		role := ""
		if room.MyRole == "master" {
			role = " *"
		}
		lines = append(lines, mark+"#"+room.Name+role)
	}
	lines = append(lines, "", m.tr("Tersedia di LAN"))
	for _, name := range m.availableRoomNames() {
		lines = append(lines, "  #"+name)
	}

	return lines
}
