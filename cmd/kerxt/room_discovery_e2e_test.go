package main

import "testing"

func TestE2EDiscoversRoomWithoutKnowingItsName(t *testing.T) {
	alice, bob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Bob")
	connectRoomTestPeers(t, alice, bob)
	if _, err := alice.rooms.CreateRoom("unexpected-room"); err != nil {
		t.Fatal(err)
	}
	awaitRoomCondition(t, "automatic room search", func() bool {
		rooms := bob.rooms.AvailableRooms()
		return len(rooms) == 1 && rooms[0].Name == "unexpected-room"
	})
	records, err := bob.store.GetAllRooms()
	if err != nil || len(records) != 0 {
		t.Fatal("discovery joined or created a room")
	}
	if bob.transport.IsConnected(alice.id.PeerID()) {
		t.Fatal("catalog search opened a chat connection")
	}
}
