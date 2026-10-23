package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.Mutex
}

type MessageRecord struct {
	MsgID          string    `json:"msg_id"`
	ConversationID string    `json:"conversation_id"`
	AuthorID       string    `json:"author_id"`
	Body           string    `json:"body"`
	Timestamp      int64     `json:"ts"`
	Signature      string    `json:"sig"`
	Status         string    `json:"status"` // queued, sent, delivered, failed
	FileRef        string    `json:"file_ref,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type PeerRecord struct {
	PeerID      string    `json:"peer_id"`
	DisplayName string    `json:"display_name"`
	PubKey      string    `json:"pubkey"`
	LastIP      string    `json:"last_ip"`
	LastPort    int       `json:"last_port"`
	LastSeen    time.Time `json:"last_seen"`
	Trusted     bool      `json:"trusted"`
}

type RoomRecord struct {
	Name         string    `json:"name"`
	Gen          int64     `json:"gen"`
	MasterID     string    `json:"master_id"`
	MyRole       string    `json:"my_role"` // master, guest, none
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
	State        string    `json:"state"` // active, disbanded, left
}

type OutboxItem struct {
	MsgID        string    `json:"msg_id"`
	TargetPeerID string    `json:"target_peer_id"`
	Attempts     int       `json:"attempts"`
	NextRetryAt  time.Time `json:"next_retry_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TransferRecord struct {
	FileID    string    `json:"file_id"`
	PeerID    string    `json:"peer_id"`
	Direction string    `json:"direction"` // upload, download
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	State     string    `json:"state"` // pending, transferring, completed, failed, paused
	LocalPath string    `json:"local_path"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MediaCacheRecord struct {
	FileID    string    `json:"file_id"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	ExpiresAt time.Time `json:"expires_at"`
}

func Open(configDir string) (*Store, error) {
	dbPath := filepath.Join(configDir, "history.db")
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=auto_vacuum(INCREMENTAL)&_pragma=foreign_keys(ON)", dbPath)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS identity (
			peer_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS peers (
			peer_id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			pubkey TEXT NOT NULL,
			last_ip TEXT NOT NULL,
			last_port INTEGER NOT NULL,
			last_seen DATETIME NOT NULL,
			trusted INTEGER DEFAULT 0
		);`,

		`CREATE TABLE IF NOT EXISTS rooms (
			name TEXT NOT NULL,
			gen INTEGER NOT NULL,
			master_id TEXT NOT NULL,
			my_role TEXT NOT NULL DEFAULT 'guest',
			created_at DATETIME NOT NULL,
			last_activity DATETIME NOT NULL,
			state TEXT NOT NULL DEFAULT 'active',
			PRIMARY KEY (name, gen)
		);`,

		`CREATE TABLE IF NOT EXISTS room_members (
			room TEXT NOT NULL,
			peer_id TEXT NOT NULL,
			joined_at DATETIME NOT NULL,
			left_at DATETIME,
			PRIMARY KEY (room, peer_id)
		);`,

		`CREATE TABLE IF NOT EXISTS room_tombstones (
			name TEXT NOT NULL,
			gen INTEGER NOT NULL,
			closed_at DATETIME NOT NULL,
			PRIMARY KEY (name, gen)
		);`,

		`CREATE TABLE IF NOT EXISTS messages (
			msg_id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL,
			author_id TEXT NOT NULL,
			body TEXT NOT NULL,
			ts INTEGER NOT NULL,
			sig TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			file_ref TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conv_ts ON messages(conversation_id, ts);`,

		`CREATE TABLE IF NOT EXISTS outbox (
			msg_id TEXT NOT NULL,
			target_peer_id TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			next_retry_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			PRIMARY KEY (msg_id, target_peer_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_retry ON outbox(next_retry_at);`,

		`CREATE TABLE IF NOT EXISTS transfers (
			file_id TEXT NOT NULL,
			peer_id TEXT NOT NULL,
			direction TEXT NOT NULL,
			filename TEXT NOT NULL,
			size INTEGER NOT NULL,
			state TEXT NOT NULL,
			local_path TEXT NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (file_id, peer_id, direction)
		);`,

		`CREATE TABLE IF NOT EXISTS media_cache (
			file_id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			size INTEGER NOT NULL,
			expires_at DATETIME NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("error running migration (%s): %w", q, err)
		}
	}

	return nil
}

func (s *Store) PruneRetention(retentionDays int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoffSeconds := time.Now().AddDate(0, 0, -retentionDays).Unix()
	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)

	// Prune messages older than retentionDays
	if _, err := s.db.Exec(`DELETE FROM messages WHERE ts < ?`, cutoffSeconds); err != nil {
		return err
	}

	// Prune expired outbox messages
	if _, err := s.db.Exec(`DELETE FROM outbox WHERE expires_at < ?`, time.Now()); err != nil {
		return err
	}

	// Prune expired tombstones
	if _, err := s.db.Exec(`DELETE FROM room_tombstones WHERE closed_at < ?`, cutoffDate); err != nil {
		return err
	}

	// Prune inactive rooms
	if _, err := s.db.Exec(`DELETE FROM rooms WHERE last_activity < ? AND state != 'active'`, cutoffDate); err != nil {
		return err
	}

	// Prune expired media cache entries
	if _, err := s.db.Exec(`DELETE FROM media_cache WHERE expires_at < ?`, time.Now()); err != nil {
		return err
	}

	// Run incremental vacuum
	_, _ = s.db.Exec(`PRAGMA incremental_vacuum;`)
	return nil
}

// Identity operations
func (s *Store) SaveIdentity(peerID, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO identity (peer_id, display_name, created_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET display_name = excluded.display_name`,
		peerID, displayName, time.Now(),
	)
	return err
}

func (s *Store) GetIdentity() (peerID, displayName string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`SELECT peer_id, display_name FROM identity LIMIT 1`)
	err = row.Scan(&peerID, &displayName)
	return
}

// Peer operations
func (s *Store) UpsertPeer(p PeerRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO peers (peer_id, display_name, pubkey, last_ip, last_port, last_seen, trusted)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(peer_id) DO UPDATE SET
		   display_name = excluded.display_name,
		   pubkey = excluded.pubkey,
		   last_ip = excluded.last_ip,
		   last_port = excluded.last_port,
		   last_seen = excluded.last_seen`,
		p.PeerID, p.DisplayName, p.PubKey, p.LastIP, p.LastPort, p.LastSeen, p.Trusted,
	)
	return err
}

func (s *Store) GetAllPeers() ([]PeerRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT peer_id, display_name, pubkey, last_ip, last_port, last_seen, trusted FROM peers ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []PeerRecord
	for rows.Next() {
		var p PeerRecord
		if err := rows.Scan(&p.PeerID, &p.DisplayName, &p.PubKey, &p.LastIP, &p.LastPort, &p.LastSeen, &p.Trusted); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, nil
}

func (s *Store) GetPeer(peerID string) (*PeerRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`SELECT peer_id, display_name, pubkey, last_ip, last_port, last_seen, trusted FROM peers WHERE peer_id = ?`, peerID)
	var p PeerRecord
	if err := row.Scan(&p.PeerID, &p.DisplayName, &p.PubKey, &p.LastIP, &p.LastPort, &p.LastSeen, &p.Trusted); err != nil {
		return nil, err
	}
	return &p, nil
}

// Message operations
func (s *Store) SaveMessage(m MessageRecord) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}

	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO messages (msg_id, conversation_id, author_id, body, ts, sig, status, file_ref, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.MsgID, m.ConversationID, m.AuthorID, m.Body, m.Timestamp, m.Signature, m.Status, m.FileRef, m.CreatedAt,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) UpdateMessageStatus(msgID, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE messages SET status = ? WHERE msg_id = ?`, status, msgID)
	return err
}

func (s *Store) GetMessages(conversationID string, limit int) ([]MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.Query(
		`SELECT * FROM (SELECT msg_id, conversation_id, author_id, body, ts, sig, status, COALESCE(file_ref, ''), created_at
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY ts DESC, created_at DESC
		 LIMIT ?) ORDER BY ts ASC, created_at ASC`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []MessageRecord
	for rows.Next() {
		var m MessageRecord
		if err := rows.Scan(&m.MsgID, &m.ConversationID, &m.AuthorID, &m.Body, &m.Timestamp, &m.Signature, &m.Status, &m.FileRef, &m.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, nil
}

func (s *Store) GetRecentMessageIDs(conversationID string, sinceSeconds int64) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT msg_id FROM messages WHERE conversation_id = ? AND ts >= ? ORDER BY ts ASC`,
		conversationID, sinceSeconds,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) GetMessage(msgID string) (*MessageRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT msg_id, conversation_id, author_id, body, ts, sig, status, COALESCE(file_ref, ''), created_at
		 FROM messages WHERE msg_id = ?`,
		msgID,
	)
	var m MessageRecord
	if err := row.Scan(&m.MsgID, &m.ConversationID, &m.AuthorID, &m.Body, &m.Timestamp, &m.Signature, &m.Status, &m.FileRef, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// Outbox operations
func (s *Store) AddToOutbox(item OutboxItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO outbox (msg_id, target_peer_id, attempts, next_retry_at, expires_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(msg_id, target_peer_id) DO UPDATE SET
		   attempts = excluded.attempts,
		   next_retry_at = excluded.next_retry_at`,
		item.MsgID, item.TargetPeerID, item.Attempts, item.NextRetryAt, item.ExpiresAt,
	)
	return err
}

func (s *Store) RemoveFromOutbox(msgID, targetPeerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM outbox WHERE msg_id = ? AND target_peer_id = ?`, msgID, targetPeerID)
	return err
}

func (s *Store) GetPendingOutboxItems(now time.Time) ([]OutboxItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(
		`SELECT msg_id, target_peer_id, attempts, next_retry_at, expires_at
		 FROM outbox
		 WHERE next_retry_at <= ? AND expires_at > ?
		 ORDER BY next_retry_at ASC`,
		now, now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OutboxItem
	for rows.Next() {
		var it OutboxItem
		if err := rows.Scan(&it.MsgID, &it.TargetPeerID, &it.Attempts, &it.NextRetryAt, &it.ExpiresAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, nil
}

// Room operations
func (s *Store) UpsertRoom(r RoomRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO rooms (name, gen, master_id, my_role, created_at, last_activity, state)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name, gen) DO UPDATE SET
		   master_id = excluded.master_id,
		   my_role = excluded.my_role,
		   last_activity = excluded.last_activity,
		   state = excluded.state`,
		r.Name, r.Gen, r.MasterID, r.MyRole, r.CreatedAt, r.LastActivity, r.State,
	)
	return err
}

func (s *Store) GetRoom(name string) (*RoomRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT name, gen, master_id, my_role, created_at, last_activity, state
		 FROM rooms WHERE name = ? ORDER BY gen DESC LIMIT 1`,
		name,
	)
	var r RoomRecord
	if err := row.Scan(&r.Name, &r.Gen, &r.MasterID, &r.MyRole, &r.CreatedAt, &r.LastActivity, &r.State); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetAllRooms() ([]RoomRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT name, gen, master_id, my_role, created_at, last_activity, state FROM rooms ORDER BY last_activity DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RoomRecord
	for rows.Next() {
		var r RoomRecord
		if err := rows.Scan(&r.Name, &r.Gen, &r.MasterID, &r.MyRole, &r.CreatedAt, &r.LastActivity, &r.State); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) AddRoomMember(room, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO room_members (room, peer_id, joined_at, left_at)
		 VALUES (?, ?, ?, NULL)
		 ON CONFLICT(room, peer_id) DO UPDATE SET left_at = NULL`,
		room, peerID, time.Now(),
	)
	return err
}

func (s *Store) RemoveRoomMember(room, peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`UPDATE room_members SET left_at = ? WHERE room = ? AND peer_id = ?`, time.Now(), room, peerID)
	return err
}

func (s *Store) GetRoomMembers(room string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`SELECT peer_id FROM room_members WHERE room = ? AND left_at IS NULL`, room)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, nil
}

func (s *Store) AddTombstone(name string, gen int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO room_tombstones (name, gen, closed_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(name, gen) DO UPDATE SET closed_at = excluded.closed_at`,
		name, gen, time.Now(),
	)
	return err
}

func (s *Store) HasTombstone(name string, gen int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var count int
	row := s.db.QueryRow(`SELECT count(*) FROM room_tombstones WHERE name = ? AND gen = ?`, name, gen)
	_ = row.Scan(&count)
	return count > 0
}

// Transfer operations
func (s *Store) UpsertTransfer(t TransferRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t.UpdatedAt.IsZero() {
		t.UpdatedAt = time.Now()
	}

	_, err := s.db.Exec(
		`INSERT INTO transfers (file_id, peer_id, direction, filename, size, state, local_path, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(file_id, peer_id, direction) DO UPDATE SET
		   state = excluded.state,
		   local_path = excluded.local_path,
		   updated_at = excluded.updated_at`,
		t.FileID, t.PeerID, t.Direction, t.Filename, t.Size, t.State, t.LocalPath, t.UpdatedAt,
	)
	return err
}

func (s *Store) GetTransfer(fileID, peerID, direction string) (*TransferRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT file_id, peer_id, direction, filename, size, state, local_path, updated_at
		 FROM transfers WHERE file_id = ? AND peer_id = ? AND direction = ?`,
		fileID, peerID, direction,
	)
	var t TransferRecord
	if err := row.Scan(&t.FileID, &t.PeerID, &t.Direction, &t.Filename, &t.Size, &t.State, &t.LocalPath, &t.UpdatedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

// Media cache operations
func (s *Store) AddMediaCache(rec MediaCacheRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO media_cache (file_id, path, size, expires_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(file_id) DO UPDATE SET
		   path = excluded.path,
		   size = excluded.size,
		   expires_at = excluded.expires_at`,
		rec.FileID, rec.Path, rec.Size, rec.ExpiresAt,
	)
	return err
}

func (s *Store) GetMediaCache(fileID string) (*MediaCacheRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(
		`SELECT file_id, path, size, expires_at FROM media_cache WHERE file_id = ? AND expires_at > ?`,
		fileID, time.Now(),
	)
	var rec MediaCacheRecord
	if err := row.Scan(&rec.FileID, &rec.Path, &rec.Size, &rec.ExpiresAt); err != nil {
		return nil, err
	}
	return &rec, nil
}
