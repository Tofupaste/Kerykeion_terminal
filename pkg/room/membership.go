package room

import (
	"fmt"
	"slices"
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/protocol"
)

func (m *Manager) sendRoomFrame(peerID string, frame byte, payload []byte) error {
	if err := m.transport.SendToPeer(peerID, frame, payload); err == nil {
		return nil
	}
	if peer, ok := m.discovery.GetPeer(peerID); ok {
		conn, err := m.transport.DialPeer(peerID, peer.IP, peer.Port, "chat")
		if err != nil {
			return err
		}
		return conn.WriteFrame(frame, payload)
	}
	peer, err := m.store.GetPeer(peerID)
	if err != nil {
		return err
	}
	if peer == nil {
		return fmt.Errorf("room peer %s is unavailable", peerID)
	}
	conn, err := m.transport.DialPeer(peerID, peer.LastIP, peer.LastPort, "chat")
	if err != nil {
		return err
	}
	return conn.WriteFrame(frame, payload)
}

func (m *Manager) peerPublicKey(peerID string) string {
	if peerID == m.idMgr.PeerID() {
		return m.idMgr.PublicKeyBase64()
	}
	if peer, ok := m.discovery.GetPeer(peerID); ok {
		return peer.PubKey
	}
	if peer, err := m.store.GetPeer(peerID); err == nil && peer != nil {
		return peer.PubKey
	}
	return ""
}

type membershipWait struct {
	peerID string
	ready  chan struct{}
}

func (m *Manager) connectRoom(name string, gen int64, peerID string) error {
	wait := membershipWait{peerID: peerID, ready: make(chan struct{}, 1)}
	m.pendingMembersMu.Lock()
	m.pendingMembers[name] = wait
	m.pendingMembersMu.Unlock()
	defer func() {
		m.pendingMembersMu.Lock()
		delete(m.pendingMembers, name)
		m.pendingMembersMu.Unlock()
	}()
	m.broadcastJoinEvent(name, gen)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-wait.ready:
	case <-timer.C:
		return fmt.Errorf("room %s did not confirm membership; try /join again", name)
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
	m.syncRoomHistory(name, peerID)
	return nil
}

func (m *Manager) reconnectRoom(rec *db.RoomRecord) {
	m.broadcastJoinEvent(rec.Name, rec.Gen)
	members, _ := m.store.GetRoomMembers(rec.Name)
	for _, id := range members {
		if id != m.idMgr.PeerID() {
			m.syncRoomHistory(rec.Name, id)
		}
	}
}

func (m *Manager) broadcastRoomMeta(rec *db.RoomRecord) {
	members, err := m.store.GetRoomMembers(rec.Name)
	if err != nil {
		return
	}
	payload, err := protocol.MarshalPayload(protocol.RoomMetaPayload{V: 1, Name: rec.Name, Gen: rec.Gen, MasterID: rec.MasterID, Members: members})
	if err != nil {
		return
	}
	for _, id := range members {
		if id != m.idMgr.PeerID() {
			_ = m.sendRoomFrame(id, protocol.TypeRoomMeta, payload)
		}
	}
}

func (m *Manager) roomMember(name, peerID string) bool {
	members, err := m.store.GetRoomMembers(name)
	return err == nil && slices.Contains(members, peerID)
}

func (m *Manager) HandleRoomMeta(payload []byte, fromPeerID string) {
	var meta protocol.RoomMetaPayload
	if protocol.UnmarshalPayload(payload, &meta) != nil {
		return
	}
	rec, err := m.store.GetRoom(meta.Name)
	if err != nil || rec == nil || rec.State != "active" || rec.Gen != meta.Gen || rec.MasterID != meta.MasterID {
		return
	}
	if !m.roomMember(meta.Name, fromPeerID) {
		return
	}
	for _, id := range meta.Members {
		_ = m.store.AddRoomMember(meta.Name, id)
	}
	m.confirmMembership(meta.Name, fromPeerID)
	if m.onEvent != nil {
		m.onEvent("ROOM_MEMBERS_SYNCED", meta.Name, "Daftar anggota room diperbarui")
	}
}

func (m *Manager) confirmMembership(name, peerID string) {
	m.pendingMembersMu.Lock()
	defer m.pendingMembersMu.Unlock()
	wait, ok := m.pendingMembers[name]
	if !ok || wait.peerID != peerID {
		return
	}
	select {
	case wait.ready <- struct{}{}:
	default:
	}
}
