package ui

import (
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
