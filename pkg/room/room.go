package room

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transport"
)

var (
	roomNameRegex = regexp.MustCompile(`^[a-z0-9\-_]{2,32}$`)
)

type EventCallback func(eventName, roomName, detail string)

type Manager struct {
	idMgr     *identity.Manager
	store     *db.Store
	discovery *discovery.Discovery
	transport *transport.Manager
	onEvent   EventCallback

	pendingQueriesMu sync.Mutex
	pendingQueries   map[string]chan protocol.RoomHerePacket
	pendingMembersMu sync.Mutex
	pendingMembers   map[string]membershipWait

	ctx       context.Context
	cancelCtx context.CancelFunc
}

func New(
	idMgr *identity.Manager,
	store *db.Store,
	disc *discovery.Discovery,
	trans *transport.Manager,
	onEvent EventCallback,
) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		idMgr:          idMgr,
		store:          store,
		discovery:      disc,
		transport:      trans,
		onEvent:        onEvent,
		pendingQueries: make(map[string]chan protocol.RoomHerePacket),
		pendingMembers: make(map[string]membershipWait),
		ctx:            ctx,
		cancelCtx:      cancel,
	}
}

func (m *Manager) Stop() {
	if m.cancelCtx != nil {
		m.cancelCtx()
	}
}

func NormalizeName(name string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(name))
	norm = strings.TrimPrefix(norm, "#")
	if !roomNameRegex.MatchString(norm) {
		return "", errors.New("nama room harus 2-32 karakter, hanya huruf kecil, angka, '-' atau '_'")
	}
	return norm, nil
}

func (m *Manager) CreateRoom(name string) (*db.RoomRecord, error) {
	norm, err := NormalizeName(name)
	if err != nil {
		return nil, err
	}

	rooms, err := m.store.GetAllRooms()
	if err == nil && len(rooms) >= 20 {
		return nil, errors.New("batas maksimal 20 room per pengguna telah tercapai")
	}

	gen := time.Now().Unix()
	rec := db.RoomRecord{
		Name:         norm,
		Gen:          gen,
		MasterID:     m.idMgr.PeerID(),
		MyRole:       "master",
		CreatedAt:    time.Now(),
		LastActivity: time.Now(),
		State:        "active",
	}

	if err := m.store.UpsertRoom(rec); err != nil {
		return nil, err
	}
	_ = m.store.AddRoomMember(norm, m.idMgr.PeerID())

	if m.onEvent != nil {
		m.onEvent("ROOM_CREATED", norm, "Kamu membuat room ini sebagai master")
	}

	return &rec, nil
}

func (m *Manager) JoinRoom(name string, myTCPPort int) (*db.RoomRecord, bool, error) {
	norm, err := NormalizeName(name)
	if err != nil {
		return nil, false, err
	}

	existing, _ := m.store.GetRoom(norm)

	hereCh := make(chan protocol.RoomHerePacket, 5)
	m.pendingQueriesMu.Lock()
	m.pendingQueries[norm] = hereCh
	m.pendingQueriesMu.Unlock()

	defer func() {
		m.pendingQueriesMu.Lock()
		delete(m.pendingQueries, norm)
		m.pendingQueriesMu.Unlock()
	}()

	var foundResp *protocol.RoomHerePacket
	for attempt := 0; attempt < 2; attempt++ {
		m.discovery.BroadcastRoomQuery(norm, myTCPPort)
		select {
		case resp := <-hereCh:
			foundResp = &resp
			break
		case <-time.After(800 * time.Millisecond):
		}
		if foundResp != nil {
			break
		}
	}

	if foundResp == nil {
		if existing != nil && existing.State == "active" {
			go m.reconnectRoom(existing)
			return existing, true, nil
		}
		return nil, false, nil
	}

	if m.store.HasTombstone(norm, foundResp.Gen) {
		return nil, false, errors.New("room ini sudah pernah dibubarkan oleh master")
	}

	rec := db.RoomRecord{
		Name:         norm,
		Gen:          foundResp.Gen,
		MasterID:     foundResp.MasterID,
		MyRole:       "guest",
		CreatedAt:    time.Unix(foundResp.Gen, 0),
		LastActivity: time.Now(),
		State:        "active",
	}
	if rec.MasterID == m.idMgr.PeerID() {
		rec.MyRole = "master"
	}
	if err := m.store.UpsertRoom(rec); err != nil {
		return nil, false, err
	}
	_ = m.store.AddRoomMember(norm, m.idMgr.PeerID())
	_ = m.store.AddRoomMember(norm, foundResp.FromPeer)

	if err := m.connectRoom(norm, foundResp.Gen, foundResp.FromPeer); err != nil {
		return nil, false, err
	}

	if m.onEvent != nil {
		m.onEvent("ROOM_JOINED", norm, fmt.Sprintf("Berhasil bergabung ke room %s", norm))
	}

	return &rec, true, nil
}

// JoinOrCreateRoom tries to join an existing room on the LAN. If no peer hosts it,
// it automatically creates the room as master. The boolean return is true if joined existing, false if newly created.
func (m *Manager) JoinOrCreateRoom(name string, myTCPPort int) (*db.RoomRecord, bool, error) {
	norm, err := NormalizeName(name)
	if err != nil {
		return nil, false, err
	}

	rec, joined, err := m.JoinRoom(norm, myTCPPort)
	if err != nil {
		return nil, false, err
	}
	if joined && rec != nil {
		return rec, true, nil
	}

	newRec, err := m.CreateRoom(norm)
	if err != nil {
		return nil, false, err
	}
	return newRec, false, nil
}

func (m *Manager) broadcastJoinEvent(roomName string, gen int64) {
	now := time.Now().Unix()
	joinPkt := protocol.RoomJoinPayload{
		V:      1,
		Name:   roomName,
		Gen:    gen,
		PeerID: m.idMgr.PeerID(),
		TS:     now,
	}
	joinPkt.Sig = m.idMgr.SignBase64(joinPkt.SigBytes())

	payload, err := protocol.MarshalPayload(joinPkt)
	if err != nil {
		return
	}

	members, _ := m.store.GetRoomMembers(roomName)
	for _, memberID := range members {
		if memberID != m.idMgr.PeerID() {
			_ = m.sendRoomFrame(memberID, protocol.TypeRoomJoin, payload)
		}
	}
}

func (m *Manager) LeaveRoom(name string) error {
	norm, err := NormalizeName(name)
	if err != nil {
		return err
	}

	rec, err := m.store.GetRoom(norm)
	if err != nil || rec == nil {
		return errors.New("kamu tidak berada di room ini")
	}

	now := time.Now().Unix()
	leavePkt := protocol.RoomLeavePayload{
		V:      1,
		Name:   norm,
		Gen:    rec.Gen,
		PeerID: m.idMgr.PeerID(),
		TS:     now,
	}
	leavePkt.Sig = m.idMgr.SignBase64(leavePkt.SigBytes())

	payload, _ := protocol.MarshalPayload(leavePkt)
	members, _ := m.store.GetRoomMembers(norm)
	for _, memberID := range members {
		if memberID == m.idMgr.PeerID() {
			continue
		}
		if peerInfo, ok := m.discovery.GetPeer(memberID); ok && peerInfo.IsOnline {
			_ = m.transport.SendToPeer(memberID, protocol.TypeRoomLeave, payload)
		}
	}

	rec.State = "left"
	_ = m.store.UpsertRoom(*rec)
	_ = m.store.RemoveRoomMember(norm, m.idMgr.PeerID())

	if m.onEvent != nil {
		m.onEvent("ROOM_LEFT", norm, "Kamu telah keluar dari room")
	}
	return nil
}

func (m *Manager) DisbandRoom(name string) error {
	norm, err := NormalizeName(name)
	if err != nil {
		return err
	}

	rec, err := m.store.GetRoom(norm)
	if err != nil || rec == nil {
		return errors.New("room tidak ditemukan")
	}

	if rec.MasterID != m.idMgr.PeerID() {
		return errors.New("hanya master room yang dapat membubarkan room ini")
	}

	now := time.Now().Unix()
	closedPkt := protocol.RoomClosedPayload{
		V:        1,
		Name:     norm,
		Gen:      rec.Gen,
		MasterID: m.idMgr.PeerID(),
		TS:       now,
	}
	closedPkt.Sig = m.idMgr.SignBase64(closedPkt.SigBytes())

	payload, _ := protocol.MarshalPayload(closedPkt)
	members, _ := m.store.GetRoomMembers(norm)
	for _, memberID := range members {
		if memberID == m.idMgr.PeerID() {
			continue
		}
		_ = m.transport.SendToPeer(memberID, protocol.TypeRoomClosed, payload)
	}

	_ = m.store.AddTombstone(norm, rec.Gen)
	rec.State = "disbanded"
	_ = m.store.UpsertRoom(*rec)

	if m.onEvent != nil {
		m.onEvent("ROOM_DISBANDED", norm, "Room telah dibubarkan oleh master")
	}
	return nil
}

func (m *Manager) TransferMaster(name, newMasterPeerID string) error {
	norm, err := NormalizeName(name)
	if err != nil {
		return err
	}

	rec, err := m.store.GetRoom(norm)
	if err != nil || rec == nil {
		return errors.New("room tidak ditemukan")
	}

	if rec.MasterID != m.idMgr.PeerID() {
		return errors.New("hanya master yang dapat mengalihkan kepemilikan room")
	}

	now := time.Now().Unix()
	transferPkt := protocol.RoomTransferPayload{
		V:         1,
		Name:      norm,
		Gen:       rec.Gen,
		OldMaster: m.idMgr.PeerID(),
		NewMaster: newMasterPeerID,
		TS:        now,
	}
	transferPkt.Sig = m.idMgr.SignBase64(transferPkt.SigBytes())

	payload, _ := protocol.MarshalPayload(transferPkt)
	members, _ := m.store.GetRoomMembers(norm)
	for _, memberID := range members {
		if memberID == m.idMgr.PeerID() {
			continue
		}
		_ = m.transport.SendToPeer(memberID, protocol.TypeRoomTransfer, payload)
	}

	rec.MasterID = newMasterPeerID
	rec.MyRole = "guest"
	_ = m.store.UpsertRoom(*rec)

	if m.onEvent != nil {
		m.onEvent("ROOM_TRANSFERRED", norm, fmt.Sprintf("Master dialihkan ke %s", newMasterPeerID))
	}
	return nil
}

func (m *Manager) HandleRoomQuery(q protocol.RoomQueryPacket, source *net.UDPAddr, myTCPPort int) {
	norm, err := NormalizeName(q.RoomName)
	if err != nil {
		return
	}

	rec, err := m.store.GetRoom(norm)
	if err == nil && rec != nil && rec.State == "active" {
		if source != nil && q.FromPeer != m.idMgr.PeerID() {
			m.discovery.SendRoomHere(source, norm, rec.Gen, rec.MasterID, myTCPPort)
		}
	}
}

func (m *Manager) HandleRoomHere(h protocol.RoomHerePacket) {
	m.pendingQueriesMu.Lock()
	ch, ok := m.pendingQueries[h.RoomName]
	m.pendingQueriesMu.Unlock()

	if ok {
		select {
		case ch <- h:
		default:
		}
	}
}

func (m *Manager) HandleRoomJoin(payload []byte) {
	var pkt protocol.RoomJoinPayload
	if err := protocol.UnmarshalPayload(payload, &pkt); err != nil {
		return
	}

	if !identity.VerifyBase64(m.peerPublicKey(pkt.PeerID), pkt.SigBytes(), pkt.Sig) {
		return
	}

	rec, err := m.store.GetRoom(pkt.Name)
	if err != nil || rec == nil || rec.State != "active" || rec.Gen != pkt.Gen {
		return
	}
	if err := m.store.AddRoomMember(pkt.Name, pkt.PeerID); err != nil {
		return
	}
	m.broadcastRoomMeta(rec)

	if m.onEvent != nil {
		name := pkt.PeerID
		if peer, ok := m.discovery.GetPeer(pkt.PeerID); ok {
			name = peer.DisplayName
		}
		m.onEvent("MEMBER_JOINED", pkt.Name, fmt.Sprintf("%s bergabung ke room", name))
	}
}

func (m *Manager) HandleRoomLeave(payload []byte) {
	var pkt protocol.RoomLeavePayload
	if err := protocol.UnmarshalPayload(payload, &pkt); err != nil {
		return
	}

	peerInfo, ok := m.discovery.GetPeer(pkt.PeerID)
	if !ok {
		return
	}
	if !identity.VerifyBase64(peerInfo.PubKey, pkt.SigBytes(), pkt.Sig) {
		return
	}

	_ = m.store.RemoveRoomMember(pkt.Name, pkt.PeerID)

	if m.onEvent != nil {
		m.onEvent("MEMBER_LEFT", pkt.Name, fmt.Sprintf("%s keluar dari room", peerInfo.DisplayName))
	}
}

func (m *Manager) HandleRoomClosed(payload []byte) {
	var pkt protocol.RoomClosedPayload
	if err := protocol.UnmarshalPayload(payload, &pkt); err != nil {
		return
	}

	peerInfo, ok := m.discovery.GetPeer(pkt.MasterID)
	if !ok {
		return
	}
	if !identity.VerifyBase64(peerInfo.PubKey, pkt.SigBytes(), pkt.Sig) {
		return
	}

	rec, err := m.store.GetRoom(pkt.Name)
	if err != nil || rec == nil || rec.Gen != pkt.Gen || rec.MasterID != pkt.MasterID {
		return
	}
	_ = m.store.AddTombstone(pkt.Name, pkt.Gen)
	rec.State = "disbanded"
	_ = m.store.UpsertRoom(*rec)

	members, _ := m.store.GetRoomMembers(pkt.Name)
	for _, mID := range members {
		if mID != m.idMgr.PeerID() && mID != pkt.MasterID {
			_ = m.transport.SendToPeer(mID, protocol.TypeRoomClosed, payload)
		}
	}

	if m.onEvent != nil {
		m.onEvent("ROOM_DISBANDED", pkt.Name, "Room dibubarkan oleh master")
	}
}

func (m *Manager) HandleRoomTransfer(payload []byte) {
	var pkt protocol.RoomTransferPayload
	if err := protocol.UnmarshalPayload(payload, &pkt); err != nil {
		return
	}

	peerInfo, ok := m.discovery.GetPeer(pkt.OldMaster)
	if !ok {
		return
	}
	if !identity.VerifyBase64(peerInfo.PubKey, pkt.SigBytes(), pkt.Sig) {
		return
	}

	rec, err := m.store.GetRoom(pkt.Name)
	if err == nil && rec != nil && rec.Gen == pkt.Gen && rec.MasterID == pkt.OldMaster {
		rec.MasterID = pkt.NewMaster
		if pkt.NewMaster == m.idMgr.PeerID() {
			rec.MyRole = "master"
		} else {
			rec.MyRole = "guest"
		}
		_ = m.store.UpsertRoom(*rec)

		if m.onEvent != nil {
			m.onEvent("ROOM_TRANSFERRED", pkt.Name, fmt.Sprintf("Master room kini beralih ke %s", pkt.NewMaster))
		}
	}
}

func (m *Manager) syncRoomHistory(roomName, targetPeerID string) {
	recentIDs, _ := m.store.GetRecentMessageIDs(roomName, time.Now().AddDate(0, 0, -7).Unix())
	reqPayload, _ := protocol.MarshalPayload(protocol.RoomSyncReqPayload{
		V:            1,
		Room:         roomName,
		KnownMsgIDs:  recentIDs,
		SinceSeconds: time.Now().AddDate(0, 0, -7).Unix(),
	})
	_ = m.sendRoomFrame(targetPeerID, protocol.TypeRoomSyncReq, reqPayload)
}

func (m *Manager) HandleRoomSyncReq(payload []byte, fromPeerID string) {
	var req protocol.RoomSyncReqPayload
	if err := protocol.UnmarshalPayload(payload, &req); err != nil {
		return
	}

	rec, err := m.store.GetRoom(req.Room)
	if err != nil || rec == nil || rec.State != "active" || !m.roomMember(req.Room, fromPeerID) {
		return
	}

	msgs, err := m.store.GetMessages(req.Room, 500)
	if err != nil {
		return
	}

	knownMap := make(map[string]bool)
	for _, id := range req.KnownMsgIDs {
		knownMap[id] = true
	}

	var missingMsgs []protocol.MsgPayload
	authorKeys := make(map[string]string)
	for _, message := range msgs {
		if !knownMap[message.MsgID] && message.Timestamp >= req.SinceSeconds {
			authorKeys[message.AuthorID] = m.peerPublicKey(message.AuthorID)
			missingMsgs = append(missingMsgs, protocol.MsgPayload{
				V:        1,
				MsgID:    message.MsgID,
				AuthorID: message.AuthorID,
				Room:     message.ConversationID,
				Body:     message.Body,
				TS:       message.Timestamp,
				Sig:      message.Signature,
				FileRef:  message.FileRef,
			})
		}
	}

	m.sendHistoryResponses(req.Room, fromPeerID, missingMsgs, authorKeys)
}

func (m *Manager) HandleRoomSyncRes(payload []byte, fromPeerID string) {
	var res protocol.RoomSyncResPayload
	if protocol.UnmarshalPayload(payload, &res) != nil {
		return
	}
	rec, err := m.store.GetRoom(res.Room)
	if err != nil || rec == nil || rec.State != "active" || !m.roomMember(res.Room, fromPeerID) {
		return
	}
	inserted := 0
	for _, msg := range res.Messages {
		if m.saveHistoryMessage(res.Room, msg, res.AuthorKeys[msg.AuthorID]) {
			inserted++
		}
	}
	if inserted > 0 && m.onEvent != nil {
		m.onEvent("ROOM_HISTORY_SYNCED", res.Room, fmt.Sprintf("%d pesan riwayat dimuat", inserted))
	}
}
