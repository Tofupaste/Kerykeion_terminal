package transfer

import (
	"strings"
	"testing"

	"kerykeion/pkg/config"
	"kerykeion/pkg/protocol"
)

func TestAnnouncementNumbersAreLocalAndDeduplicated(t *testing.T) {
	manager := &Manager{announcements: make(map[int]protocol.FileAnnouncePayload), nextAnnounceNo: 1}
	alice := protocol.FileAnnouncePayload{AnnounceNo: 1, Room: "project", FileID: strings.Repeat("a", 64), SenderID: "alice"}
	bob := protocol.FileAnnouncePayload{AnnounceNo: 1, Room: "project", FileID: strings.Repeat("b", 64), SenderID: "bob"}
	first, fresh := manager.registerAnnouncement(alice)
	if !fresh {
		t.Fatal("first announcement ignored")
	}
	second, fresh := manager.registerAnnouncement(bob)
	if !fresh || second.AnnounceNo == first.AnnounceNo {
		t.Fatal("another sender overwrote announcement number")
	}
	repeated, fresh := manager.registerAnnouncement(alice)
	if fresh || repeated.AnnounceNo != first.AnnounceNo || len(manager.announcements) != 2 {
		t.Fatal("duplicate announcement created a new number")
	}
}

func TestRejectInvalidFileOfferBeforeCreatingPartial(t *testing.T) {
	manager := &Manager{cfg: &config.Config{MaxFileSize: 1024}}
	valid := protocol.FileOfferPayload{FileID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), Size: 1, ChunkSize: protocol.ChunkSize, TotalChunks: 1}
	if err := manager.validateOffer("conn", valid); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*protocol.FileOfferPayload){
		func(offer *protocol.FileOfferPayload) { offer.FileID = "../../outside" },
		func(offer *protocol.FileOfferPayload) { offer.Size = -1 },
		func(offer *protocol.FileOfferPayload) { offer.Size = 1025 },
		func(offer *protocol.FileOfferPayload) { offer.TotalChunks = 0 },
		func(offer *protocol.FileOfferPayload) { offer.ChunkSize = 1 },
	} {
		offer := valid
		modify(&offer)
		if manager.validateOffer("conn", offer) == nil {
			t.Fatalf("invalid offer accepted: %+v", offer)
		}
	}
}
