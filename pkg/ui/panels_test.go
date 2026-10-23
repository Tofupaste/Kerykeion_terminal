package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
)

func newPanelTestModel(t *testing.T) *AppModel {
	t.Helper()
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg.Language = "id"
	id, err := identity.LoadOrCreate(cfg.ConfigDir, "Tester")
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(cfg.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	disc := discovery.New(id, cfg.UDPPort, "")
	manager := room.New(id, store, disc, nil, nil)
	t.Cleanup(manager.Stop)
	return NewTUI(Services{Config: cfg, Identity: id, Store: store, Discovery: disc, Rooms: manager}, "")
}

func TestConversationPanelsMinimizeAndRestore(t *testing.T) {
	m := newPanelTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	fullSystemHeight := m.systemViewport.Height
	if m.chatVisible || strings.Contains(m.View(), "Percakapan ·") {
		t.Fatal("conversation should be hidden before opening a room or DM")
	}
	cmd := m.executeCommand("/room new panel-test")
	m.Update(cmd())
	if !m.chatVisible || m.systemViewport.Height >= fullSystemHeight {
		t.Fatal("opening a room should split the system and conversation panels")
	}
	if strings.Contains(m.View(), "F4 Chat") {
		t.Fatal("F4 shortcut must be hidden while chat is visible")
	}
	m.addSystemLog("SYSTEM-ONLY")
	m.Update(MsgNewMessage(db.MessageRecord{
		MsgID: "one", ConversationID: "panel-test", AuthorID: m.idMgr.PeerID(),
		Body: "CHAT-ONLY", Timestamp: time.Now().Unix(), Status: "delivered",
	}))
	if strings.Contains(m.viewport.View(), "SYSTEM-ONLY") || strings.Contains(m.systemViewport.View(), "CHAT-ONLY") {
		t.Fatal("system logs and conversation messages must stay in separate viewports")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.chatVisible || m.activeTarget != "panel-test" || m.systemViewport.Height != fullSystemHeight {
		t.Fatal("Esc should expand the system panel and preserve the active room")
	}
	if !strings.Contains(m.View(), "F4 Chat") {
		t.Fatal("F4 shortcut must return when chat is minimized")
	}
	m.Update(MsgNewMessage(db.MessageRecord{
		MsgID: "two", ConversationID: "panel-test", AuthorID: "peer", Body: "WHILE-HIDDEN",
	}))
	if m.unread != 1 || m.chatVisible {
		t.Fatal("incoming messages should remain minimized and increment unread count")
	}
	if cmd := m.handleUserInput("do not send to hidden chat"); cmd != nil {
		t.Fatal("plain text must not send to a hidden conversation")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF4})
	if !m.chatVisible || m.unread != 0 || !strings.Contains(m.viewport.View(), "WHILE-HIDDEN") {
		t.Fatal("F4 should restore the conversation and its received messages")
	}
}

func TestVisibleChatIgnoresF4(t *testing.T) {
	m := newPanelTestModel(t)
	m.Update(m.executeCommand("/room new panel-test")())
	m.viewport.SetYOffset(0)
	for _, systemFocused := range []bool{false, true} {
		m.systemFocused = systemFocused
		m.Update(tea.KeyMsg{Type: tea.KeyF4})
		if !m.chatVisible || m.systemFocused != systemFocused || m.viewport.YOffset != 0 {
			t.Fatal("F4 must preserve the visible chat, focus, and scroll position")
		}
	}
}

func TestRoomPanelCloseAndExpansion(t *testing.T) {
	m := newPanelTestModel(t)
	m.Update(m.executeCommand("/room new panel-test")())
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.executeCommand("/go panel-test")
	if !m.chatVisible {
		t.Fatal("/go should reopen a minimized conversation")
	}
	m.Update(MsgCommandResult{ClearTarget: "panel-test"})
	if m.chatVisible || m.activeTarget != "" {
		t.Fatal("leaving the active room should close its panel")
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 32})
	m.activeTarget = "panel-test"
	m.showChat()
	splitWidth := m.systemViewport.Width
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.systemViewport.Width <= splitWidth {
		t.Fatal("minimizing a side-by-side chat should expand the system panel horizontally")
	}
}

func TestOpenDMAndPanelScrollFocus(t *testing.T) {
	m := newPanelTestModel(t)
	peerID := strings.Repeat("a", 64)
	m.peers = []discovery.PeerInfo{{PeerID: peerID, DisplayName: "Aria", IsOnline: true}}
	m.executeCommand("/dm aaaaaa")
	if !m.chatVisible || m.isRoom || m.activeTarget != peerID {
		t.Fatal("/dm should open a private conversation panel")
	}
	for i := 0; i < 50; i++ {
		m.addSystemLog(fmt.Sprintf("system log %d", i))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if !m.systemFocused {
		t.Fatal("F6 should move scrolling focus to the system panel")
	}
	chatOffset := m.viewport.YOffset
	systemOffset := m.systemViewport.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if m.systemViewport.YOffset >= systemOffset || m.viewport.YOffset != chatOffset {
		t.Fatal("Page Up should scroll only the focused panel")
	}
}

func TestInputFollowsPanelFocus(t *testing.T) {
	m := newPanelTestModel(t)
	m.messaging = messaging.New(m.idMgr, m.store, nil, m.discovery, nil, nil)
	m.Update(m.executeCommand("/room new input-focus")())
	for _, body := range []string{"/lang en", "/quit", "ordinary message"} {
		cmd := m.handleUserInput(body)
		if cmd == nil {
			t.Fatalf("chat input %q did not return a send command", body)
		}
		if result, ok := cmd().(MsgCommandResult); !ok || result.Err != nil {
			t.Fatalf("chat input %q was interpreted as a system command", body)
		}
	}
	messages, err := m.store.GetMessages("input-focus", 100)
	if err != nil || len(messages) != 3 {
		t.Fatalf("expected three chat messages: %+v, %v", messages, err)
	}
	if messages[0].Body != "/lang en" || messages[1].Body != "/quit" || m.cfg.Language != "id" {
		t.Fatal("slash-prefixed messages must be stored verbatim without system side effects")
	}
	m.textInput.SetValue("/lan")
	m.handleTabCompletion()
	if m.textInput.Value() != "/lan" {
		t.Fatal("chat input must not complete system commands")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyF6})
	if !strings.Contains(m.textInput.Placeholder, "perintah") {
		t.Fatal("system focus must show the command placeholder")
	}
	m.handleUserInput("/lang en")
	if m.cfg.Language != "en" {
		t.Fatal("system focus must execute slash commands")
	}
	if cmd := m.handleUserInput("do not send from system"); cmd != nil {
		t.Fatal("plain system input must not be sent to chat")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m.handleUserInput("/lang id")
	if m.cfg.Language != "id" {
		t.Fatal("commands must work when chat is minimized")
	}
}

func TestLanguageSwitchPersistsAndTranslates(t *testing.T) {
	m := newPanelTestModel(t)
	m.Update(tea.KeyMsg{Type: tea.KeyF2})
	if m.cfg.Language != "en" || !strings.Contains(m.View(), "System messages") {
		t.Fatal("F2 should switch the interface to English")
	}
	loaded, err := config.LoadConfig(m.cfg.ConfigDir)
	if err != nil || loaded.Language != "en" {
		t.Fatalf("language was not persisted: config=%+v err=%v", loaded, err)
	}
	m.executeCommand("/help room new")
	if !strings.Contains(m.systemLogs[len(m.systemLogs)-1], "Create a room") {
		t.Fatal("detailed help should be in English")
	}
	m.executeCommand("/lang invalid")
	if m.cfg.Language != "en" {
		t.Fatal("invalid language must preserve the selection")
	}
	m.executeCommand("/lang id")
	if !strings.Contains(m.View(), "Pesan sistem") {
		t.Fatal("/lang id should restore Indonesian labels")
	}
	for _, name := range []string{"join", "dm", "room transfer", "lang", "doctor"} {
		if strings.Contains(detailHelp("en", name), "Unknown command") {
			t.Fatalf("English help missing for %s", name)
		}
	}
}

func TestPanelsFitTerminalSizes(t *testing.T) {
	m := newPanelTestModel(t)
	for _, size := range [][2]int{{20, 5}, {32, 12}, {60, 20}, {80, 24}, {100, 30}, {120, 35}, {160, 40}} {
		for _, visible := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/chat=%v", size[0], size[1], visible), func(t *testing.T) {
				m.activeTarget = "room-with-long-name"
				m.isRoom = true
				m.chatVisible = visible
				m.messages = []db.MessageRecord{{AuthorID: "peer", Body: strings.Repeat("日本語-long-message-", 40)}}
				m.addSystemLog(strings.Repeat("/long/path/", 40))
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				view := m.View()
				if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
					t.Fatalf("view is %dx%d for terminal %dx%d", lipgloss.Width(view), lipgloss.Height(view), size[0], size[1])
				}
			})
		}
	}
}

func TestScrollIndicatorPositionAndProportion(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		height, total, offset int
		first, size           int
	}{
		{"fits", 10, 4, 0, 0, 10},
		{"empty", 10, 0, 0, 0, 10},
		{"top", 10, 50, 0, 0, 2},
		{"middle", 10, 50, 20, 4, 2},
		{"bottom", 10, 50, 40, 8, 2},
		{"very long history", 10, 10000, 9990, 9, 1},
		{"clamp offset", 10, 50, 100, 8, 2},
		{"single row", 1, 100, 50, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := strings.Split(scrollIndicator(tc.height, tc.total, tc.offset), "\n")
			if len(rows) != tc.height {
				t.Fatalf("indicator height = %d", len(rows))
			}
			assertScrollRows(t, rows, tc.first, tc.size)
		})
	}
}

func assertScrollRows(t *testing.T, rows []string, first, size int) {
	t.Helper()
	for row, glyph := range rows {
		want := "│"
		if row >= first && row < first+size {
			want = "┃"
		}
		if glyph != want {
			t.Fatalf("row %d: got %q, want %q", row, glyph, want)
		}
	}
}

func TestScrollIndicatorFollowsPageNavigation(t *testing.T) {
	m := newPanelTestModel(t)
	for i := 0; i < 100; i++ {
		m.addSystemLog(fmt.Sprintf("line %d", i))
	}
	bottom := viewportWithScrollbar(m.systemViewport)
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if viewportWithScrollbar(m.systemViewport) == bottom {
		t.Fatal("Page Up did not change the indicator and viewport")
	}
	rows := strings.Split(scrollIndicator(m.systemViewport.Height, m.systemViewport.TotalLineCount(), m.systemViewport.YOffset), "\n")
	if rows[len(rows)-1] != "│" {
		t.Fatal("indicator should move away from the bottom after Page Up")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if viewportWithScrollbar(m.systemViewport) != bottom {
		t.Fatal("Page Down should restore the bottom position")
	}
}
