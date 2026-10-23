package messaging

import (
	"context"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transport"
)

var (
	urlRegex = regexp.MustCompile(`https?://[^\s/$.?#].[^\s]*`)
)

type NewMessageCallback func(msg db.MessageRecord, isSelf bool)
type StatusChangeCallback func(msgID, status string)

type deliveryKey struct{ msgID, peerID string }

type Service struct {
	idMgr          *identity.Manager
	store          *db.Store
	transport      *transport.Manager
	discovery      *discovery.Discovery
	onNewMessage   NewMessageCallback
	onStatusChange StatusChangeCallback

	pendingAcksMu sync.Mutex
	pendingAcks   map[deliveryKey]chan bool

	linksMu sync.RWMutex
	links   []string

	ctx       context.Context
	cancelCtx context.CancelFunc
	wg        sync.WaitGroup
}

func New(
	idMgr *identity.Manager,
	store *db.Store,
	trans *transport.Manager,
	disc *discovery.Discovery,
	onNewMessage NewMessageCallback,
	onStatusChange StatusChangeCallback,
) *Service {
	return &Service{
		idMgr:          idMgr,
		store:          store,
		transport:      trans,
		discovery:      disc,
		onNewMessage:   onNewMessage,
		onStatusChange: onStatusChange,
		pendingAcks:    make(map[deliveryKey]chan bool),
	}
}

func (s *Service) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancelCtx = cancel

	s.wg.Add(1)
	go s.outboxWorker()
}

func (s *Service) Stop() {
	if s.cancelCtx != nil {
		s.cancelCtx()
	}
	s.wg.Wait()
}

func (s *Service) GenerateMsgID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

func (s *Service) ExtractLinks(text string) []string {
	matches := urlRegex.FindAllString(text, -1)
	var valid []string
	for _, m := range matches {
		if u, err := url.Parse(m); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			valid = append(valid, m)
		}
	}
	return valid
}

func (s *Service) RegisterLinks(text string) []int {
	newLinks := s.ExtractLinks(text)
	if len(newLinks) == 0 {
		return nil
	}

	s.linksMu.Lock()
	defer s.linksMu.Unlock()

	var indexes []int
	for _, l := range newLinks {
		s.links = append(s.links, l)
		indexes = append(indexes, len(s.links))
	}
	return indexes
}

func (s *Service) GetLink(index int) (string, bool) {
	s.linksMu.RLock()
	defer s.linksMu.RUnlock()

	if index <= 0 || index > len(s.links) {
		return "", false
	}
	return s.links[index-1], true
}

func (s *Service) SendDM(targetPeerID, body string) (*db.MessageRecord, error) {
	msgID := s.GenerateMsgID()
	now := time.Now().Unix()

	pkt := protocol.MsgPayload{
		V:        1,
		MsgID:    msgID,
		AuthorID: s.idMgr.PeerID(),
		Room:     "",
		Body:     body,
		TS:       now,
	}
	pkt.Sig = s.idMgr.SignBase64(pkt.SigBytes())

	rec := db.MessageRecord{
		MsgID:          msgID,
		ConversationID: targetPeerID,
		AuthorID:       s.idMgr.PeerID(),
		Body:           body,
		Timestamp:      now,
		Signature:      pkt.Sig,
		Status:         "queued",
		CreatedAt:      time.Now(),
	}

	_, _ = s.store.SaveMessage(rec)
	s.RegisterLinks(body)

	if s.onNewMessage != nil {
		s.onNewMessage(rec, true)
	}

	go s.deliverWithAck(pkt, targetPeerID)

	return &rec, nil
}

func (s *Service) SendRoomMessage(roomName, body, fileRef string) (*db.MessageRecord, error) {
	msgID := s.GenerateMsgID()
	now := time.Now().Unix()

	pkt := protocol.MsgPayload{
		V:        1,
		MsgID:    msgID,
		AuthorID: s.idMgr.PeerID(),
		Room:     roomName,
		Body:     body,
		TS:       now,
		FileRef:  fileRef,
	}
	pkt.Sig = s.idMgr.SignBase64(pkt.SigBytes())

	rec := db.MessageRecord{
		MsgID:          msgID,
		ConversationID: roomName,
		AuthorID:       s.idMgr.PeerID(),
		Body:           body,
		Timestamp:      now,
		Signature:      pkt.Sig,
		Status:         "sent",
		FileRef:        fileRef,
		CreatedAt:      time.Now(),
	}

	_, _ = s.store.SaveMessage(rec)
	if body != "" {
		s.RegisterLinks(body)
	}

	if s.onNewMessage != nil {
		s.onNewMessage(rec, true)
	}

	members, err := s.store.GetRoomMembers(roomName)
	if err == nil {
		for _, memberID := range members {
			if memberID == s.idMgr.PeerID() {
				continue
			}
			go s.deliverWithAck(pkt, memberID)
		}
	}

	return &rec, nil
}

func (s *Service) deliverWithAck(pkt protocol.MsgPayload, targetPeerID string) {
	payloadBytes, err := protocol.MarshalPayload(pkt)
	if err != nil {
		return
	}

	ackCh := make(chan bool, 1)
	key := deliveryKey{pkt.MsgID, targetPeerID}
	s.pendingAcksMu.Lock()
	s.pendingAcks[key] = ackCh
	s.pendingAcksMu.Unlock()

	defer func() {
		s.pendingAcksMu.Lock()
		delete(s.pendingAcks, key)
		s.pendingAcksMu.Unlock()
	}()

	if s.sendPayload(targetPeerID, payloadBytes) {
		s.updateStatus(pkt.MsgID, "sent")
		select {
		case <-ackCh:
			s.updateStatus(pkt.MsgID, "delivered")
			_ = s.store.RemoveFromOutbox(pkt.MsgID, targetPeerID)
			return
		case <-time.After(3 * time.Second):
		}
	}

	_ = s.store.AddToOutbox(db.OutboxItem{
		MsgID:        pkt.MsgID,
		TargetPeerID: targetPeerID,
		Attempts:     1,
		NextRetryAt:  time.Now().Add(1 * time.Second),
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
	})
}

func (s *Service) sendPayload(peerID string, payload []byte) bool {
	if s.transport.IsConnected(peerID) && s.transport.SendToPeer(peerID, protocol.TypeMsg, payload) == nil {
		return true
	}
	ip, port := s.peerAddress(peerID)
	if ip == "" || port == 0 {
		return false
	}
	conn, err := s.transport.DialPeer(peerID, ip, port, "chat")
	return err == nil && conn.WriteFrame(protocol.TypeMsg, payload) == nil
}

func (s *Service) peerAddress(peerID string) (string, int) {
	if peer, ok := s.discovery.GetPeer(peerID); ok && peer.IsOnline {
		return peer.IP, peer.Port
	}
	if peer, err := s.store.GetPeer(peerID); err == nil && peer != nil {
		return peer.LastIP, peer.LastPort
	}
	return "", 0
}

func (s *Service) updateStatus(msgID, status string) {
	_ = s.store.UpdateMessageStatus(msgID, status)
	if s.onStatusChange != nil {
		s.onStatusChange(msgID, status)
	}
}

func (s *Service) HandleIncomingMsg(payload []byte, fromPeerID string) {
	var pkt protocol.MsgPayload
	if err := protocol.UnmarshalPayload(payload, &pkt); err != nil {
		return
	}

	var pubKey string
	if peerInfo, ok := s.discovery.GetPeer(pkt.AuthorID); ok {
		pubKey = peerInfo.PubKey
	} else if pRec, err := s.store.GetPeer(pkt.AuthorID); err == nil && pRec != nil {
		pubKey = pRec.PubKey
	}

	if pubKey == "" || !identity.VerifyBase64(pubKey, pkt.SigBytes(), pkt.Sig) {
		return
	}

	// Send ACK immediately
	ackPayload, _ := protocol.MarshalPayload(protocol.MsgAckPayload{
		V:     1,
		MsgID: pkt.MsgID,
	})
	ackTarget := fromPeerID
	if ackTarget == "" {
		ackTarget = pkt.AuthorID
	}
	_ = s.transport.SendToPeer(ackTarget, protocol.TypeMsgAck, ackPayload)

	convID := pkt.Room
	if convID == "" {
		convID = pkt.AuthorID
	}

	rec := db.MessageRecord{
		MsgID:          pkt.MsgID,
		ConversationID: convID,
		AuthorID:       pkt.AuthorID,
		Body:           pkt.Body,
		Timestamp:      pkt.TS,
		Signature:      pkt.Sig,
		Status:         "delivered",
		FileRef:        pkt.FileRef,
		CreatedAt:      time.Now(),
	}

	inserted, _ := s.store.SaveMessage(rec)
	if inserted {
		if pkt.Body != "" {
			s.RegisterLinks(pkt.Body)
		}
		if s.onNewMessage != nil {
			s.onNewMessage(rec, false)
		}
	}
}

func (s *Service) HandleIncomingMsgAck(payload []byte, fromPeerID string) {
	var ack protocol.MsgAckPayload
	if err := protocol.UnmarshalPayload(payload, &ack); err != nil {
		return
	}

	s.pendingAcksMu.Lock()
	if ch, ok := s.pendingAcks[deliveryKey{ack.MsgID, fromPeerID}]; ok {
		select {
		case ch <- true:
		default:
		}
	}
	s.pendingAcksMu.Unlock()

	s.updateStatus(ack.MsgID, "delivered")
}

func (s *Service) TriggerPeerRetry(peerID string) {
	go s.retryOutboxForPeer(peerID)
}

func (s *Service) retryOutboxForPeer(peerID string) {
	items, err := s.store.GetPendingOutboxItems(time.Now().Add(24 * time.Hour))
	if err != nil {
		return
	}

	for _, item := range items {
		if item.TargetPeerID != peerID {
			continue
		}
		msg, err := s.store.GetMessage(item.MsgID)
		if err != nil || msg == nil {
			continue
		}

		convID := ""
		if msg.ConversationID != peerID {
			convID = msg.ConversationID
		}

		pkt := protocol.MsgPayload{
			V:        1,
			MsgID:    msg.MsgID,
			AuthorID: msg.AuthorID,
			Room:     convID,
			Body:     msg.Body,
			TS:       msg.Timestamp,
			Sig:      msg.Signature,
			FileRef:  msg.FileRef,
		}

		s.deliverWithAck(pkt, peerID)
	}
}

func (s *Service) outboxWorker() {
	defer s.wg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processOutbox()
		}
	}
}

func (s *Service) processOutbox() {
	items, err := s.store.GetPendingOutboxItems(time.Now())
	if err != nil {
		return
	}

	for _, it := range items {
		peerInfo, ok := s.discovery.GetPeer(it.TargetPeerID)
		if !ok || !peerInfo.IsOnline {
			continue
		}

		msg, err := s.store.GetMessage(it.MsgID)
		if err != nil || msg == nil {
			_ = s.store.RemoveFromOutbox(it.MsgID, it.TargetPeerID)
			continue
		}

		convID := ""
		if msg.ConversationID != it.TargetPeerID {
			convID = msg.ConversationID
		}

		pkt := protocol.MsgPayload{
			V:        1,
			MsgID:    msg.MsgID,
			AuthorID: msg.AuthorID,
			Room:     convID,
			Body:     msg.Body,
			TS:       msg.Timestamp,
			Sig:      msg.Signature,
			FileRef:  msg.FileRef,
		}

		go func(item db.OutboxItem, packet protocol.MsgPayload) {
			backoffSecs := 1 << item.Attempts
			if backoffSecs > 15 {
				backoffSecs = 15
			}
			item.Attempts++
			item.NextRetryAt = time.Now().Add(time.Duration(backoffSecs) * time.Second)
			_ = s.store.AddToOutbox(item)

			s.deliverWithAck(packet, item.TargetPeerID)
		}(it, pkt)
	}
}
