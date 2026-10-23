package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/db"
)

func TestCopyIDsStaysStillDuringIncomingMessages(t *testing.T) {
	m := newPanelTestModel(t)
	m.cfg.Language = "en"
	m.peers = lookupTestPeers()
	m.activeTarget = m.peers[0].PeerID
	m.showChat()
	m.textInput.SetValue("keep this draft")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	before := m.View()
	m.Update(MsgNewMessage(db.MessageRecord{MsgID: "incoming", ConversationID: m.activeTarget, Body: "new message", AuthorID: m.activeTarget, Timestamp: time.Now().Unix()}))
	m.Update(MsgProgress{Filename: "file.txt", TotalBytes: 100, SentBytes: 30})
	if m.View() != before {
		t.Fatal("copy view changed during background updates")
	}
	if len(m.messages) != 1 {
		t.Fatal("incoming message was lost")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.copyIDs != nil || !m.chatVisible || m.systemFocused || m.textInput.Value() != "keep this draft" {
		t.Fatal("cancel changed draft or focus")
	}
}

func TestCopyIDWritesFullValueAndReportsFailure(t *testing.T) {
	m := newPanelTestModel(t)
	m.cfg.Language = "en"
	m.peers = lookupTestPeers()
	m.activeTarget = m.peers[0].PeerID
	m.showChat()
	m.openCopyIDs()
	selected := m.copyIDs.items[m.copyIDs.selected]
	generation := m.copyIDs.generation
	written := ""
	command := copyIDCommand(selected, generation, func(value string) error { written = value; return nil })
	m.Update(command())
	if written != m.activeTarget || len(written) != 64 || m.copyIDs != nil || !strings.Contains(m.statusText, "ID copied") {
		t.Fatal("full ID was not copied")
	}
	m.openCopyIDs()
	generation = m.copyIDs.generation
	command = copyIDCommand(selected, generation, func(string) error { return errors.New("clipboard missing") })
	m.Update(command())
	if m.copyIDs == nil || !strings.Contains(m.View(), "Clipboard unavailable") {
		t.Fatal("clipboard failure was hidden")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.openCopyIDs()
	m.Update(clipboardResult{generation: generation, label: "stale"})
	if m.copyIDs == nil {
		t.Fatal("stale copy result closed the new list")
	}
}

func TestCopyIDsIncludesAttachmentHash(t *testing.T) {
	m := newPanelTestModel(t)
	id := strings.Repeat("f", 64)
	m.messages = []db.MessageRecord{{FileRef: id, Body: `{"filename":"notes.txt","size":42}`}, {FileRef: id}}
	m.openCopyIDs()
	count := 0
	for _, item := range m.copyIDs.items {
		if item.value == id {
			count++
		}
	}
	if count != 1 {
		t.Fatal("file ID missing or duplicated")
	}
}
