package db

import (
	"os"
	"testing"
	"time"
)

func TestDatabaseOperationsAndPruning(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerxt-test-db-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := Open(tempDir)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	// 1. Identity
	if err := store.SaveIdentity("peer123", "Alice"); err != nil {
		t.Fatalf("failed to save identity: %v", err)
	}
	pID, dName, err := store.GetIdentity()
	if err != nil || pID != "peer123" || dName != "Alice" {
		t.Errorf("unexpected identity: %s, %s, err: %v", pID, dName, err)
	}

	// 2. Messages & Deduplication
	msg1 := MessageRecord{
		MsgID:          "0192b450-7000-7000-8000-000000000001",
		ConversationID: "ngobs",
		AuthorID:       "peer123",
		Body:           "Halo semua",
		Timestamp:      time.Now().Unix(),
		Signature:      "sig123",
		Status:         "queued",
		CreatedAt:      time.Now(),
	}

	inserted, err := store.SaveMessage(msg1)
	if err != nil || !inserted {
		t.Fatalf("expected message to be inserted, got inserted=%v, err=%v", inserted, err)
	}

	// Insert duplicate message with same msg_id
	insertedAgain, err := store.SaveMessage(msg1)
	if err != nil || insertedAgain {
		t.Fatalf("expected duplicate message to be ignored, got inserted=%v, err=%v", insertedAgain, err)
	}

	// 3. Update Message Status
	if err := store.UpdateMessageStatus(msg1.MsgID, "delivered"); err != nil {
		t.Fatalf("failed to update message status: %v", err)
	}
	retrieved, err := store.GetMessage(msg1.MsgID)
	if err != nil || retrieved.Status != "delivered" {
		t.Errorf("expected status 'delivered', got '%s'", retrieved.Status)
	}

	// 4. Room Tombstones
	_ = store.AddTombstone("proyek-x", 12345)
	if !store.HasTombstone("proyek-x", 12345) {
		t.Errorf("expected tombstone for proyek-x gen 12345 to exist")
	}
	if store.HasTombstone("proyek-x", 99999) {
		t.Errorf("tombstone for proyek-x gen 99999 should not exist")
	}

	// 5. 30-Day Retention Pruning
	oldMsg := MessageRecord{
		MsgID:          "0192b450-7000-7000-8000-000000000002",
		ConversationID: "ngobs",
		AuthorID:       "peer123",
		Body:           "Pesan lama",
		Timestamp:      time.Now().AddDate(0, 0, -35).Unix(), // 35 days ago
		Signature:      "sig_old",
		Status:         "delivered",
		CreatedAt:      time.Now().AddDate(0, 0, -35),
	}
	_, _ = store.SaveMessage(oldMsg)

	if err := store.PruneRetention(30); err != nil {
		t.Fatalf("failed to prune retention: %v", err)
	}

	msgs, err := store.GetMessages("ngobs", 10)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	for _, m := range msgs {
		if m.MsgID == oldMsg.MsgID {
			t.Errorf("old message older than 30 days was not pruned!")
		}
	}
}
