package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	TypeHello       byte = 0x01
	TypeMsg         byte = 0x02
	TypeMsgAck      byte = 0x03
	TypeFileOffer   byte = 0x10
	TypeFileAccept  byte = 0x11
	TypeChunk       byte = 0x12
	TypeChunkAck    byte = 0x13
	TypeFileDone    byte = 0x14
	TypeFileReject  byte = 0x15
	TypeRoomMeta    byte = 0x20
	TypeRoomJoin    byte = 0x21
	TypeRoomLeave   byte = 0x22
	TypeRoomClosed  byte = 0x23
	TypeRoomTransfer byte = 0x24
	TypeRoomSyncReq byte = 0x25
	TypeRoomSyncRes byte = 0x26
	TypeFileAnnounce byte = 0x30
	TypeFileWhoHas  byte = 0x31
	TypeFileHave    byte = 0x32
	TypeAIQueryReq   byte = 0x40
	TypeAIQueryRes   byte = 0x41
	TypeAISessionSync byte = 0x42
	TypePing        byte = 0x7F

	MaxFrameLength = 1024 * 1024 // 1 MB
	ChunkSize      = 256 * 1024  // 256 KB
)

var (
	ErrFrameTooLarge = errors.New("frame exceeds maximum allowed size (1MB)")
	ErrInvalidFrame  = errors.New("invalid frame structure")
)

func ReadFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}

	frameType := header[0]
	length := binary.BigEndian.Uint32(header[1:5])

	if length > MaxFrameLength {
		return 0, nil, ErrFrameTooLarge
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}

	return frameType, payload, nil
}

func WriteFrame(w io.Writer, frameType byte, payload []byte) error {
	length := len(payload)
	if length > MaxFrameLength {
		return ErrFrameTooLarge
	}

	header := make([]byte, 5)
	header[0] = frameType
	binary.BigEndian.PutUint32(header[1:5], uint32(length))

	if _, err := w.Write(header); err != nil {
		return err
	}
	if length > 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

func MarshalPayload(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func UnmarshalPayload(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// UDP Announce packet
type AnnouncePacket struct {
	V         int    `json:"v"`
	Type      string `json:"type"` // ANNOUNCE
	PubKey    string `json:"pubkey"`
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Seq       uint64 `json:"seq"`
	Timestamp int64  `json:"ts"`
	Sig       string `json:"sig"`
	OfficeKey string `json:"office_key,omitempty"`
}

func (a *AnnouncePacket) SigBytes() []byte {
	return []byte(fmt.Sprintf("%d:%s:%s:%s:%d:%d:%d:%s", a.V, a.Type, a.PubKey, a.Name, a.Port, a.Seq, a.Timestamp, a.OfficeKey))
}

// UDP Room Query & Here packets
type RoomQueryPacket struct {
	V        int    `json:"v"`
	Type     string `json:"type"` // ROOM_QUERY
	RoomName string `json:"room_name"`
	FromPeer string `json:"from_peer"`
	FromPort int    `json:"from_port"`
}

type RoomHerePacket struct {
	V        int    `json:"v"`
	Type     string `json:"type"` // ROOM_HERE
	RoomName string `json:"room_name"`
	Gen      int64  `json:"gen"`
	MasterID string `json:"master_id"`
	FromPeer string `json:"from_peer"`
	FromPort int    `json:"from_port"`
}

// TCP Payloads
type HelloPayload struct {
	V           int      `json:"v"`
	PeerID      string   `json:"peer_id"`
	DisplayName string   `json:"display_name"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type MsgPayload struct {
	V        int    `json:"v"`
	MsgID    string `json:"msg_id"` // UUIDv7
	AuthorID string `json:"author_id"`
	Room     string `json:"room,omitempty"` // empty for DM
	Body     string `json:"body"`
	TS       int64  `json:"ts"`
	Sig      string `json:"sig"`
	FileRef  string `json:"file_ref,omitempty"`
}

func (m *MsgPayload) SigBytes() []byte {
	return []byte(fmt.Sprintf("%s:%s:%s:%s:%d:%s", m.MsgID, m.AuthorID, m.Room, m.Body, m.TS, m.FileRef))
}

type MsgAckPayload struct {
	V     int    `json:"v"`
	MsgID string `json:"msg_id"`
}

type FileOfferPayload struct {
	V           int    `json:"v"`
	FileID      string `json:"file_id"` // SHA-256 of entire file
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	ChunkSize   int    `json:"chunk_size"`
	TotalChunks int    `json:"total_chunks"`
	SHA256      string `json:"sha256"`
	Room        string `json:"room,omitempty"`
}

type FileAcceptPayload struct {
	V           int    `json:"v"`
	FileID      string `json:"file_id"`
	NeededChunkBitset []byte `json:"needed_chunks"` // Bitmap of needed chunks
}

type ChunkAckPayload struct {
	V          int    `json:"v"`
	FileID     string `json:"file_id"`
	ChunkIndex uint32 `json:"chunk_index"`
}

type FileDonePayload struct {
	V      int    `json:"v"`
	FileID string `json:"file_id"`
	Status string `json:"status"` // ok, checksum_mismatch, rejected
}

type FileRejectPayload struct {
	V      int    `json:"v"`
	FileID string `json:"file_id"`
	Reason string `json:"reason"`
}

type RoomMetaPayload struct {
	V        int      `json:"v"`
	Name     string   `json:"name"`
	Gen      int64    `json:"gen"`
	MasterID string   `json:"master_id"`
	Members  []string `json:"members"`
}

type RoomJoinPayload struct {
	V        int    `json:"v"`
	Name     string `json:"name"`
	Gen      int64  `json:"gen"`
	PeerID   string `json:"peer_id"`
	TS       int64  `json:"ts"`
	Sig      string `json:"sig"`
}

func (p *RoomJoinPayload) SigBytes() []byte {
	return []byte(fmt.Sprintf("JOIN:%s:%d:%s:%d", p.Name, p.Gen, p.PeerID, p.TS))
}

type RoomLeavePayload struct {
	V        int    `json:"v"`
	Name     string `json:"name"`
	Gen      int64  `json:"gen"`
	PeerID   string `json:"peer_id"`
	TS       int64  `json:"ts"`
	Sig      string `json:"sig"`
}

func (p *RoomLeavePayload) SigBytes() []byte {
	return []byte(fmt.Sprintf("LEAVE:%s:%d:%s:%d", p.Name, p.Gen, p.PeerID, p.TS))
}

type RoomClosedPayload struct {
	V        int    `json:"v"`
	Name     string `json:"name"`
	Gen      int64  `json:"gen"`
	MasterID string `json:"master_id"`
	TS       int64  `json:"ts"`
	Sig      string `json:"sig"`
}

func (p *RoomClosedPayload) SigBytes() []byte {
	return []byte(fmt.Sprintf("CLOSED:%s:%d:%s:%d", p.Name, p.Gen, p.MasterID, p.TS))
}

type RoomTransferPayload struct {
	V          int    `json:"v"`
	Name       string `json:"name"`
	Gen        int64  `json:"gen"`
	OldMaster  string `json:"old_master"`
	NewMaster  string `json:"new_master"`
	TS         int64  `json:"ts"`
	Sig        string `json:"sig"`
}

func (p *RoomTransferPayload) SigBytes() []byte {
	return []byte(fmt.Sprintf("TRANSFER:%s:%d:%s:%s:%d", p.Name, p.Gen, p.OldMaster, p.NewMaster, p.TS))
}

type RoomSyncReqPayload struct {
	V             int      `json:"v"`
	Room          string   `json:"room"`
	KnownMsgIDs   []string `json:"known_msg_ids"`
	SinceSeconds  int64    `json:"since_seconds"`
}

type RoomSyncResPayload struct {
	V        int          `json:"v"`
	Room     string       `json:"room"`
	Messages []MsgPayload `json:"messages"`
}

type FileAnnouncePayload struct {
	V         int    `json:"v"`
	Room      string `json:"room"`
	AnnounceNo int   `json:"announce_no"`
	FileID    string `json:"file_id"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
	SenderID  string `json:"sender_id"`
	Thumbnail string `json:"thumbnail,omitempty"` // Base64 thumbnail <= 8 KB
}

type FileWhoHasPayload struct {
	V      int    `json:"v"`
	Room   string `json:"room"`
	FileID string `json:"file_id"`
}

type FileHavePayload struct {
	V      int    `json:"v"`
	Room   string `json:"room"`
	FileID string `json:"file_id"`
	PeerID string `json:"peer_id"`
}

type AIQueryReqPayload struct {
	V           int    `json:"v"`
	ReqID       string `json:"req_id"`
	FromPeerID  string `json:"from_peer_id"`
	TargetModel string `json:"target_model,omitempty"`
	Prompt      string `json:"prompt"`
	Context     string `json:"context,omitempty"`
	TS          int64  `json:"ts"`
}

type AIQueryResPayload struct {
	V      int    `json:"v"`
	ReqID  string `json:"req_id"`
	Model  string `json:"model"`
	Answer string `json:"answer"`
	Error  string `json:"error,omitempty"`
}

type AISessionSyncPayload struct {
	V           int    `json:"v"`
	FromPeerID  string `json:"from_peer_id"`
	TargetConv  string `json:"target_conv"`
	Model       string `json:"model"`
	HistoryJSON string `json:"history_json"`
	GitDiff     string `json:"git_diff,omitempty"`
	TS          int64  `json:"ts"`
}

