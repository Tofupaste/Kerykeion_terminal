package main

import (
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

type roomTestPeer struct {
	config        *config.Config
	transfers     *transfer.Manager
	announcements chan protocol.FileAnnouncePayload
	progress      chan transfer.TransferProgress
	id            *identity.Manager
	store         *db.Store
	discovery     *discovery.Discovery
	transport     *transport.Manager
	rooms         *room.Manager
	messages      *messaging.Service
	events        chan string
}

func newRoomTestPeer(t *testing.T, name string) *roomTestPeer {
	t.Helper()
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.LoadOrCreate(cfg.ConfigDir, name)
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(cfg.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	trans, err := transport.New(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	disc := discovery.New(id, 0, "")
	p := &roomTestPeer{id: id, store: store, transport: trans, discovery: disc, events: make(chan string, 100), config: cfg, announcements: make(chan protocol.FileAnnouncePayload, 100), progress: make(chan transfer.TransferProgress, 1000)}
	p.messages = messaging.New(id, store, trans, disc, nil, nil)
	p.rooms = room.New(id, store, disc, trans, func(event, name, detail string) { p.events <- event })
	p.transfers = transfer.New(cfg, id, store, disc, trans, func(progress transfer.TransferProgress) { p.progress <- progress }, func(announcement protocol.FileAnnouncePayload) { p.announcements <- announcement })
	p.transfers.SetMessaging(p.messages)
	p.transfers.Start()
	dispatcher := frameDispatcher{messages: p.messages, rooms: p.rooms, transfers: p.transfers}
	trans.SetFrameHandler(dispatcher.handle)
	if err := trans.Start(); err != nil {
		t.Fatal(err)
	}
	cfg.TCPPort = trans.Port()
	callbacks := peerCallbacks{config: cfg, store: store, messaging: p.messages, rooms: p.rooms, events: &uiEvents{}}
	disc.SetCallbacks(callbacks.discovered, callbacks.updated, callbacks.roomQuery, callbacks.roomHere)
	if err := disc.Start(trans.Port()); err != nil {
		trans.Stop()
		t.Fatal(err)
	}
	t.Cleanup(func() { p.transfers.Stop(); disc.Stop(); trans.Stop(); p.rooms.Stop() })
	return p
}

func connectRoomTestPeers(t *testing.T, peers ...*roomTestPeer) {
	t.Helper()
	for _, from := range peers {
		for _, to := range peers {
			if from != to {
				from.discovery.SendAnnounceTo(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: to.discovery.Port()}, from.transport.Port())
			}
		}
	}
	awaitRoomCondition(t, "peer discovery", func() bool {
		for _, from := range peers {
			if len(from.discovery.GetPeers()) != len(peers)-1 {
				return false
			}
		}
		return true
	})
}

func awaitRoomCondition(t *testing.T, label string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

func awaitRoomMembers(t *testing.T, p *roomTestPeer, count int) {
	t.Helper()
	awaitRoomCondition(t, "room membership", func() bool {
		members, err := p.store.GetRoomMembers("room-e2e")
		return err == nil && len(members) == count
	})
}

func awaitRoomMessage(t *testing.T, p *roomTestPeer, id string) {
	t.Helper()
	awaitRoomCondition(t, "room message", func() bool {
		message, err := p.store.GetMessage(id)
		return err == nil && message != nil && message.ConversationID == "room-e2e"
	})
}

func TestE2ERoomJoinChatAndHistory(t *testing.T) {
	alice := newRoomTestPeer(t, "Alice")
	bob := newRoomTestPeer(t, "Bob")
	carol := newRoomTestPeer(t, "Carol")
	connectRoomTestPeers(t, alice, bob, carol)
	created, err := alice.rooms.CreateRoom("room-e2e")
	if err != nil {
		t.Fatal(err)
	}
	historical, err := alice.messages.SendRoomMessage("room-e2e", "Before Bob joined", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bob.rooms.CreateRoom("room-e2e"); err != nil {
		t.Fatal(err)
	}
	joined, found, err := bob.rooms.JoinOrCreateRoom("room-e2e", bob.transport.Port())
	if err != nil || !found || joined == nil {
		t.Fatalf("join: %v %v", found, err)
	}
	if joined.MasterID != alice.id.PeerID() || joined.Gen != created.Gen || joined.MyRole != "guest" {
		t.Fatal("join created a separate room")
	}
	awaitRoomMembers(t, alice, 2)
	awaitRoomMembers(t, bob, 2)
	awaitRoomMessage(t, bob, historical.MsgID)
	awaitRoomCondition(t, "history event", func() bool {
		for len(bob.events) > 0 {
			if <-bob.events == "ROOM_HISTORY_SYNCED" {
				return true
			}
		}
		return false
	})
	checkThirdRoomPeer(t, alice, bob, carol, historical.MsgID)
}

func checkThirdRoomPeer(t *testing.T, alice, bob, carol *roomTestPeer, historicalID string) {
	t.Helper()
	reply, err := bob.messages.SendRoomMessage("room-e2e", "Bob replies", "")
	if err != nil {
		t.Fatal(err)
	}
	awaitRoomMessage(t, alice, reply.MsgID)
	if _, found, err := carol.rooms.JoinRoom("room-e2e", carol.transport.Port()); err != nil || !found {
		t.Fatalf("Carol join: %v", err)
	}
	broadcast, err := carol.messages.SendRoomMessage("room-e2e", "Hello everyone", "")
	if err != nil {
		t.Fatal(err)
	}
	awaitRoomMessage(t, alice, broadcast.MsgID)
	awaitRoomMessage(t, bob, broadcast.MsgID)
	for _, p := range []*roomTestPeer{alice, bob, carol} {
		awaitRoomMembers(t, p, 3)
	}
	awaitRoomMessage(t, carol, historicalID)
	awaitRoomMessage(t, carol, reply.MsgID)
	final, err := alice.messages.SendRoomMessage("room-e2e", "All three connected", "")
	if err != nil {
		t.Fatal(err)
	}
	awaitRoomMessage(t, bob, final.MsgID)
	awaitRoomMessage(t, carol, final.MsgID)
	members, _ := carol.store.GetRoomMembers("room-e2e")
	if !slices.Contains(members, bob.id.PeerID()) {
		t.Fatal("Carol lacks Bob in metadata")
	}
}

func TestE2ERoomHistoryLargerThanFrame(t *testing.T) {
	alice := newRoomTestPeer(t, "Alice")
	bob := newRoomTestPeer(t, "Bob")
	connectRoomTestPeers(t, alice, bob)
	if _, err := alice.rooms.CreateRoom("room-e2e"); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 16; i++ {
		message, err := alice.messages.SendRoomMessage("room-e2e", strings.Repeat("history ", 10000), "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, message.MsgID)
	}
	if _, found, err := bob.rooms.JoinRoom("room-e2e", bob.transport.Port()); err != nil || !found {
		t.Fatalf("join: %v", err)
	}
	awaitRoomMessage(t, bob, ids[len(ids)-1])
	messages, err := bob.store.GetMessages("room-e2e", 100)
	if err != nil || len(messages) != len(ids) {
		t.Fatalf("history missing: %d, %v", len(messages), err)
	}
	if _, found, err := bob.rooms.JoinRoom("room-e2e", bob.transport.Port()); err != nil || !found {
		t.Fatalf("rejoin: %v", err)
	}
	messages, err = bob.store.GetMessages("room-e2e", 100)
	if err != nil || len(messages) != len(ids) {
		t.Fatal("rejoin duplicated history")
	}
}
