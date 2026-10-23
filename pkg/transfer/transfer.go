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

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transport"
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
	LocalPath    string
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

	pendingOffersMu  sync.Mutex
	pendingAccepts   map[string]chan []byte // connection -> needed bitmap
	pendingDones     map[string]chan string // connection -> status
	pendingDownloads map[string]*downloadWait
	pendingSources   map[string]chan protocol.FileHavePayload
	activePulls      map[string]bool
	receiveMu        sync.Mutex
	workerMu         sync.Mutex
	stopped          bool

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
		cfg:              cfg,
		idMgr:            idMgr,
		store:            store,
		discovery:        disc,
		transport:        trans,
		onProgress:       onProgress,
		onAnnounce:       onAnnounce,
		announcements:    make(map[int]protocol.FileAnnouncePayload),
		nextAnnounceNo:   1,
		pendingAccepts:   make(map[string]chan []byte),
		pendingDones:     make(map[string]chan string),
		pendingDownloads: make(map[string]*downloadWait),
		pendingSources:   make(map[string]chan protocol.FileHavePayload),
		activePulls:      make(map[string]bool),
		connToFileID:     make(map[string]string),
		connByFileID:     make(map[string]string),
		ctx:              ctx,
		cancelCtx:        cancel,
	}
}

func (m *Manager) Start() {
	m.CleanAbandonedPartialFiles()
}

func (m *Manager) Stop() {
	m.workerMu.Lock()
	m.stopped = true
	m.cancelCtx()
	m.workerMu.Unlock()
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
	if filePath != cachePath {
		if err := copyFile(filePath, cachePath); err != nil {
			return nil, fmt.Errorf("failed to cache file: %w", err)
		}
	}
	err = m.store.AddMediaCache(db.MediaCacheRecord{
		FileID:    shaHex,
		Path:      cachePath,
		Size:      size,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
	})
	if err != nil {
		return nil, err
	}

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
		if err := m.announceRoomFile(offer, filePath); err != nil {
			return nil, err
		}
		return &offer, nil
	}
	if !m.startWorker(func() { m.executeDirectTransfer(cachePath, offer, targetPeerID) }) {
		return nil, m.ctx.Err()
	}

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

func (m *Manager) sendNeededChunks(conn *transport.ConnWrapper, file *os.File, offer protocol.FileOfferPayload, needed []byte) (int64, error) {
	start := time.Now()
	var sent int64
	buf := make([]byte, protocol.ChunkSize)
	for index := 0; index < offer.TotalChunks; index++ {
		if chunkAlreadyReceived(needed, index) {
			sent += int64(protocol.ChunkSize)
			continue
		}
		n, err := file.ReadAt(buf, int64(index)*int64(protocol.ChunkSize))
		if err != nil && err != io.EOF {
			break
		}
		if n <= 0 {
			break
		}
		payload := make([]byte, 4+n)
		binary.BigEndian.PutUint32(payload[:4], uint32(index))
		copy(payload[4:], buf[:n])
		if err := conn.WriteFrame(protocol.TypeChunk, payload); err != nil {
			m.reportProgress(uploadProgress(offer, sent, 0, false, "Koneksi terputus"))
			return sent, err
		}
		sent += int64(n)
		m.reportProgress(uploadProgress(offer, sent, transferSpeed(sent, start), false, ""))
	}
	return sent, nil
}

func chunkAlreadyReceived(needed []byte, index int) bool {
	byteIndex := index / 8
	return byteIndex < len(needed) && needed[byteIndex]&(1<<uint(index%8)) == 0
}

func transferSpeed(sent int64, started time.Time) int64 {
	elapsed := time.Since(started).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return int64(float64(sent) / elapsed)
}

func (m *Manager) awaitFileDone(done <-chan string, offer protocol.FileOfferPayload, sent int64) {
	select {
	case <-m.ctx.Done():
		return
	case status := <-done:
		if status == "ok" {
			m.reportProgress(uploadProgress(offer, offer.Size, 0, true, ""))
		} else {
			m.reportProgress(uploadProgress(offer, sent, 0, false, "Verifikasi integritas gagal"))
		}
	case <-time.After(30 * time.Second):
		m.reportProgress(uploadProgress(offer, sent, 0, false, "Timeout verifikasi"))
	}
}

func uploadProgress(offer protocol.FileOfferPayload, sent, speed int64, completed bool, errMsg string) TransferProgress {
	return TransferProgress{
		FileID: offer.FileID, Filename: offer.Filename, TotalBytes: offer.Size,
		SentBytes: sent, SpeedBps: speed, IsCompleted: completed, ErrorMessage: errMsg,
	}
}

func (m *Manager) reportProgress(progress TransferProgress) {
	if m.onProgress != nil {
		m.onProgress(progress)
	}
}
