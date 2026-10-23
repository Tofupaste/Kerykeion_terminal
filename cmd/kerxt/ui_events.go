package main

import (
	"net"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/ui"
)

type uiEvents struct {
	mu      sync.RWMutex
	program *tea.Program
	repl    *ui.REPL
}

func (e *uiEvents) setTUI(program *tea.Program) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.program = program
}

func (e *uiEvents) setREPL(repl *ui.REPL) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.repl = repl
}

func (e *uiEvents) targets() (*tea.Program, *ui.REPL) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.program, e.repl
}

func (e *uiEvents) send(message tea.Msg) {
	program, _ := e.targets()
	if program != nil {
		program.Send(message)
	}
}

func (e *uiEvents) onMessage(message db.MessageRecord, self bool) {
	program, repl := e.targets()
	if program != nil {
		program.Send(ui.MsgNewMessage(message))
	}
	if repl != nil {
		repl.OnNewMessage(message, self)
	}
}

func (e *uiEvents) onStatus(id, status string) {
	e.send(ui.MsgStatusChange{MsgID: id, Status: status})
}

func (e *uiEvents) onProgress(progress transfer.TransferProgress) {
	e.send(ui.MsgProgress(progress))
	_, repl := e.targets()
	if repl != nil {
		repl.OnTransferProgress(progress)
	}
}
func (e *uiEvents) onAnnounce(announcement protocol.FileAnnouncePayload) {
	e.send(ui.MsgAnnounce(announcement))
	_, repl := e.targets()
	if repl != nil {
		repl.OnFileAnnounce(announcement)
	}
}

func (e *uiEvents) onRoomEvent(event, name, detail string) {
	program, repl := e.targets()
	if program != nil {
		program.Send(ui.MsgRoomEvent{Event: event, Room: name, Detail: detail})
	}
	if repl != nil {
		repl.Printf("[%s] #%s: %s\n", event, name, detail)
	}
}

type peerCallbacks struct {
	config    *config.Config
	store     *db.Store
	messaging *messaging.Service
	rooms     *room.Manager
	events    *uiEvents
}

func (p peerCallbacks) save(peer discovery.PeerInfo) {
	_ = p.store.UpsertPeer(db.PeerRecord{
		PeerID: peer.PeerID, DisplayName: peer.DisplayName, PubKey: peer.PubKey,
		LastIP: peer.IP, LastPort: peer.Port, LastSeen: peer.LastSeen, Trusted: true,
	})
	p.messaging.TriggerPeerRetry(peer.PeerID)
}

func (p peerCallbacks) discovered(peer discovery.PeerInfo) {
	p.save(peer)
	p.events.send(ui.MsgPeerDiscovered(peer))
}

func (p peerCallbacks) updated(peer discovery.PeerInfo) {
	p.save(peer)
	p.events.send(ui.MsgPeerUpdated(peer))
}

func (p peerCallbacks) roomQuery(query protocol.RoomQueryPacket, addr *net.UDPAddr) {
	p.rooms.HandleRoomQuery(query, addr, p.config.TCPPort)
}

func (p peerCallbacks) roomHere(here protocol.RoomHerePacket, _ *net.UDPAddr) {
	p.rooms.HandleRoomHere(here)
}
