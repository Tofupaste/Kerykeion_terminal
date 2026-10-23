package ui

import (
	"strings"
	"testing"
)

func TestAvailableRoomListIsActionableAndLocalized(t *testing.T) {
	text := availableRoomList("en", []string{"lounge", "project-a"})
	for _, expected := range []string{"3 seconds", "/join lounge", "/join project-a"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("room list missing %q", expected)
		}
	}
	if !strings.Contains(availableRoomList("id", nil), "Tidak ada room") {
		t.Fatal("empty room list was not localized")
	}
}

func TestRoomCatalogTickPreservesDraftAndCopyView(t *testing.T) {
	m := newPanelTestModel(t)
	m.textInput.SetValue("keep this draft")
	m.openCopyIDs()
	before := m.View()
	_, cmd := m.Update(roomCatalogTick{})
	if cmd == nil {
		t.Fatal("next catalog refresh was not scheduled")
	}
	if m.View() != before || m.textInput.Value() != "keep this draft" {
		t.Fatal("periodic refresh changed copy view or draft")
	}
}
