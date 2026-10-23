package ui

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"kerykeion/pkg/config"
	"kerykeion/pkg/db"
	"kerykeion/pkg/discovery"
	"kerykeion/pkg/identity"
	"kerykeion/pkg/messaging"
	"kerykeion/pkg/room"
	"kerykeion/pkg/transfer"
	"kerykeion/pkg/transport"
)

func TestTUIRoomNewAndJoin(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerxt-ui-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg, _ := config.LoadConfig(tempDir)
	cfg.DisplayName = "Tester"
	cfg.TCPPort = 44100
	cfg.UDPPort = 44101

	idMgr, _ := identity.LoadOrCreate(cfg.ConfigDir, cfg.DisplayName)
	store, _ := db.Open(cfg.ConfigDir)
	defer store.Close()

	transMgr, _ := transport.New(idMgr, cfg.TCPPort)
	disc := discovery.New(idMgr, cfg.UDPPort, cfg.ConfigDir)
	msgSvc := messaging.New(idMgr, store, transMgr, disc, nil, nil)
	roomMgr := room.New(idMgr, store, disc, transMgr, nil)
	tferMgr := transfer.New(cfg, idMgr, store, disc, transMgr, nil, nil)

	appModel := NewTUI(Services{Config: cfg, Identity: idMgr, Store: store, Discovery: disc, Messaging: msgSvc, Rooms: roomMgr, Transfers: tferMgr}, "")

	// Test executeCommand /room new
	cmd := appModel.executeCommand("/room new testroom")
	if cmd == nil {
		t.Fatal("room creation must run as a command")
	}
	appModel.Update(cmd())
	if appModel.activeTarget != "testroom" {
		t.Errorf("expected activeTarget testroom, got %s", appModel.activeTarget)
	}
	if !appModel.isRoom {
		t.Errorf("expected isRoom true")
	}

	// Test JoinOrCreateRoom directly
	rec, joined, err := roomMgr.JoinOrCreateRoom("#ngobs", cfg.TCPPort)
	if err != nil || rec == nil {
		t.Fatalf("JoinOrCreateRoom failed: %v", err)
	}
	if joined {
		t.Errorf("expected room to be newly created (joined=false), got joined=true")
	}
	if rec.Name != "ngobs" {
		t.Errorf("expected normalized room name 'ngobs', got %q", rec.Name)
	}

	// Test FormatQuickHelp has full coverage
	helpStr := FormatQuickHelp()
	expectedCommands := []string{"/join", "/dm", "/who", "/room new", "/room list", "/room info", "/room transfer", "/send", "/get", "/leave", "/disband", "/doctor", "/name", "/open", "/export", "/add", "/quit", "kerxt"}
	for _, cmd := range expectedCommands {
		if !strings.Contains(helpStr, cmd) {
			t.Errorf("FormatQuickHelp missing command: %s", cmd)
		}
	}

	viewOutput := appModel.View()
	if len(viewOutput) == 0 {
		t.Errorf("view output should not be empty")
	}
}

// Run a real event loop: a synchronous Program.Send from Update deadlocks it.
type roomCreationModel struct {
	*AppModel
	completed chan string
}

func (m roomCreationModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.AppModel.Update(msg)
	if result, ok := msg.(MsgRoomJoined); ok {
		if result.Err != nil {
			m.completed <- result.Err.Error()
		} else {
			m.completed <- m.activeTarget
		}
		return m, tea.Quit
	}
	return m, cmd
}

func TestRoomCreationWithProgramCallback(t *testing.T) {
	cfg, err := config.LoadConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.LoadOrCreate(cfg.ConfigDir, "Tester")
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(cfg.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var program *tea.Program
	manager := room.New(id, store, nil, nil, func(event, name, detail string) {
		program.Send(MsgRoomEvent{Event: event, Room: name, Detail: detail})
	})
	defer manager.Stop()
	app := NewTUI(Services{Config: cfg, Identity: id, Store: store, Rooms: manager}, "")
	app.textInput.SetValue("/room new regression")
	completed := make(chan string, 1)
	program = tea.NewProgram(roomCreationModel{app, completed}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer())
	exited := make(chan error, 1)
	go func() { _, err := program.Run(); exited <- err }()
	defer program.Kill()
	program.Send(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case target := <-completed:
		if target != "regression" {
			t.Fatalf("unexpected active room: %s", target)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("room creation froze while delivering its callback")
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("program did not exit")
	}
	rec, err := store.GetRoom("regression")
	if err != nil || rec == nil || rec.State != "active" {
		t.Fatalf("room was not persisted: room=%+v err=%v", rec, err)
	}
}
