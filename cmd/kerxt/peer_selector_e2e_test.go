package main

import (
	"bytes"
	"strings"
	"testing"

	"kerykeion/pkg/ui"
)

func TestREPLDMWithDuplicatePeerNames(t *testing.T) {
	alice, bob, otherBob := newRoomTestPeer(t, "Alice"), newRoomTestPeer(t, "Budi"), newRoomTestPeer(t, "Budi")
	connectRoomTestPeers(t, alice, bob, otherBob)
	input := "/who\n/dm Budi\n/dm Budi+" + bob.id.PeerID() + "\nOnly the selected Budi should get this\n/quit\n"
	var output bytes.Buffer
	repl := ui.NewREPL(ui.Services{Config: alice.config, Identity: alice.id, Store: alice.store, Discovery: alice.discovery, Messaging: alice.messages, Rooms: alice.rooms, Transfers: alice.transfers}, strings.NewReader(input), &output, "")
	repl.Run()
	if !strings.Contains(output.String(), bob.id.PeerID()) || !strings.Contains(output.String(), otherBob.id.PeerID()) || !strings.Contains(output.String(), "matches multiple users") {
		t.Fatal("/who or duplicate-name choices omitted IDs")
	}
	awaitRoomCondition(t, "DM to selected duplicate-name peer", func() bool {
		messages, err := bob.store.GetMessages(alice.id.PeerID(), 10)
		return err == nil && len(messages) == 1 && messages[0].Body == "Only the selected Budi should get this"
	})
	messages, err := otherBob.store.GetMessages(alice.id.PeerID(), 10)
	if err != nil || len(messages) != 0 {
		t.Fatal("DM reached the other user with the same name")
	}
}
