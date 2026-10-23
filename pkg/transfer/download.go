package transfer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"kerykeion/pkg/protocol"
)

type downloadWait struct {
	announce protocol.FileAnnouncePayload
	started  chan struct{}
	result   chan error
}

func (m *Manager) PullAnnouncedFile(number int) error {
	if !m.beginWork() {
		return m.ctx.Err()
	}
	defer m.wg.Done()
	m.announcementsMu.RLock()
	announce, ok := m.announcements[number]
	m.announcementsMu.RUnlock()
	if !ok {
		return fmt.Errorf("file announcement #%d was not found", number)
	}
	return m.pullAnnouncement(announce)
}

func (m *Manager) pullAnnouncement(announce protocol.FileAnnouncePayload) error {
	if m.existingDownload(announce) {
		return nil
	}
	m.pendingOffersMu.Lock()
	if m.activePulls[announce.FileID] {
		m.pendingOffersMu.Unlock()
		return fmt.Errorf("this file is already downloading")
	}
	m.activePulls[announce.FileID] = true
	m.pendingOffersMu.Unlock()
	defer m.clearPull(announce.FileID)
	err := m.pullFromPeer(announce, announce.SenderID)
	if err == nil {
		return nil
	}
	var pathError *os.PathError
	var linkError *os.LinkError
	if errors.As(err, &pathError) || errors.As(err, &linkError) || errors.Is(err, m.ctx.Err()) {
		return err
	}
	if announce.Room == "" {
		return err
	}
	return m.pullFromRoomCache(announce, err)
}

func (m *Manager) clearPull(fileID string) {
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	delete(m.activePulls, fileID)
	delete(m.pendingSources, fileID)
}

func (m *Manager) pullFromPeer(announce protocol.FileAnnouncePayload, peerID string) error {
	conn, err := m.dialFilePeer(peerID)
	if err != nil {
		return err
	}
	defer conn.Close()
	id := connectionID(conn)
	wait := &downloadWait{announce: announce, started: make(chan struct{}, 1), result: make(chan error, 1)}
	m.pendingOffersMu.Lock()
	m.pendingDownloads[id] = wait
	m.pendingOffersMu.Unlock()
	defer m.clearDownload(id)
	payload, _ := protocol.MarshalPayload(protocol.FileRequestPayload{V: 1, Room: announce.Room, FileID: announce.FileID})
	if err := conn.WriteFrame(protocol.TypeFileRequest, payload); err != nil {
		return err
	}
	return m.waitForDownload(wait, conn.Done())
}

func (m *Manager) clearDownload(connID string) {
	m.pendingOffersMu.Lock()
	delete(m.pendingDownloads, connID)
	m.pendingOffersMu.Unlock()
}

func (m *Manager) waitForDownload(wait *downloadWait, closed <-chan struct{}) error {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case err := <-wait.result:
		return err
	case <-wait.started:
	case <-closed:
		return closedDownloadResult(wait)
	case <-m.ctx.Done():
		return m.ctx.Err()
	case <-timer.C:
		return fmt.Errorf("timed out waiting for the file owner to offer the download")
	}
	timer.Reset(5 * time.Minute)
	select {
	case err := <-wait.result:
		return err
	case <-closed:
		return closedDownloadResult(wait)
	case <-m.ctx.Done():
		return m.ctx.Err()
	case <-timer.C:
		return fmt.Errorf("download timed out; retry /get to resume")
	}
}

func closedDownloadResult(wait *downloadWait) error {
	select {
	case err := <-wait.result:
		return err
	default:
		return io.ErrUnexpectedEOF
	}
}

func (m *Manager) pullFromRoomCache(announce protocol.FileAnnouncePayload, originalError error) error {
	sources := make(chan protocol.FileHavePayload, 32)
	m.pendingOffersMu.Lock()
	m.pendingSources[announce.FileID] = sources
	m.pendingOffersMu.Unlock()
	payload, _ := protocol.MarshalPayload(protocol.FileWhoHasPayload{V: 1, Room: announce.Room, FileID: announce.FileID})
	members, _ := m.store.GetRoomMembers(announce.Room)
	for _, id := range members {
		if id != m.idMgr.PeerID() && id != announce.SenderID {
			_ = m.sendPeerFrame(id, protocol.TypeFileWhoHas, payload)
		}
	}
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case source := <-sources:
			if source.Room == announce.Room && m.pullFromPeer(announce, source.PeerID) == nil {
				return nil
			}
		case <-m.ctx.Done():
			return m.ctx.Err()
		case <-timer.C:
			return fmt.Errorf("file owner could not serve the download (%v); no room cache was available", originalError)
		}
	}
}

func (m *Manager) finishRequestedDownload(connID string, err error) {
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if wait, ok := m.pendingDownloads[connID]; ok {
		select {
		case wait.result <- err:
		default:
		}
	}
}

func (m *Manager) HandleFileReject(connID string, payload []byte) {
	var rejection protocol.FileRejectPayload
	if protocol.UnmarshalPayload(payload, &rejection) != nil {
		return
	}
	m.finishRequestedDownload(connID, fmt.Errorf("file request rejected: %s", rejection.Reason))
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if done, ok := m.pendingDones[connID]; ok {
		select {
		case done <- rejection.Reason:
		default:
		}
	}
}
