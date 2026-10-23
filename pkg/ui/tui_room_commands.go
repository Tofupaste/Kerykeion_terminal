package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *AppModel) commandRoomNew(parts []string) tea.Cmd {
	if len(parts) < 3 {
		m.addSystemLog(m.tr("Gunakan: /room new <nama_room>"))
		return nil
	}
	name := parts[2]
	return func() tea.Msg {
		rec, err := m.roomMgr.CreateRoom(name)
		return MsgRoomJoined{Room: rec, WasCreated: true, Err: err}
	}
}

func (m *AppModel) commandRoomList(parts []string) tea.Cmd {
	m.refreshRoomsAndPeers()
	var sb strings.Builder
	sb.WriteString(m.tr("=== DAFTAR ROOM YANG DIIKUTI ===\n"))
	for _, r := range m.rooms {
		role := "guest"
		if r.MyRole == "master" {
			role = "master"
		}
		sb.WriteString(fmt.Sprintf("- #%s [%s]\n", r.Name, role))
	}
	m.addSystemLog(sb.String())
	return nil
}

func (m *AppModel) commandRoomInfo(parts []string) tea.Cmd {
	name := m.activeTarget
	if len(parts) >= 3 {
		name = parts[2]
	}
	rec, err := m.store.GetRoom(name)
	if err != nil || rec == nil {
		m.addSystemLog(m.tr("Room tidak ditemukan."))
		return nil
	}
	members, _ := m.store.GetRoomMembers(name)
	var sb strings.Builder
	sb.WriteString(m.tr("=== INFO ROOM: #%s ===\n", rec.Name))
	sb.WriteString(fmt.Sprintf("Master : %s\n", rec.MasterID))
	sb.WriteString(m.tr("Peran  : %s\n", rec.MyRole))
	sb.WriteString(m.tr("Dibuat : %s\n", rec.CreatedAt.Format("02 Jan 2006 15:04")))
	sb.WriteString(m.tr("Anggota (%d):\n", len(members)))
	for _, mID := range members {
		name := shortPeerID(mID)
		if p, ok := m.discovery.GetPeer(mID); ok {
			name = peerLabel(*p, m.peers)
		}
		sb.WriteString(fmt.Sprintf("  - %s\n", name))
	}
	m.addSystemLog(sb.String())
	return nil
}

func (m *AppModel) commandRoomTransfer(parts []string) tea.Cmd {
	if len(parts) < 4 {
		m.addSystemLog(m.tr("Gunakan: /room transfer <nama_room> <rekan>"))
		return nil
	}
	targetPeer, ok := m.discovery.FindPeerByName(parts[3])
	if !ok {
		m.addSystemLog(m.tr("Rekan '%s' tidak ditemukan.", parts[3]))
		return nil
	}
	name, peerID, displayName := parts[2], targetPeer.PeerID, targetPeer.DisplayName
	language := m.cfg.Language
	return func() tea.Msg {
		err := m.roomMgr.TransferMaster(name, peerID)
		return MsgCommandResult{Err: err, Log: localized(language, "Kepemilikan room #%s diserahkan ke %s", name, displayName)}
	}
}
