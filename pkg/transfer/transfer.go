package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"kantorchat/pkg/config"
	"kantorchat/pkg/db"
	"kantorchat/pkg/discovery"
	"kantorchat/pkg/identity"
	"kantorchat/pkg/protocol"
	"kantorchat/pkg/transport"
)

var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

var dangerousExtensions = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".sh": true,
	".vbs": true, ".ps1": true, ".msi": true, ".com": true,
}

type TransferProgress struct {
	FileID       string
	Filename     string
	TotalBytes   int64
	SentBytes    int64
	SpeedBps     int64
	IsDownload   bool
	IsCompleted  bool
	ErrorMessage string
}

type ProgressCallback func(p TransferProgress)
type AnnounceCallback func(a protocol.FileAnnouncePayload)

type Manager struct {
	cfg        *config.Config
	idMgr      *identity.Manager
	store      *db.Store
	discovery  *discovery.Discovery
	transport  *transport.Manager
	onProgress ProgressCallback
	onAnnounce AnnounceCallback

	announcementsMu sync.RWMutex
	announcements   map[int]protocol.FileAnnouncePayload
	nextAnnounceNo  int

	pendingOffersMu sync.Mutex
	pendingAccepts  map[string]chan []byte // fileID -> needed bitmap
	pendingDones    map[string]chan string // fileID -> status

	connMapMu    sync.RWMutex
	connToFileID map[string]string
	connByFileID map[string]string

	ctx       context.Context
	cancelCtx context.CancelFunc
	wg        sync.WaitGroup
}

func New(
	cfg *config.Config,
	idMgr *identity.Manager,
	store *db.Store,
	disc *discovery.Discovery,
	trans *transport.Manager,
	onProgress ProgressCallback,
	onAnnounce AnnounceCallback,
) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		cfg:            cfg,
		idMgr:          idMgr,
		store:          store,
		discovery:      disc,
		transport:      trans,
		onProgress:     onProgress,
		onAnnounce:     onAnnounce,
		announcements:  make(map[int]protocol.FileAnnouncePayload),
		nextAnnounceNo: 1,
		pendingAccepts: make(map[string]chan []byte),
		pendingDones:   make(map[string]chan string),
		connToFileID:   make(map[string]string),
		connByFileID:   make(map[string]string),
		ctx:            ctx,
		cancelCtx:      cancel,
	}
}

func (m *Manager) Start() {
	m.CleanAbandonedPartialFiles()
}

func (m *Manager) Stop() {
	if m.cancelCtx != nil {
		m.cancelCtx()
	}
	m.wg.Wait()
}

func SanitizeFilename(rawName string) string {
	cleaned := strings.ReplaceAll(rawName, "\\", "/")
	base := filepath.Base(cleaned)
	base = strings.ReplaceAll(base, "..", "")
	base = strings.ReplaceAll(base, "/", "")
	base = strings.TrimSpace(base)

	if base == "" || base == "." {
		base = "downloaded_file"
	}

	ext := filepath.Ext(base)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	if windowsReservedNames[strings.ToUpper(nameWithoutExt)] {
		base = nameWithoutExt + "_file" + ext
	}

	return base
}

func IsDangerous(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return dangerousExtensions[ext]
}

func ComputeFileSHA256(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func (m *Manager) CleanAbandonedPartialFiles() {
	files, err := os.ReadDir(m.cfg.PartialDir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	for _, fi := range files {
		info, err := fi.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(m.cfg.PartialDir, fi.Name()))
		}
	}
}

func (m *Manager) GenerateThumbnail(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return "", nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return "", errors.New("invalid image bounds")
	}

	thumbMax := 64
	thumbW, thumbH := thumbMax, thumbMax
	if w > h {
		thumbH = (h * thumbMax) / w
	} else {
		thumbW = (w * thumbMax) / h
	}
	if thumbW <= 0 {
		thumbW = 1
	}
	if thumbH <= 0 {
		thumbH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, thumbW, thumbH))
	for y := 0; y < thumbH; y++ {
		for x := 0; x < thumbW; x++ {
			srcX := bounds.Min.X + (x*w)/thumbW
			srcY := bounds.Min.Y + (y*h)/thumbH
			dst.Set(x, y, img.At(srcX, srcY))
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 50}); err != nil {
		return "", err
	}

	if buf.Len() > 8192 {
		return "", nil
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func (m *Manager) SendFile(filePath, targetPeerID, roomName string) (*protocol.FileOfferPayload, error) {
	shaHex, size, err := ComputeFileSHA256(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash file: %w", err)
	}

	if size > m.cfg.MaxFileSize {
		return nil, fmt.Errorf("file size (%d bytes) exceeds max configured limit (%d bytes)", size, m.cfg.MaxFileSize)
	}

	filename := filepath.Base(filePath)
	totalChunks := int((size + int64(protocol.ChunkSize) - 1) / int64(protocol.ChunkSize))

	cachePath := filepath.Join(m.cfg.CacheDir, shaHex)
	_ = copyFile(filePath, cachePath)
	_ = m.store.AddMediaCache(db.MediaCacheRecord{
		FileID:    shaHex,
		Path:      cachePath,
		Size:      size,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	})

	offer := protocol.FileOfferPayload{
		V:           1,
		FileID:      shaHex,
		Filename:    filename,
		Size:        size,
		ChunkSize:   protocol.ChunkSize,
		TotalChunks: totalChunks,
		SHA256:      shaHex,
		Room:        roomName,
	}

	if roomName != "" {
		m.announcementsMu.Lock()
		num := m.nextAnnounceNo
		m.nextAnnounceNo++
		m.announcementsMu.Unlock()

		thumb, _ := m.GenerateThumbnail(filePath)
		announce := protocol.FileAnnouncePayload{
			V:          1,
			Room:       roomName,
			AnnounceNo: num,
			FileID:     shaHex,
			Filename:   filename,
			Size:       size,
			SenderID:   m.idMgr.PeerID(),
			Thumbnail:  thumb,
		}

		m.announcementsMu.Lock()
		m.announcements[num] = announce
		m.announcementsMu.Unlock()

		if m.onAnnounce != nil {
			m.onAnnounce(announce)
		}

		return &offer, nil
	}

	go m.executeDirectTransfer(filePath, offer, targetPeerID)
	return &offer, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func (m *Manager) executeDirectTransfer(filePath string, offer protocol.FileOfferPayload, targetPeerID string) {
	var targetIP string
	var targetPort int
	if peerInfo, ok := m.discovery.GetPeer(targetPeerID); ok && peerInfo.IsOnline {
		targetIP = peerInfo.IP
		targetPort = peerInfo.Port
	} else if pRec, err := m.store.GetPeer(targetPeerID); err == nil && pRec != nil {
		targetIP = pRec.LastIP
		targetPort = pRec.LastPort
	}

	if targetIP == "" || targetPort == 0 {
		m.reportProgress(offer.FileID, offer.Filename, offer.Size, 0, 0, false, false, "Penerima sedang offline")
		return
	}

	conn, err := m.transport.DialPeer(targetPeerID, targetIP, targetPort, "file")
	if err != nil {
		m.reportProgress(offer.FileID, offer.Filename, offer.Size, 0, 0, false, false, err.Error())
		return
	}
	defer conn.Close()

	acceptCh := make(chan []byte, 1)
	m.pendingOffersMu.Lock()
	m.pendingAccepts[offer.FileID] = acceptCh
	m.pendingOffersMu.Unlock()

	defer func() {
		m.pendingOffersMu.Lock()
		delete(m.pendingAccepts, offer.FileID)
		m.pendingOffersMu.Unlock()
	}()

	offerBytes, _ := protocol.MarshalPayload(offer)
	if err := conn.WriteFrame(protocol.TypeFileOffer, offerBytes); err != nil {
		return
	}

	var neededBitset []byte
	select {
	case neededBitset = <-acceptCh:
	case <-time.After(15 * time.Second):
		m.reportProgress(offer.FileID, offer.Filename, offer.Size, 0, 0, false, false, "Timeout menunggu persetujuan penerima")
		return
	}

	f, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer f.Close()

	doneCh := make(chan string, 1)
	m.pendingOffersMu.Lock()
	m.pendingDones[offer.FileID] = doneCh
	m.pendingOffersMu.Unlock()

	defer func() {
		m.pendingOffersMu.Lock()
		delete(m.pendingDones, offer.FileID)
		m.pendingOffersMu.Unlock()
	}()

	startTime := time.Now()
	var sentBytes int64

	buf := make([]byte, protocol.ChunkSize)
	for i := 0; i < offer.TotalChunks; i++ {
		byteIdx := i / 8
		bitIdx := uint(i % 8)
		if byteIdx < len(neededBitset) && (neededBitset[byteIdx]&(1<<bitIdx)) == 0 {
			sentBytes += int64(protocol.ChunkSize)
			continue
		}

		offset := int64(i) * int64(protocol.ChunkSize)
		n, err := f.ReadAt(buf, offset)
		if err != nil && err != io.EOF {
			break
		}
		if n <= 0 {
			break
		}

		chunkPayload := make([]byte, 4+n)
		binary.BigEndian.PutUint32(chunkPayload[0:4], uint32(i))
		copy(chunkPayload[4:], buf[:n])

		if err := conn.WriteFrame(protocol.TypeChunk, chunkPayload); err != nil {
			m.reportProgress(offer.FileID, offer.Filename, offer.Size, sentBytes, 0, false, false, "Koneksi terputus")
			return
		}

		sentBytes += int64(n)
		elapsed := time.Since(startTime).Seconds()
		var speed int64
		if elapsed > 0 {
			speed = int64(float64(sentBytes) / elapsed)
		}
		m.reportProgress(offer.FileID, offer.Filename, offer.Size, sentBytes, speed, false, false, "")
	}

	select {
	case status := <-doneCh:
		if status == "ok" {
			m.reportProgress(offer.FileID, offer.Filename, offer.Size, offer.Size, 0, false, true, "")
		} else {
			m.reportProgress(offer.FileID, offer.Filename, offer.Size, sentBytes, 0, false, false, "Verifikasi integritas gagal")
		}
	case <-time.After(30 * time.Second):
		m.reportProgress(offer.FileID, offer.Filename, offer.Size, sentBytes, 0, false, false, "Timeout verifikasi")
	}
}

func (m *Manager) reportProgress(fileID, filename string, total, sent, speed int64, isDownload, isCompleted bool, errMsg string) {
	if m.onProgress != nil {
		m.onProgress(TransferProgress{
			FileID:       fileID,
			Filename:     filename,
			TotalBytes:   total,
			SentBytes:    sent,
			SpeedBps:     speed,
			IsDownload:   isDownload,
			IsCompleted:  isCompleted,
			ErrorMessage: errMsg,
		})
	}
}

func (m *Manager) HandleFileOffer(connID string, payload []byte, fromPeerID string) {
	var offer protocol.FileOfferPayload
	if err := protocol.UnmarshalPayload(payload, &offer); err != nil {
		return
	}

	sanitizedName := SanitizeFilename(offer.Filename)
	partPath := filepath.Join(m.cfg.PartialDir, offer.FileID+".part")
	metaPath := filepath.Join(m.cfg.PartialDir, offer.FileID+".meta")

	bitsetLen := (offer.TotalChunks + 7) / 8
	bitset := make([]byte, bitsetLen)
	for i := 0; i < offer.TotalChunks; i++ {
		bitset[i/8] |= (1 << uint(i%8))
	}

	if data, err := os.ReadFile(metaPath); err == nil && len(data) == bitsetLen {
		copy(bitset, data)
	} else {
		f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			_ = f.Truncate(offer.Size)
			_ = f.Close()
		}
		_ = os.WriteFile(metaPath, bitset, 0600)
	}

	m.connMapMu.Lock()
	m.connToFileID[connID] = offer.FileID
	m.connByFileID[offer.FileID] = connID
	m.connMapMu.Unlock()

	_ = m.store.UpsertTransfer(db.TransferRecord{
		FileID:    offer.FileID,
		PeerID:    fromPeerID,
		Direction: "download",
		Filename:  sanitizedName,
		Size:      offer.Size,
		State:     "transferring",
		LocalPath: partPath,
		UpdatedAt: time.Now(),
	})

	acceptPayload, _ := protocol.MarshalPayload(protocol.FileAcceptPayload{
		V:                 1,
		FileID:            offer.FileID,
		NeededChunkBitset: bitset,
	})
	if err := m.transport.SendToConn(connID, protocol.TypeFileAccept, acceptPayload); err != nil {
		_ = m.transport.SendToPeer(fromPeerID, protocol.TypeFileAccept, acceptPayload)
	}
}

func (m *Manager) HandleFileAccept(payload []byte) {
	var accept protocol.FileAcceptPayload
	if err := protocol.UnmarshalPayload(payload, &accept); err != nil {
		return
	}

	m.pendingOffersMu.Lock()
	ch, ok := m.pendingAccepts[accept.FileID]
	m.pendingOffersMu.Unlock()

	if ok {
		select {
		case ch <- accept.NeededChunkBitset:
		default:
		}
	}
}

func (m *Manager) HandleChunk(connID string, payload []byte, fromPeerID string) {
	if len(payload) < 4 {
		return
	}

	chunkIdx := binary.BigEndian.Uint32(payload[0:4])
	chunkData := payload[4:]

	m.connMapMu.RLock()
	fileID := m.connToFileID[connID]
	m.connMapMu.RUnlock()

	m.saveIncomingChunk(connID, fileID, chunkIdx, chunkData, fromPeerID)
}

func (m *Manager) saveIncomingChunk(connID, fileID string, chunkIdx uint32, chunkData []byte, fromPeerID string) {
	if fileID == "" {
		entries, err := os.ReadDir(m.cfg.PartialDir)
		if err == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".part") {
					fileID = strings.TrimSuffix(entry.Name(), ".part")
					break
				}
			}
		}
	}
	if fileID == "" {
		return
	}

	partPath := filepath.Join(m.cfg.PartialDir, fileID+".part")
	metaPath := filepath.Join(m.cfg.PartialDir, fileID+".meta")

	offset := int64(chunkIdx) * int64(protocol.ChunkSize)
	f, err := os.OpenFile(partPath, os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	_, _ = f.WriteAt(chunkData, offset)
	_ = f.Close()

	bitset, _ := os.ReadFile(metaPath)
	byteIdx := int(chunkIdx / 8)
	bitIdx := uint(chunkIdx % 8)
	if byteIdx < len(bitset) {
		bitset[byteIdx] &^= (1 << bitIdx)
		_ = os.WriteFile(metaPath, bitset, 0600)
	}

	if m.isTransferComplete(bitset) {
		m.finalizeDownload(connID, fileID, partPath, metaPath, fromPeerID)
	}
}

func (m *Manager) isTransferComplete(bitset []byte) bool {
	for _, b := range bitset {
		if b != 0 {
			return false
		}
	}
	return len(bitset) > 0
}

func (m *Manager) finalizeDownload(connID, fileID, partPath, metaPath, fromPeerID string) {
	shaHex, size, err := ComputeFileSHA256(partPath)
	if err != nil || shaHex != fileID {
		donePayload, _ := protocol.MarshalPayload(protocol.FileDonePayload{
			V:      1,
			FileID: fileID,
			Status: "checksum_mismatch",
		})
		if connID != "" {
			_ = m.transport.SendToConn(connID, protocol.TypeFileDone, donePayload)
		}
		_ = m.transport.SendToPeer(fromPeerID, protocol.TypeFileDone, donePayload)
		return
	}

	trans, err := m.store.GetTransfer(fileID, fromPeerID, "download")
	finalFilename := "downloaded_file"
	if err == nil && trans != nil {
		finalFilename = trans.Filename
	}

	destPath := filepath.Join(m.cfg.DownloadsDir, finalFilename)
	counter := 1
	for {
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			break
		}
		ext := filepath.Ext(finalFilename)
		nameWithoutExt := strings.TrimSuffix(finalFilename, ext)
		destPath = filepath.Join(m.cfg.DownloadsDir, fmt.Sprintf("%s(%d)%s", nameWithoutExt, counter, ext))
		counter++
	}

	_ = os.Rename(partPath, destPath)
	_ = os.Remove(metaPath)

	_ = m.store.UpsertTransfer(db.TransferRecord{
		FileID:    fileID,
		PeerID:    fromPeerID,
		Direction: "download",
		Filename:  filepath.Base(destPath),
		Size:      size,
		State:     "completed",
		LocalPath: destPath,
		UpdatedAt: time.Now(),
	})

	donePayload, _ := protocol.MarshalPayload(protocol.FileDonePayload{
		V:      1,
		FileID: fileID,
		Status: "ok",
	})
	if connID != "" {
		_ = m.transport.SendToConn(connID, protocol.TypeFileDone, donePayload)
	}
	_ = m.transport.SendToPeer(fromPeerID, protocol.TypeFileDone, donePayload)

	m.reportProgress(fileID, filepath.Base(destPath), size, size, 0, true, true, "")
}

func (m *Manager) HandleFileDone(payload []byte) {
	var done protocol.FileDonePayload
	if err := protocol.UnmarshalPayload(payload, &done); err != nil {
		return
	}

	m.pendingOffersMu.Lock()
	ch, ok := m.pendingDones[done.FileID]
	m.pendingOffersMu.Unlock()

	if ok {
		select {
		case ch <- done.Status:
		default:
		}
	}
}

func (m *Manager) HandleFileAnnounce(payload []byte) {
	var announce protocol.FileAnnouncePayload
	if err := protocol.UnmarshalPayload(payload, &announce); err != nil {
		return
	}

	m.announcementsMu.Lock()
	m.announcements[announce.AnnounceNo] = announce
	m.announcementsMu.Unlock()

	if m.onAnnounce != nil {
		m.onAnnounce(announce)
	}

	if announce.Size <= m.cfg.AutoDownloadImageMax && announce.Thumbnail != "" {
		go m.PullAnnouncedFile(announce.AnnounceNo)
	}
}

func (m *Manager) PullAnnouncedFile(announceNo int) error {
	m.announcementsMu.RLock()
	announce, ok := m.announcements[announceNo]
	m.announcementsMu.RUnlock()

	if !ok {
		return fmt.Errorf("pengumuman file #%d tidak ditemukan", announceNo)
	}

	peerInfo, ok := m.discovery.GetPeer(announce.SenderID)
	if ok && peerInfo.IsOnline {
		conn, err := m.transport.DialPeer(announce.SenderID, peerInfo.IP, peerInfo.Port, "file")
		if err == nil {
			offerPayload, _ := protocol.MarshalPayload(protocol.FileOfferPayload{
				V:           1,
				FileID:      announce.FileID,
				Filename:    announce.Filename,
				Size:        announce.Size,
				ChunkSize:   protocol.ChunkSize,
				TotalChunks: int((announce.Size + int64(protocol.ChunkSize) - 1) / int64(protocol.ChunkSize)),
				SHA256:      announce.FileID,
				Room:        announce.Room,
			})
			_ = conn.WriteFrame(protocol.TypeFileOffer, offerPayload)
			return nil
		}
	}

	// Multi-source fallback: broadcast TypeFileWhoHas to room members
	whoHasPayload, _ := protocol.MarshalPayload(protocol.FileWhoHasPayload{
		V:      1,
		Room:   announce.Room,
		FileID: announce.FileID,
	})

	members, _ := m.store.GetRoomMembers(announce.Room)
	for _, mID := range members {
		if mID != m.idMgr.PeerID() && mID != announce.SenderID {
			_ = m.transport.SendToPeer(mID, protocol.TypeFileWhoHas, whoHasPayload)
		}
	}

	return nil
}

func (m *Manager) HandleFileWhoHas(payload []byte, fromPeerID string) {
	var req protocol.FileWhoHasPayload
	if err := protocol.UnmarshalPayload(payload, &req); err != nil {
		return
	}

	members, _ := m.store.GetRoomMembers(req.Room)
	isMember := false
	for _, mID := range members {
		if mID == fromPeerID {
			isMember = true
			break
		}
	}
	if !isMember {
		return
	}

	cacheRec, err := m.store.GetMediaCache(req.FileID)
	if err == nil && cacheRec != nil {
		havePayload, _ := protocol.MarshalPayload(protocol.FileHavePayload{
			V:      1,
			Room:   req.Room,
			FileID: req.FileID,
			PeerID: m.idMgr.PeerID(),
		})
		_ = m.transport.SendToPeer(fromPeerID, protocol.TypeFileHave, havePayload)
	}
}

func (m *Manager) HandleFileHave(payload []byte) {
	var have protocol.FileHavePayload
	if err := protocol.UnmarshalPayload(payload, &have); err != nil {
		return
	}

	m.announcementsMu.RLock()
	var matchingAnnounce *protocol.FileAnnouncePayload
	for _, a := range m.announcements {
		if a.FileID == have.FileID {
			cp := a
			matchingAnnounce = &cp
			break
		}
	}
	m.announcementsMu.RUnlock()

	if matchingAnnounce != nil {
		peerInfo, ok := m.discovery.GetPeer(have.PeerID)
		if ok && peerInfo.IsOnline {
			conn, err := m.transport.DialPeer(have.PeerID, peerInfo.IP, peerInfo.Port, "file")
			if err == nil {
				offerPayload, _ := protocol.MarshalPayload(protocol.FileOfferPayload{
					V:           1,
					FileID:      matchingAnnounce.FileID,
					Filename:    matchingAnnounce.Filename,
					Size:        matchingAnnounce.Size,
					ChunkSize:   protocol.ChunkSize,
					TotalChunks: int((matchingAnnounce.Size + int64(protocol.ChunkSize) - 1) / int64(protocol.ChunkSize)),
					SHA256:      matchingAnnounce.FileID,
					Room:        matchingAnnounce.Room,
				})
				_ = conn.WriteFrame(protocol.TypeFileOffer, offerPayload)
			}
		}
	}
}
