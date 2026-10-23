package transfer

import (
	"strings"
	"testing"

	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
)

func TestFileReferenceUsesHistoryAndRestrictsDMRecipients(t *testing.T) {
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.LoadOrCreate(cfg.ConfigDir, "sender")
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(cfg.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := New(cfg, id, store, discovery.New(id, 0, ""), nil, nil, nil)
	defer manager.Stop()
	fileID := strings.Repeat("1", 64)
	_, err = store.SaveMessage(db.MessageRecord{MsgID: "attachment", AuthorID: id.PeerID(), ConversationID: "recipient", FileRef: fileID, Body: `{"filename":"arrow-left3.svg","size":317}`})
	if err != nil {
		t.Fatal(err)
	}
	announce, ok := manager.announcementByID(fileID)
	if !ok || announce.Filename != "arrow-left3.svg" || announce.Room != "" {
		t.Fatal("file ID was not resolved from DM history")
	}
	if _, ok := manager.directFileForPeer(fileID, "recipient"); !ok {
		t.Fatal("original recipient was rejected")
	}
	if _, ok := manager.directFileForPeer(fileID, "unrelated-peer"); ok {
		t.Fatal("unrelated peer gained access to a DM attachment")
	}
}

func TestParseFileReferenceAcceptsFullHashAndAnnouncementNumber(t *testing.T) {
	id := strings.Repeat("1", 64)
	number, hash, err := ParseFileReference(id)
	if err != nil || hash != id || number != 0 {
		t.Fatal("numeric-looking file hash was interpreted as a number")
	}
	for _, reference := range []string{"3", "#3"} {
		number, hash, err = ParseFileReference(reference)
		if err != nil || number != 3 || hash != "" {
			t.Fatal("announcement number was rejected")
		}
	}
	for _, reference := range []string{"0", "-1", "invalid", strings.Repeat("a", 8)} {
		if _, _, err := ParseFileReference(reference); err == nil {
			t.Fatal("invalid reference accepted")
		}
	}
}
