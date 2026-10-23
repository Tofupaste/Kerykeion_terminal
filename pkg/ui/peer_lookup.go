package ui

import (
	"strings"

	"kerykeion/pkg/discovery"
)

func findPeer(disc *discovery.Discovery, peers []discovery.PeerInfo, target string) (*discovery.PeerInfo, bool) {
	if peer, ok := disc.FindPeerByName(target); ok {
		return peer, true
	}
	for _, peer := range peers {
		if strings.HasPrefix(peer.PeerID, target) {
			return &peer, true
		}
	}
	return nil, false
}
