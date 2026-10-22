package ui

import (
	"os"
	"strings"
	"testing"

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

	appModel := NewTUI(cfg, idMgr, store, disc, msgSvc, roomMgr, tferMgr, "")

	// Test executeCommand /room new
	appModel.executeCommand("/room new testroom")
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
