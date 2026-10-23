package room

import (
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
)

func (m *Manager) saveHistoryMessage(room string, msg protocol.MsgPayload, providedKey string) bool {
	if msg.Room != room || msg.MsgID == "" {
		return false
	}
	key := m.peerPublicKey(msg.AuthorID)
	if key == "" {
		key = providedKey
	}
	decoded, err := identity.DecodeBase64PubKey(key)
	if err != nil || identity.Fingerprint(decoded) != msg.AuthorID {
		return false
	}
	if !identity.VerifyBase64(key, msg.SigBytes(), msg.Sig) {
		return false
	}
	if m.peerPublicKey(msg.AuthorID) == "" {
		_ = m.store.UpsertPeer(db.PeerRecord{PeerID: msg.AuthorID, PubKey: key, DisplayName: msg.AuthorID, LastSeen: time.Unix(msg.TS, 0)})
	}
	inserted, err := m.store.SaveMessage(db.MessageRecord{
		MsgID: msg.MsgID, ConversationID: room, AuthorID: msg.AuthorID, Body: msg.Body,
		Timestamp: msg.TS, Signature: msg.Sig, Status: "delivered", FileRef: msg.FileRef, CreatedAt: time.Now(),
	})
	return err == nil && inserted
}

func (m *Manager) sendHistoryResponses(room, target string, messages []protocol.MsgPayload, keys map[string]string) {
	batch := protocol.RoomSyncResPayload{V: 1, Room: room, AuthorKeys: keys}
	for _, message := range messages {
		batch.Messages = append(batch.Messages, message)
		payload, err := protocol.MarshalPayload(batch)
		if err != nil {
			return
		}
		if len(payload) <= protocol.MaxFrameLength {
			continue
		}
		batch.Messages = batch.Messages[:len(batch.Messages)-1]
		m.sendHistoryResponse(target, batch)
		batch.Messages = []protocol.MsgPayload{message}
	}
	m.sendHistoryResponse(target, batch)
}

func (m *Manager) sendHistoryResponse(target string, batch protocol.RoomSyncResPayload) {
	if len(batch.Messages) == 0 {
		return
	}
	payload, err := protocol.MarshalPayload(batch)
	if err == nil {
		_ = m.sendRoomFrame(target, protocol.TypeRoomSyncRes, payload)
	}
}
