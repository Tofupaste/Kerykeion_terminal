package transfer

import (
	"fmt"
	"os"
	"slices"

	"kerykeion/pkg/protocol"
)

func (m *Manager) roomMember(room, peerID string) bool {
	rec, err := m.store.GetRoom(room)
	if err != nil || rec == nil || rec.State != "active" {
		return false
	}
	members, err := m.store.GetRoomMembers(room)
	return err == nil && slices.Contains(members, peerID)
}

func (m *Manager) sendPeerFrame(peerID string, frame byte, payload []byte) error {
	if err := m.transport.SendToPeer(peerID, frame, payload); err == nil {
		return nil
	}
	peer, ok := m.discovery.GetPeer(peerID)
	if ok {
		conn, err := m.transport.DialPeer(peerID, peer.IP, peer.Port, "chat")
		if err != nil {
			return err
		}
		return conn.WriteFrame(frame, payload)
	}
	stored, err := m.store.GetPeer(peerID)
	if err != nil || stored == nil {
		return fmt.Errorf("peer %s is unavailable", peerID)
	}
	conn, err := m.transport.DialPeer(peerID, stored.LastIP, stored.LastPort, "chat")
	if err != nil {
		return err
	}
	return conn.WriteFrame(frame, payload)
}

func (m *Manager) registerAnnouncement(announce protocol.FileAnnouncePayload) (protocol.FileAnnouncePayload, bool) {
	m.announcementsMu.Lock()
	defer m.announcementsMu.Unlock()
	for _, known := range m.announcements {
		if known.Room == announce.Room && known.FileID == announce.FileID && known.SenderID == announce.SenderID {
			return known, false
		}
	}
	announce.AnnounceNo = m.nextAnnounceNo
	m.nextAnnounceNo++
	m.announcements[announce.AnnounceNo] = announce
	return announce, true
}

func (m *Manager) announceRoomFile(offer protocol.FileOfferPayload, path string) error {
	thumbnail, _ := m.GenerateThumbnail(path)
	announce, _ := m.registerAnnouncement(protocol.FileAnnouncePayload{V: 1, Room: offer.Room, FileID: offer.FileID, Filename: offer.Filename, Size: offer.Size, SenderID: m.idMgr.PeerID(), Thumbnail: thumbnail})
	if m.onAnnounce != nil {
		m.onAnnounce(announce)
	}
	payload, err := protocol.MarshalPayload(announce)
	if err != nil {
		return err
	}
	members, err := m.store.GetRoomMembers(offer.Room)
	if err != nil {
		return err
	}
	for _, id := range members {
		if id != m.idMgr.PeerID() {
			if err := m.sendPeerFrame(id, protocol.TypeFileAnnounce, payload); err != nil {
				m.reportProgress(uploadProgress(offer, 0, 0, false, fmt.Sprintf("file announcement to %s failed: %v", id, err)))
			}
		}
	}
	return nil
}

func (m *Manager) HandleFileAnnounce(payload []byte, fromPeerID string) {
	var announce protocol.FileAnnouncePayload
	if protocol.UnmarshalPayload(payload, &announce) != nil {
		return
	}
	if announce.SenderID != fromPeerID || !m.roomMember(announce.Room, fromPeerID) {
		return
	}
	if !validFileID(announce.FileID) || announce.Size < 0 || announce.Size > m.cfg.MaxFileSize {
		return
	}
	announce, fresh := m.registerAnnouncement(announce)
	if !fresh {
		return
	}
	if m.onAnnounce != nil {
		m.onAnnounce(announce)
	}
	if announce.Size <= m.cfg.AutoDownloadImageMax && announce.Thumbnail != "" {
		m.startWorker(func() {
			if err := m.PullAnnouncedFile(announce.AnnounceNo); err != nil {
				m.reportProgress(TransferProgress{FileID: announce.FileID, Filename: announce.Filename, IsDownload: true, ErrorMessage: err.Error()})
			}
		})
	}
}

func (m *Manager) announcedFile(room, id string) (protocol.FileAnnouncePayload, bool) {
	m.announcementsMu.RLock()
	for _, announce := range m.announcements {
		if announce.Room == room && announce.FileID == id {
			m.announcementsMu.RUnlock()
			return announce, true
		}
	}
	m.announcementsMu.RUnlock()
	messages, _ := m.store.GetFileMessages(id)
	for _, message := range messages {
		if message.ConversationID == room {
			if announce, ok := attachmentAnnouncement(message); ok {
				announce.Room = room
				return announce, true
			}
		}
	}
	return protocol.FileAnnouncePayload{}, false
}

func (m *Manager) cachedRoomFile(room, id, requester string) (string, error) {
	if !validFileID(id) {
		return "", fmt.Errorf("invalid file ID")
	}
	if room == "" {
		if _, ok := m.directFileForPeer(id, requester); !ok {
			return "", fmt.Errorf("this file was not sent to you")
		}
	} else {
		if !m.roomMember(room, requester) {
			return "", fmt.Errorf("file is not available to this room member")
		}
		if _, ok := m.announcedFile(room, id); !ok {
			return "", fmt.Errorf("file was not announced in this room")
		}
	}

	cached, err := m.store.GetMediaCache(id)
	if err != nil {
		return "", fmt.Errorf("cached file is unavailable")
	}
	info, err := os.Stat(cached.Path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("cached file is missing")
	}
	return cached.Path, nil
}

func (m *Manager) HandleFileRequest(connID string, payload []byte, fromPeerID string) {
	var req protocol.FileRequestPayload
	if protocol.UnmarshalPayload(payload, &req) != nil {
		return
	}
	m.startWorker(func() { m.serveFileRequest(connID, req, fromPeerID) })
}

func (m *Manager) serveFileRequest(connID string, req protocol.FileRequestPayload, peerID string) {
	path, err := m.cachedRoomFile(req.Room, req.FileID, peerID)
	if err != nil {
		m.rejectFile(connID, req.FileID, err)
		return
	}
	hash, size, err := ComputeFileSHA256(path)
	if err != nil || hash != req.FileID {
		m.rejectFile(connID, req.FileID, fmt.Errorf("cached file failed integrity verification"))
		return
	}
	announce, _ := m.announcedFile(req.Room, req.FileID)
	if req.Room == "" {
		announce, _ = m.directFileForPeer(req.FileID, peerID)
	}
	offer := protocol.FileOfferPayload{V: 1, FileID: req.FileID, SHA256: hash, Room: req.Room, Filename: announce.Filename, Size: size, ChunkSize: protocol.ChunkSize, TotalChunks: int((size + protocol.ChunkSize - 1) / protocol.ChunkSize)}
	conn, ok := m.transport.Connection(connID)
	if ok {
		m.sendFileOnConnection(conn, path, offer)
	}
}

func (m *Manager) rejectFile(connID, fileID string, err error) {
	payload, _ := protocol.MarshalPayload(protocol.FileRejectPayload{V: 1, FileID: fileID, Reason: err.Error()})
	_ = m.transport.SendToConn(connID, protocol.TypeFileReject, payload)
}

func (m *Manager) HandleFileWhoHas(payload []byte, fromPeerID string) {
	var req protocol.FileWhoHasPayload
	if protocol.UnmarshalPayload(payload, &req) != nil {
		return
	}
	if _, err := m.cachedRoomFile(req.Room, req.FileID, fromPeerID); err != nil {
		return
	}
	payload, _ = protocol.MarshalPayload(protocol.FileHavePayload{V: 1, Room: req.Room, FileID: req.FileID, PeerID: m.idMgr.PeerID()})
	_ = m.sendPeerFrame(fromPeerID, protocol.TypeFileHave, payload)
}

func (m *Manager) HandleFileHave(payload []byte, fromPeerID string) {
	var have protocol.FileHavePayload
	if protocol.UnmarshalPayload(payload, &have) != nil || have.PeerID != fromPeerID {
		return
	}
	if !m.roomMember(have.Room, fromPeerID) {
		return
	}
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if channel, ok := m.pendingSources[have.FileID]; ok {
		select {
		case channel <- have:
		default:
		}
	}
}
