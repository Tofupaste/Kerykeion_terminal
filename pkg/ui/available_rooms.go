package ui

import (
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/discovery"
)

type roomCatalogTick struct{}

func roomCatalogTickCommand() tea.Cmd {
	return tea.Tick(discovery.RoomRefreshInterval, func(time.Time) tea.Msg { return roomCatalogTick{} })
}

func (m *AppModel) availableRoomNames() []string {
	if m.roomMgr == nil {
		return nil
	}
	joined := make(map[string]bool)
	for _, record := range m.rooms {
		joined[record.Name] = true
	}
	var names []string
	for _, record := range m.roomMgr.AvailableRooms() {
		if !joined[record.Name] {
			names = append(names, record.Name)
		}
	}
	return names
}

func (m *AppModel) refreshRoomCatalog() {
	m.refreshRoomsAndPeers()
	names := m.availableRoomNames()
	if slices.Equal(names, m.availableNames) {
		return
	}
	m.availableNames = names
	if len(names) == 0 {
		m.addSystemLog(m.tr("Tidak ada room LAN lain yang terdeteksi."))
		return
	}
	m.addSystemLog(m.tr("Room tersedia: #%s. Gunakan /join <nama>.", strings.Join(names, ", #")))
}

func availableRoomList(language string, names []string) string {
	text := localized(language, "Room tersedia di LAN (refresh 3 detik):")
	if len(names) == 0 {
		return text + "\n" + localized(language, "Tidak ada room LAN lain yang terdeteksi.")
	}
	for _, name := range names {
		text += "\n  /join " + name
	}
	return text
}
