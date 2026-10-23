package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestREPLRoomLifecycle(t *testing.T) {
	model := newPanelTestModel(t)
	services := Services{Config: model.cfg, Identity: model.idMgr, Store: model.store, Discovery: model.discovery, Rooms: model.roomMgr}
	input := strings.NewReader("/room new repl-test\n/room list\n/room info repl-test\n/leave repl-test\n/quit\n")
	var output bytes.Buffer
	repl := NewREPL(services, input, &output, "")
	repl.Run()
	for _, text := range []string{"repl-test", "DAFTAR ROOM", "INFO ROOM", "Sampai jumpa"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("missing output %q: %s", text, output.String())
		}
	}
	room, err := model.store.GetRoom("repl-test")
	if err != nil || room.State != "left" || repl.running || repl.activeTarget != "" {
		t.Fatalf("room lifecycle did not complete: room=%+v err=%v", room, err)
	}
}

func TestREPLCommandValidation(t *testing.T) {
	model := newPanelTestModel(t)
	for _, command := range []string{"/dm", "/join", "/send", "/get invalid", "/room new", "/room transfer", "/name", "/open invalid", "/add", "/unrecognized"} {
		t.Run(command, func(t *testing.T) {
			var output bytes.Buffer
			repl := NewREPL(Services{Config: model.cfg, Identity: model.idMgr, Store: model.store, Discovery: model.discovery}, nil, &output, "")
			repl.executeCommand(command)
			if output.Len() == 0 || repl.activeTarget != "" {
				t.Fatalf("invalid command did not produce help: %q", output.String())
			}
		})
	}
}
