package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/protocol"
	"kerykeion/pkg/transfer"
)

func TestSendPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WhatsApp Image 2026-10-19 at 20.41.48(1).jpeg")
	if err := os.WriteFile(path, []byte("photo"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{path, `"` + path + `"`, "'" + path + "'"} {
		parts := commandParts("/send " + raw)
		resolved, err := resolveAttachmentPath(parts[1])
		if err != nil || resolved != path {
			t.Fatalf("%q: %q, %v", raw, resolved, err)
		}
		if _, err := attachmentInfo(resolved, 100); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := attachmentInfo(path, 1); err == nil {
		t.Fatal("oversize file accepted")
	}
	if _, err := attachmentInfo(dir, 100); err == nil {
		t.Fatal("directory accepted")
	}
	home, _ := os.UserHomeDir()
	for _, path := range []string{"~", `"~"`} {
		resolved, err := resolveAttachmentPath(path)
		if err != nil || resolved != home {
			t.Fatalf("home: %q %v", resolved, err)
		}
	}
	if _, err := resolveAttachmentPath(`"unterminated`); err == nil {
		t.Fatal("unclosed quote accepted")
	}
}

func TestAttachmentPickerNavigationAndCancel(t *testing.T) {
	m := newPanelTestModel(t)
	m.activeTarget = "test-room"
	m.isRoom = true
	m.showChat()
	m.cfg.Language = "en"
	root := t.TempDir()
	folder := filepath.Join(root, "photos")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "A photo (1).jpeg")
	if err := os.WriteFile(file, []byte("photo"), 0600); err != nil {
		t.Fatal(err)
	}
	m.pickerDirectory = root
	m.textInput.SetValue("draft /keep")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m.Update(cmd())
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if m.picker.directory != folder {
		t.Fatal("directory did not open")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if m.picker.file != file || !strings.Contains(m.View(), "Enter Send") {
		t.Fatal("missing confirmation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker != nil || !m.chatVisible || m.textInput.Value() != "draft /keep" {
		t.Fatal("cancel changed chat or draft")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	stale := cmd()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.Update(stale)
	if m.picker != nil {
		t.Fatal("stale directory result reopened picker")
	}
	if m.pickerDirectory != folder {
		t.Fatal("last directory not remembered")
	}
	m.systemFocused = true
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if cmd != nil || m.picker != nil {
		t.Fatal("picker opened outside chat focus")
	}
}

func TestAttachmentPickerReadError(t *testing.T) {
	m := newPanelTestModel(t)
	m.activeTarget = "room"
	m.showChat()
	m.pickerDirectory = filepath.Join(t.TempDir(), "missing")
	cmd := m.openAttachmentPicker()
	m.Update(cmd())
	if m.picker.err == nil || m.picker.loading {
		t.Fatal("directory error not displayed")
	}
}

func TestAttachmentConfirmationSendsOnlyAfterSecondEnter(t *testing.T) {
	m := newPanelTestModel(t)
	m.activeTarget = "test-room"
	m.isRoom = true
	m.showChat()
	root := t.TempDir()
	path := filepath.Join(root, "photo with spaces.jpeg")
	if err := os.WriteFile(path, []byte("photo"), 0600); err != nil {
		t.Fatal(err)
	}
	var announcements []protocol.FileAnnouncePayload
	m.transfer = transfer.New(m.cfg, m.idMgr, m.store, m.discovery, nil, nil, func(a protocol.FileAnnouncePayload) { announcements = append(announcements, a) })
	m.pickerDirectory = root
	m.Update(m.openAttachmentPicker()())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(cmd())
	if len(announcements) != 0 {
		t.Fatal("file sent before confirmation")
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmation did not schedule send")
	}
	result := cmd().(MsgCommandResult)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(announcements) != 1 || announcements[0].Filename != filepath.Base(path) {
		t.Fatal("wrong attachment")
	}
	cmd = m.executeCommand("/send " + path)
	if result := cmd().(MsgCommandResult); result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(announcements) != 2 {
		t.Fatal("slash send failed")
	}
}
