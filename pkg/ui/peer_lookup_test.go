package ui

import (
	"strings"
	"testing"

	"kerykeion/pkg/discovery"
)

func lookupTestPeers() []discovery.PeerInfo {
	return []discovery.PeerInfo{
		{PeerID: "abcdef001" + strings.Repeat("1", 55), DisplayName: "Budi"},
		{PeerID: "abcdef002" + strings.Repeat("2", 55), DisplayName: "Budi"},
		{PeerID: strings.Repeat("c", 64), DisplayName: "Aria Putra"},
	}
}

func TestResolvePeerSelectors(t *testing.T) {
	peers := lookupTestPeers()
	for _, target := range []string{peers[0].PeerID, "abcdef001", "Budi#abcdef001", "Budi+abcdef001", "Budi#ABCDEF001"} {
		peer, matches := resolvePeer(peers, target)
		if peer == nil || peer.PeerID != peers[0].PeerID || len(matches) != 0 {
			t.Fatalf("selector %q failed", target)
		}
	}
	for _, target := range []string{"Budi", "abcdef00", "Budi#abcdef00"} {
		peer, matches := resolvePeer(peers, target)
		if peer != nil || len(matches) != 2 {
			t.Fatalf("ambiguous %q selected a peer", target)
		}
	}
}

func TestResolvePeerInvalidAndSpacedNames(t *testing.T) {
	peers := lookupTestPeers()
	for _, target := range []string{"", "missing", "Aria#abcdef001", "Budi#", "Budi+"} {
		peer, matches := resolvePeer(peers, target)
		if peer != nil || len(matches) != 0 {
			t.Fatalf("invalid selector %q matched", target)
		}
	}
	for _, target := range []string{"Aria Putra", `"Aria Putra"`, "Aria Putra#cccccccc"} {
		peer, _ := resolvePeer(peers, target)
		if peer == nil || peer.PeerID != peers[2].PeerID {
			t.Fatalf("spaced username %q failed", target)
		}
	}
}

func TestPeerLabelsExtendCollidingIDPrefixes(t *testing.T) {
	peers := lookupTestPeers()
	if peerLabel(peers[0], peers) != "Budi#abcdef001" || peerLabel(peers[1], peers) != "Budi#abcdef002" {
		t.Fatal("labels did not extend colliding prefixes")
	}
	if peerLabel(peers[2], peers) != "Aria Putra#cccccccc" {
		t.Fatal("unexpected default prefix")
	}
}

func TestDMRejectsAmbiguousNamesAndPrefixes(t *testing.T) {
	m := newPanelTestModel(t)
	m.cfg.Language = "en"
	m.peers = lookupTestPeers()
	m.activeTarget = "existing-room"
	m.isRoom = true
	for _, target := range []string{"Budi", "abcdef00"} {
		m.executeCommand("/dm " + target)
		if m.activeTarget != "existing-room" || !m.isRoom {
			t.Fatal("ambiguous DM changed the active target")
		}
		log := m.systemLogs[len(m.systemLogs)-1]
		if !strings.Contains(log, "Budi#abcdef001") || !strings.Contains(log, "Budi#abcdef002") {
			t.Fatal("ambiguity message omitted unique choices")
		}
	}
	m.executeCommand("/dm Budi#abcdef002")
	if m.activeTarget != m.peers[1].PeerID || m.isRoom || !m.chatVisible {
		t.Fatal("qualified DM did not open correct peer")
	}
	m.executeCommand("/dm Aria Putra")
	if m.activeTarget != m.peers[2].PeerID {
		t.Fatal("spaced name was split")
	}
	m.executeCommand("/go Budi+abcdef001")
	if m.activeTarget != m.peers[0].PeerID {
		t.Fatal("/go did not use the shared selector")
	}
}
