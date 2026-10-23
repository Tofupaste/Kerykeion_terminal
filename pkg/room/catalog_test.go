package room

import (
	"testing"
	"time"
)

func TestAvailableRoomsDeduplicatesHostsAndExpires(t *testing.T) {
	now := time.Now()
	manager := &Manager{available: map[string]AvailableRoom{
		"one-old":    {Name: "one", HostID: "old", LastSeen: now.Add(-AvailableRoomTTL - time.Second)},
		"one-active": {Name: "one", HostID: "active", LastSeen: now},
		"two-old":    {Name: "two", HostID: "old", LastSeen: now.Add(-AvailableRoomTTL - time.Second)},
		"one-other":  {Name: "one", HostID: "other", LastSeen: now.Add(-time.Second)},
	}}
	rooms := manager.availableRoomsAt(now)
	if len(rooms) != 1 || rooms[0].Name != "one" || rooms[0].HostID != "active" {
		t.Fatalf("unexpected catalog: %+v", rooms)
	}
	if len(manager.available) != 2 {
		t.Fatal("expired hosts were not removed")
	}
	if rooms := manager.availableRoomsAt(now.Add(AvailableRoomTTL + time.Second)); len(rooms) != 0 {
		t.Fatal("offline rooms remained available")
	}
}
