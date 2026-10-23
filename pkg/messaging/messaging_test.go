package messaging

import (
	"testing"

	"kerykeion/pkg/db"
	"kerykeion/pkg/protocol"
)

func TestRoomAcknowledgmentIsMatchedToRecipient(t *testing.T) {
	store, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bob, carol := make(chan bool, 1), make(chan bool, 1)
	service := &Service{store: store, pendingAcks: map[deliveryKey]chan bool{
		{msgID: "room-message", peerID: "bob"}:   bob,
		{msgID: "room-message", peerID: "carol"}: carol,
	}}
	payload, err := protocol.MarshalPayload(protocol.MsgAckPayload{V: 1, MsgID: "room-message"})
	if err != nil {
		t.Fatal(err)
	}
	service.HandleIncomingMsgAck(payload, "bob")
	if len(bob) != 1 || len(carol) != 0 {
		t.Fatal("Bob's ACK was routed to another member")
	}
	service.HandleIncomingMsgAck(payload, "carol")
	if len(carol) != 1 {
		t.Fatal("Carol's ACK was lost")
	}
}
