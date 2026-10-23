package transfer

import (
	"fmt"
	"io"
	"os"
	"time"

	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transport"
)

func connectionID(conn *transport.ConnWrapper) string { return fmt.Sprintf("%p", conn) }

func (m *Manager) beginWork() bool {
	m.workerMu.Lock()
	defer m.workerMu.Unlock()
	if m.stopped {
		return false
	}
	m.wg.Add(1)
	return true
}

func (m *Manager) startWorker(work func()) bool {
	if !m.beginWork() {
		return false
	}
	go func() { defer m.wg.Done(); work() }()
	return true
}

func (m *Manager) dialFilePeer(peerID string) (*transport.ConnWrapper, error) {
	if peerID == m.idMgr.PeerID() {
		return m.transport.DialPeer(peerID, "127.0.0.1", m.transport.Port(), "file")
	}
	if peer, ok := m.discovery.GetPeer(peerID); ok && peer.IsOnline {
		return m.transport.DialPeer(peerID, peer.IP, peer.Port, "file")
	}
	if peer, err := m.store.GetPeer(peerID); err == nil && peer != nil {
		return m.transport.DialPeer(peerID, peer.LastIP, peer.LastPort, "file")
	}
	return nil, fmt.Errorf("peer %s is unavailable", peerID)
}

func (m *Manager) executeDirectTransfer(path string, offer protocol.FileOfferPayload, target string) {
	conn, err := m.dialFilePeer(target)
	if err != nil {
		m.reportProgress(uploadProgress(offer, 0, 0, false, err.Error()))
		return
	}
	m.sendFileOnConnection(conn, path, offer)
}

func (m *Manager) sendFileOnConnection(conn *transport.ConnWrapper, path string, offer protocol.FileOfferPayload) {
	defer conn.Close()
	id := connectionID(conn)
	accept, done := make(chan []byte, 1), make(chan string, 1)
	m.pendingOffersMu.Lock()
	m.pendingAccepts[id], m.pendingDones[id] = accept, done
	m.pendingOffersMu.Unlock()
	defer m.clearUpload(id)
	err := m.writeOfferAndChunks(conn, path, offer, accept, done)
	if err != nil {
		m.reportProgress(uploadProgress(offer, 0, 0, false, err.Error()))
	}
}

func (m *Manager) clearUpload(id string) {
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	delete(m.pendingAccepts, id)
	delete(m.pendingDones, id)
}

func (m *Manager) writeOfferAndChunks(conn *transport.ConnWrapper, path string, offer protocol.FileOfferPayload, accept <-chan []byte, done <-chan string) error {
	payload, err := protocol.MarshalPayload(offer)
	if err != nil {
		return err
	}
	if err = conn.WriteFrame(protocol.TypeFileOffer, payload); err != nil {
		return err
	}
	var needed []byte
	select {
	case needed = <-accept:
	case status := <-done:
		return fmt.Errorf("receiver rejected file: %s", status)
	case <-conn.Done():
		return io.ErrUnexpectedEOF
	case <-m.ctx.Done():
		return m.ctx.Err()
	case <-time.After(15 * time.Second):
		return fmt.Errorf("timed out waiting for file acceptance")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sent, err := m.sendNeededChunks(conn, file, offer, needed)
	if err != nil {
		return err
	}
	m.awaitFileDone(done, offer, sent)
	return nil
}

func (m *Manager) HandleFileAccept(connID string, payload []byte) {
	var accept protocol.FileAcceptPayload
	if protocol.UnmarshalPayload(payload, &accept) != nil {
		return
	}
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if channel, ok := m.pendingAccepts[connID]; ok {
		select {
		case channel <- accept.NeededChunkBitset:
		default:
		}
	}
}

func (m *Manager) HandleFileDone(connID string, payload []byte) {
	var done protocol.FileDonePayload
	if protocol.UnmarshalPayload(payload, &done) != nil {
		return
	}
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if channel, ok := m.pendingDones[connID]; ok {
		select {
		case channel <- done.Status:
		default:
		}
	}
}
