package transfer

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"kerykeion/pkg/db"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
)

func (m *Manager) PullFile(reference string) error {
	number, id, err := ParseFileReference(reference)
	if err != nil {
		return err
	}
	if id == "" {
		return m.PullAnnouncedFile(number)
	}
	reference = id
	announce, ok := m.announcementByID(reference)
	if !ok {
		return fmt.Errorf("file ID was not found in announcements or attachment history")
	}
	if !m.beginWork() {
		return m.ctx.Err()
	}
	defer m.wg.Done()
	return m.pullAnnouncement(announce)
}

func ParseFileReference(reference string) (int, string, error) {
	reference = strings.TrimSpace(reference)
	if validFileID(reference) {
		return 0, reference, nil
	}
	number, err := strconv.Atoi(strings.TrimPrefix(reference, "#"))
	if err != nil || number < 1 {
		return 0, "", fmt.Errorf("use /get <announcement number or full file ID>")
	}
	return number, "", nil
}

func (m *Manager) announcementByID(id string) (protocol.FileAnnouncePayload, bool) {
	m.announcementsMu.RLock()
	for _, announce := range m.announcements {
		if announce.FileID == id {
			m.announcementsMu.RUnlock()
			return announce, true
		}
	}
	m.announcementsMu.RUnlock()
	messages, _ := m.store.GetFileMessages(id)
	for _, message := range messages {
		announce, ok := attachmentAnnouncement(message)
		if !ok {
			continue
		}
		if room, _ := m.store.GetRoom(message.ConversationID); room != nil {
			if room.State != "active" {
				continue
			}
			announce.Room = room.Name
		}
		return announce, true
	}
	return protocol.FileAnnouncePayload{}, false
}

func attachmentAnnouncement(message db.MessageRecord) (protocol.FileAnnouncePayload, bool) {
	var file messaging.FileAttachment
	if json.Unmarshal([]byte(message.Body), &file) != nil || file.Filename == "" || file.Size < 0 {
		return protocol.FileAnnouncePayload{}, false
	}
	return protocol.FileAnnouncePayload{V: 1, FileID: message.FileRef, Filename: file.Filename, Size: file.Size, SenderID: message.AuthorID}, true
}

func (m *Manager) directFileForPeer(id, peerID string) (protocol.FileAnnouncePayload, bool) {
	messages, _ := m.store.GetFileMessages(id)
	for _, message := range messages {
		if message.AuthorID != m.idMgr.PeerID() {
			continue
		}
		if message.ConversationID != peerID && peerID != m.idMgr.PeerID() {
			continue
		}
		if room, _ := m.store.GetRoom(message.ConversationID); room != nil {
			continue
		}
		if announce, ok := attachmentAnnouncement(message); ok {
			return announce, true
		}
	}
	return protocol.FileAnnouncePayload{}, false
}

func (m *Manager) existingDownload(announce protocol.FileAnnouncePayload) bool {
	record, err := m.store.GetTransfer(announce.FileID, announce.SenderID, "download")
	if err != nil || record == nil || record.State != "completed" {
		return false
	}
	hash, _, err := ComputeFileSHA256(record.LocalPath)
	if err != nil || hash != announce.FileID {
		return false
	}
	m.reportProgress(TransferProgress{FileID: announce.FileID, Filename: record.Filename, LocalPath: record.LocalPath, TotalBytes: record.Size, SentBytes: record.Size, IsDownload: true, IsCompleted: true})
	return true
}
