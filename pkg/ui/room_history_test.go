package ui

import (
	"strings"
	"testing"
	"time"

	"kerykeion/pkg/db"
)

func TestRoomHistorySyncRefreshesVisibleChat(t *testing.T) {
	m := newPanelTestModel(t)
	m.activeTarget = "history-test"
	m.isRoom = true
	m.showChat()
	m.textInput.SetValue("unfinished draft")
	_, err := m.store.SaveMessage(db.MessageRecord{MsgID: "history-1", ConversationID: "history-test", AuthorID: m.idMgr.PeerID(), Body: "A message from room history", Timestamp: time.Now().Unix(), Status: "delivered"})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(MsgRoomEvent{Event: "ROOM_HISTORY_SYNCED", Room: "different-room"})
	if len(m.messages) != 0 {
		t.Fatal("unrelated history changed active chat")
	}
	m.Update(MsgRoomEvent{Event: "ROOM_HISTORY_SYNCED", Room: "history-test"})
	if len(m.messages) != 1 || !strings.Contains(m.View(), "A message from room history") {
		t.Fatal("history not displayed")
	}
	if m.textInput.Value() != "unfinished draft" || !m.chatVisible {
		t.Fatal("history refresh disturbed chat")
	}
}
