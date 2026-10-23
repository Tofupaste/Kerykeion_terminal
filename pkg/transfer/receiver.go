package transfer

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/protocol"
)

func validFileID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 32 && id == fmt.Sprintf("%x", decoded)
}

func (m *Manager) validateOffer(connID string, offer protocol.FileOfferPayload) error {
	if !validFileID(offer.FileID) || offer.SHA256 != offer.FileID {
		return fmt.Errorf("invalid file hash")
	}
	if offer.Size < 0 || offer.Size > m.cfg.MaxFileSize {
		return fmt.Errorf("file exceeds the configured size limit")
	}
	count := (offer.Size + protocol.ChunkSize - 1) / protocol.ChunkSize
	if offer.ChunkSize != protocol.ChunkSize || int64(offer.TotalChunks) != count {
		return fmt.Errorf("invalid file chunk layout")
	}
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if wait, ok := m.pendingDownloads[connID]; ok {
		if wait.announce.FileID != offer.FileID || wait.announce.Size != offer.Size || wait.announce.Room != offer.Room {
			return fmt.Errorf("file offer does not match the requested announcement")
		}
	}
	return nil
}

func (m *Manager) HandleFileOffer(connID string, payload []byte, fromPeerID string) {
	m.receiveMu.Lock()
	defer m.receiveMu.Unlock()
	var offer protocol.FileOfferPayload
	if err := protocol.UnmarshalPayload(payload, &offer); err != nil {
		return
	}
	if err := m.prepareDownload(connID, offer, fromPeerID); err != nil {
		m.downloadFailure(connID, offer.FileID, offer.Filename, err)
	}
}

func (m *Manager) prepareDownload(connID string, offer protocol.FileOfferPayload, peerID string) error {
	if err := m.validateOffer(connID, offer); err != nil {
		return err
	}
	bitmap, err := m.preparePartialFile(offer)
	if err != nil {
		return err
	}
	partPath := filepath.Join(m.cfg.PartialDir, offer.FileID+".part")
	err = m.store.UpsertTransfer(db.TransferRecord{FileID: offer.FileID, PeerID: peerID, Direction: "download", Filename: SanitizeFilename(offer.Filename), Size: offer.Size, State: "transferring", LocalPath: partPath, UpdatedAt: time.Now()})
	if err != nil {
		return err
	}
	m.connMapMu.Lock()
	m.connToFileID[connID] = offer.FileID
	m.connByFileID[offer.FileID] = connID
	m.connMapMu.Unlock()
	payload, _ := protocol.MarshalPayload(protocol.FileAcceptPayload{V: 1, FileID: offer.FileID, NeededChunkBitset: bitmap})
	if err = m.transport.SendToConn(connID, protocol.TypeFileAccept, payload); err != nil {
		return err
	}
	m.markDownloadStarted(connID)
	if offer.TotalChunks == 0 || m.isTransferComplete(bitmap) {
		return m.finalizeDownload(connID, offer.FileID, peerID)
	}
	return nil
}

func (m *Manager) preparePartialFile(offer protocol.FileOfferPayload) ([]byte, error) {
	partPath := filepath.Join(m.cfg.PartialDir, offer.FileID+".part")
	metaPath := filepath.Join(m.cfg.PartialDir, offer.FileID+".meta")
	length := (offer.TotalChunks + 7) / 8
	info, statErr := os.Stat(partPath)
	bitmap, err := os.ReadFile(metaPath)
	if err == nil && statErr == nil && info.Size() == offer.Size && len(bitmap) == length {
		return bitmap, nil
	}
	bitmap = make([]byte, length)
	for i := 0; i < offer.TotalChunks; i++ {
		bitmap[i/8] |= 1 << uint(i%8)
	}
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	err = file.Truncate(offer.Size)
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return bitmap, os.WriteFile(metaPath, bitmap, 0600)
}

func (m *Manager) markDownloadStarted(connID string) {
	m.pendingOffersMu.Lock()
	defer m.pendingOffersMu.Unlock()
	if wait, ok := m.pendingDownloads[connID]; ok {
		select {
		case wait.started <- struct{}{}:
		default:
		}
	}
}

func (m *Manager) HandleChunk(connID string, payload []byte, peerID string) {
	if len(payload) < 4 {
		return
	}
	m.receiveMu.Lock()
	defer m.receiveMu.Unlock()
	m.connMapMu.RLock()
	id := m.connToFileID[connID]
	m.connMapMu.RUnlock()
	if id == "" {
		return
	}
	if err := m.saveIncomingChunk(connID, id, binary.BigEndian.Uint32(payload[:4]), payload[4:], peerID); err != nil {
		m.downloadFailure(connID, id, "", err)
	}
}

func (m *Manager) saveIncomingChunk(connID, id string, index uint32, data []byte, peerID string) error {
	record, err := m.store.GetTransfer(id, peerID, "download")
	if err != nil || record == nil {
		return fmt.Errorf("download metadata is missing")
	}
	offset := int64(index) * protocol.ChunkSize
	remaining := record.Size - offset
	if remaining <= 0 || int64(len(data)) != min(int64(protocol.ChunkSize), remaining) {
		return fmt.Errorf("invalid file chunk")
	}
	file, err := os.OpenFile(record.LocalPath, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = file.WriteAt(data, offset)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	bitmapPath := filepath.Join(m.cfg.PartialDir, id+".meta")
	bitmap, err := os.ReadFile(bitmapPath)
	if err != nil {
		return err
	}
	if int(index/8) >= len(bitmap) {
		return fmt.Errorf("invalid file chunk index")
	}
	bitmap[index/8] &^= 1 << uint(index%8)
	if err = os.WriteFile(bitmapPath, bitmap, 0600); err != nil {
		return err
	}
	if m.isTransferComplete(bitmap) {
		return m.finalizeDownload(connID, id, peerID)
	}
	m.reportProgress(TransferProgress{FileID: id, Filename: record.Filename, TotalBytes: record.Size, SentBytes: min(offset+int64(len(data)), record.Size), IsDownload: true})
	return nil
}

func (m *Manager) isTransferComplete(bitmap []byte) bool {
	for _, value := range bitmap {
		if value != 0 {
			return false
		}
	}
	return len(bitmap) > 0
}

func (m *Manager) finalizeDownload(connID, id, peerID string) error {
	part := filepath.Join(m.cfg.PartialDir, id+".part")
	meta := filepath.Join(m.cfg.PartialDir, id+".meta")
	hash, size, err := ComputeFileSHA256(part)
	if err != nil {
		return err
	}
	if hash != id {
		_ = os.Remove(meta)
		return fmt.Errorf("download failed SHA-256 verification")
	}
	record, err := m.store.GetTransfer(id, peerID, "download")
	if err != nil || record == nil {
		return fmt.Errorf("download metadata is missing")
	}
	dest, err := m.downloadDestination(record.Filename)
	if err != nil {
		return err
	}
	if err = os.Rename(part, dest); err != nil {
		return err
	}
	_ = os.Remove(meta)
	record.Filename, record.LocalPath, record.State, record.UpdatedAt = filepath.Base(dest), dest, "completed", time.Now()
	if err = m.store.UpsertTransfer(*record); err != nil {
		return err
	}
	_ = m.store.AddMediaCache(db.MediaCacheRecord{FileID: id, Path: dest, Size: size, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)})
	done, _ := protocol.MarshalPayload(protocol.FileDonePayload{V: 1, FileID: id, Status: "ok"})
	_ = m.transport.SendToConn(connID, protocol.TypeFileDone, done)
	m.reportProgress(TransferProgress{FileID: id, Filename: filepath.Base(dest), TotalBytes: size, SentBytes: size, IsDownload: true, IsCompleted: true, LocalPath: dest})
	m.finishRequestedDownload(connID, nil)
	m.clearIncomingConnection(connID, id)
	return nil
}

func (m *Manager) downloadDestination(filename string) (string, error) {
	for counter := 0; ; counter++ {
		name := filename
		if counter > 0 {
			ext := filepath.Ext(filename)
			name = fmt.Sprintf("%s(%d)%s", filename[:len(filename)-len(ext)], counter, ext)
		}
		path := filepath.Join(m.cfg.DownloadsDir, name)
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return path, nil
		}
		if err != nil {
			return "", err
		}
	}
}

func (m *Manager) clearIncomingConnection(connID, id string) {
	m.connMapMu.Lock()
	defer m.connMapMu.Unlock()
	delete(m.connToFileID, connID)
	if m.connByFileID[id] == connID {
		delete(m.connByFileID, id)
	}
}

func (m *Manager) downloadFailure(connID, id, filename string, err error) {
	m.rejectFile(connID, id, err)
	m.finishRequestedDownload(connID, err)
	m.reportProgress(TransferProgress{FileID: id, Filename: filename, IsDownload: true, ErrorMessage: err.Error()})
	m.clearIncomingConnection(connID, id)
}
