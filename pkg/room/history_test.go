package room

import (
	"testing"
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
)

func TestHistoryValidatesRoomAndAuthor(t *testing.T) {
	dir := t.TempDir()
	id, err := identity.LoadOrCreate(dir, "reader")
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager := New(id, store, discovery.New(id, 0, ""), nil, nil)
	defer manager.Stop()
	author, err := identity.LoadOrCreate(t.TempDir(), "author")
	if err != nil {
		t.Fatal(err)
	}
	msg := protocol.MsgPayload{V: 1, MsgID: "signed-history", AuthorID: author.PeerID(), Room: "project", Body: "Original text", TS: time.Now().Unix()}
	msg.Sig = author.SignBase64(msg.SigBytes())
	if manager.saveHistoryMessage("other-room", msg, author.PublicKeyBase64()) {
		t.Fatal("cross-room history accepted")
	}
	tampered := msg
	tampered.Body = "Tampered text"
	if manager.saveHistoryMessage("project", tampered, author.PublicKeyBase64()) {
		t.Fatal("tampered history accepted")
	}
	if manager.saveHistoryMessage("project", msg, id.PublicKeyBase64()) {
		t.Fatal("incorrect author key accepted")
	}
	if !manager.saveHistoryMessage("project", msg, author.PublicKeyBase64()) {
		t.Fatal("valid history rejected")
	}
	if manager.saveHistoryMessage("project", msg, author.PublicKeyBase64()) {
		t.Fatal("duplicate history inserted")
	}
	if manager.peerPublicKey(author.PeerID()) != author.PublicKeyBase64() {
		t.Fatal("verified author key not retained")
	}
}
