package ui

import (
	"sort"
	"strings"

	"kerykeion/pkg/discovery"
)

func peerIDPrefix(id string, peers []discovery.PeerInfo) string {
	length := min(8, len(id))
	for length < len(id) && prefixCollides(id, id[:length], peers) {
		length++
	}
	return id[:length]
}

func prefixCollides(id, prefix string, peers []discovery.PeerInfo) bool {
	for _, peer := range peers {
		if peer.PeerID != id && strings.HasPrefix(peer.PeerID, prefix) {
			return true
		}
	}
	return false
}

func peerLabel(peer discovery.PeerInfo, peers []discovery.PeerInfo) string {
	return peer.DisplayName + "#" + peerIDPrefix(peer.PeerID, peers)
}

func peerMatches(peer discovery.PeerInfo, target string) bool {
	if peer.DisplayName == target || strings.HasPrefix(peer.PeerID, strings.ToLower(target)) {
		return true
	}
	split := strings.LastIndexAny(target, "#+")
	if split < 1 || split == len(target)-1 {
		return false
	}
	return peer.DisplayName == target[:split] && strings.HasPrefix(peer.PeerID, strings.ToLower(target[split+1:]))
}

func resolvePeer(peers []discovery.PeerInfo, target string) (*discovery.PeerInfo, []discovery.PeerInfo) {
	target = strings.TrimSpace(target)
	if len(target) > 1 && (target[0] == '"' || target[0] == '\'') && target[len(target)-1] == target[0] {
		target = target[1 : len(target)-1]
	}
	if target == "" {
		return nil, nil
	}
	var matches []discovery.PeerInfo
	seen := make(map[string]bool)
	for _, peer := range peers {
		if strings.EqualFold(peer.PeerID, target) {
			return &peer, nil
		}
		if !seen[peer.PeerID] && peerMatches(peer, target) {
			matches = append(matches, peer)
			seen[peer.PeerID] = true
		}
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].PeerID < matches[j].PeerID })
	return nil, matches
}

func peerLookupMessage(language, target string, matches, peers []discovery.PeerInfo) string {
	if len(matches) == 0 {
		return localized(language, "Rekan '%s' tidak ditemukan. Ketik /who untuk melihat daftar.", target)
	}
	lines := []string{localized(language, "Rekan '%s' cocok dengan beberapa pengguna. Gunakan ID atau nama#ID:", target)}
	for _, peer := range matches {
		lines = append(lines, "  /dm "+peerLabel(peer, peers))
	}
	return strings.Join(lines, "\n")
}

func (m *AppModel) lookupPeer(target string) (*discovery.PeerInfo, bool) {
	if peers := m.discovery.GetPeers(); len(peers) > 0 {
		m.peers = peers
	}
	peer, matches := resolvePeer(m.peers, target)
	if peer == nil {
		m.addSystemLog(peerLookupMessage(m.cfg.Language, target, matches, m.peers))
		return nil, false
	}
	return peer, true
}

func (r *REPL) lookupPeer(target string) (*discovery.PeerInfo, bool) {
	peers := r.discovery.GetPeers()
	peer, matches := resolvePeer(peers, target)
	if peer == nil {
		r.Printf("%s\n", peerLookupMessage(r.cfg.Language, target, matches, peers))
		return nil, false
	}
	return peer, true
}
