package ui

import (
	"kerykeion/pkg/transfer"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kerykeion/pkg/db"
	"kerykeion/pkg/protocol"
)

func TestAttachmentsAppearInChatWithFileIDAndGetNumber(t *testing.T) {
	m := newPanelTestModel(t)
	m.cfg.Language = "en"
	m.activeTarget = "room-test"
	m.isRoom = true
	m.showChat()
	id := strings.Repeat("a", 64)
	message := db.MessageRecord{MsgID: "attachment", ConversationID: m.activeTarget, AuthorID: m.idMgr.PeerID(), Body: `{"filename":"project notes.txt","size":42}`, FileRef: id, Timestamp: time.Now().Unix()}
	m.Update(MsgNewMessage(message))
	m.Update(MsgAnnounce(protocol.FileAnnouncePayload{Room: m.activeTarget, FileID: id, SenderID: message.AuthorID, AnnounceNo: 3, Filename: "project notes.txt", Size: 42}))
	rendered := m.viewport.View()
	for _, expected := range []string{"project notes.txt", "ID:", "/get 3"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("attachment missing %q: %s", expected, rendered)
		}
	}
	if !strings.Contains(m.messageBody(message), id) {
		t.Fatal("file ID was truncated in chat message")
	}
	m.activeTarget = "dm-peer"
	m.isRoom = false
	message.ConversationID = m.activeTarget
	m.messages = nil
	m.Update(MsgNewMessage(message))
	if !strings.Contains(m.viewport.View(), "project notes.txt") || strings.Contains(m.messageBody(message), "/get 3") {
		t.Fatal("DM attachment was missing or used a room get number")
	}
}

func TestCopiedFileIDCanBeUsedByGetCommand(t *testing.T) {
	m := newPanelTestModel(t)
	m.cfg.Language = "en"
	path := filepath.Join(t.TempDir(), "arrow-left3.svg")
	if err := os.WriteFile(path, []byte("svg content"), 0600); err != nil {
		t.Fatal(err)
	}
	fileID, size, err := transfer.ComputeFileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.store.SaveMessage(db.MessageRecord{MsgID: "file-message", ConversationID: "recipient", AuthorID: m.idMgr.PeerID(), FileRef: fileID, Body: `{"filename":"arrow-left3.svg","size":11}`})
	if err != nil {
		t.Fatal(err)
	}
	err = m.store.UpsertTransfer(db.TransferRecord{FileID: fileID, PeerID: m.idMgr.PeerID(), Direction: "download", State: "completed", Filename: "arrow-left3.svg", LocalPath: path, Size: size})
	if err != nil {
		t.Fatal(err)
	}
	m.transfer = transfer.New(m.cfg, m.idMgr, m.store, m.discovery, nil, nil, nil)
	defer m.transfer.Stop()
	command := m.executeCommand("/get " + fileID)
	if command == nil {
		t.Fatal("copied file ID was rejected before download")
	}
	if result := command().(MsgCommandResult); result.Err != nil {
		t.Fatal(result.Err)
	}
}
