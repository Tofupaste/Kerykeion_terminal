package room

import (
	"net"
	"sort"
	"time"

	"kerykeion/pkg/protocol"
)

const AvailableRoomTTL = 10 * time.Second

type AvailableRoom struct {
	Name     string
	Gen      int64
	MasterID string
	HostID   string
	LastSeen time.Time
}

func (m *Manager) handleCatalogQuery(query protocol.RoomQueryPacket, source *net.UDPAddr, tcpPort int) {
	peer, ok := m.discovery.GetPeer(query.FromPeer)
	if !ok || !peer.IsOnline || source == nil || peer.IP != source.IP.String() {
		return
	}
	now := time.Now()
	m.availableMu.Lock()
	last := m.catalogReplies[query.FromPeer]
	if now.Sub(last) < 2*time.Second {
		m.availableMu.Unlock()
		return
	}
	m.catalogReplies[query.FromPeer] = now
	m.availableMu.Unlock()
	rooms, err := m.store.GetAllRooms()
	if err != nil {
		return
	}
	for _, record := range rooms {
		if record.State == "active" {
			m.discovery.SendRoomHere(source, record.Name, record.Gen, record.MasterID, tcpPort)
		}
	}
}

func (m *Manager) rememberAvailable(here protocol.RoomHerePacket) {
	m.availableMu.Lock()
	defer m.availableMu.Unlock()
	key := here.RoomName + "\x00" + here.FromPeer
	m.available[key] = AvailableRoom{Name: here.RoomName, Gen: here.Gen, MasterID: here.MasterID, HostID: here.FromPeer, LastSeen: time.Now()}
}

func (m *Manager) AvailableRooms() []AvailableRoom {
	return m.availableRoomsAt(time.Now())
}

func (m *Manager) availableRoomsAt(now time.Time) []AvailableRoom {
	m.availableMu.Lock()
	defer m.availableMu.Unlock()
	byName := make(map[string]AvailableRoom)
	for key, record := range m.available {
		if now.Sub(record.LastSeen) > AvailableRoomTTL {
			delete(m.available, key)
			continue
		}
		previous, ok := byName[record.Name]
		if !ok || record.LastSeen.After(previous.LastSeen) {
			byName[record.Name] = record
		}
	}
	rooms := make([]AvailableRoom, 0, len(byName))
	for _, record := range byName {
		rooms = append(rooms, record)
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].Name < rooms[j].Name })
	return rooms
}
