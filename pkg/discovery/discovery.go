package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
)

type PeerInfo struct {
	PeerID      string
	DisplayName string
	PubKey      string
	IP          string
	Port        int
	UDPPort     int
	LastSeen    time.Time
	IsOnline    bool
}

type Discovery struct {
	idMgr      *identity.Manager
	udpPort    int
	officeKey  string
	conn       *net.UDPConn
	seqCounter atomic.Uint64
	peersMu    sync.RWMutex
	peers      map[string]*PeerInfo
	peerSeqs   map[string]uint64
	cancelCtx  context.CancelFunc
	ctx        context.Context
	wg         sync.WaitGroup

	onPeerDiscovered func(p PeerInfo)
	onPeerUpdated    func(p PeerInfo)
	onRoomQuery      func(q protocol.RoomQueryPacket, addr *net.UDPAddr)
	onRoomHere       func(h protocol.RoomHerePacket, addr *net.UDPAddr)
}

func New(idMgr *identity.Manager, udpPort int, officeKey string) *Discovery {
	return &Discovery{
		idMgr:     idMgr,
		udpPort:   udpPort,
		officeKey: officeKey,
		peers:     make(map[string]*PeerInfo),
		peerSeqs:  make(map[string]uint64),
	}
}

func (d *Discovery) SetCallbacks(
	onDiscovered func(p PeerInfo),
	onUpdated func(p PeerInfo),
	onRoomQuery func(q protocol.RoomQueryPacket, addr *net.UDPAddr),
	onRoomHere func(h protocol.RoomHerePacket, addr *net.UDPAddr),
) {
	d.onPeerDiscovered = onDiscovered
	d.onPeerUpdated = onUpdated
	d.onRoomQuery = onRoomQuery
	d.onRoomHere = onRoomHere
}

func (d *Discovery) Start(tcpPort int) error {
	ctx, cancel := context.WithCancel(context.Background())
	d.ctx = ctx
	d.cancelCtx = cancel

	addr := &net.UDPAddr{Port: d.udpPort, IP: net.IPv4zero}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("failed to bind UDP port %d: %w", d.udpPort, err)
	}
	d.conn = conn

	d.wg.Add(3)
	go d.readLoop()
	go d.announceLoop(tcpPort)
	go d.interfaceWatcherLoop(tcpPort)

	return nil
}

func (d *Discovery) Stop() {
	if d.cancelCtx != nil {
		d.cancelCtx()
	}
	if d.conn != nil {
		_ = d.conn.Close()
	}
	d.wg.Wait()
}

func (d *Discovery) Port() int {
	if d.conn != nil {
		return d.conn.LocalAddr().(*net.UDPAddr).Port
	}
	return d.udpPort
}

// SendAnnounceTo introduces this identity at an explicit discovery address.
func (d *Discovery) SendAnnounceTo(addr *net.UDPAddr, tcpPort int) {
	d.sendAnnounceTo(addr, tcpPort)
}

func (d *Discovery) AddManualPeer(ipPort string) (*PeerInfo, error) {
	host, portStr, err := net.SplitHostPort(ipPort)
	if err != nil {
		return nil, fmt.Errorf("invalid ip:port: %w", err)
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	udpAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", host, d.udpPort))
	if err == nil {
		d.sendAnnounceTo(udpAddr, port)
	}

	return nil, nil
}

func (d *Discovery) GetPeers() []PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	now := time.Now()
	var list []PeerInfo
	for _, p := range d.peers {
		cp := *p
		cp.IsOnline = now.Sub(cp.LastSeen) <= 15*time.Second
		list = append(list, cp)
	}
	return list
}

func (d *Discovery) GetPeer(peerID string) (*PeerInfo, bool) {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	p, ok := d.peers[peerID]
	if !ok {
		return nil, false
	}
	cp := *p
	cp.IsOnline = time.Since(cp.LastSeen) <= 15*time.Second
	return &cp, true
}

func (d *Discovery) peerUDPPort(peer *PeerInfo) int {
	if peer.UDPPort != 0 {
		return peer.UDPPort
	}
	return d.udpPort
}

func (d *Discovery) FindPeerByName(name string) (*PeerInfo, bool) {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	var matches []*PeerInfo
	for _, p := range d.peers {
		if p.DisplayName == name {
			matches = append(matches, p)
		}
	}
	if len(matches) == 1 {
		cp := *matches[0]
		cp.IsOnline = time.Since(cp.LastSeen) <= 15*time.Second
		return &cp, true
	}
	return nil, false
}

func (d *Discovery) announceLoop(tcpPort int) {
	defer d.wg.Done()

	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	d.broadcastAnnounce(tcpPort)

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.broadcastAnnounce(tcpPort)
		}
	}
}

func (d *Discovery) interfaceWatcherLoop(tcpPort int) {
	defer d.wg.Done()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastAddrs := d.getInterfaceAddrs()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			currentAddrs := d.getInterfaceAddrs()
			if !equalStrings(lastAddrs, currentAddrs) {
				lastAddrs = currentAddrs
				d.broadcastAnnounce(tcpPort)
			}
		}
	}
}

func (d *Discovery) getInterfaceAddrs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var res []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			res = append(res, a.String())
		}
	}
	return res
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (d *Discovery) broadcastAnnounce(tcpPort int) {
	targets := d.calculateBroadcastTargets()
	for _, target := range targets {
		d.sendAnnounceTo(target, tcpPort)
	}

	d.peersMu.RLock()
	var cachedAddrs []*net.UDPAddr
	for _, p := range d.peers {
		if addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", p.IP, d.peerUDPPort(p))); err == nil {
			cachedAddrs = append(cachedAddrs, addr)
		}
	}
	d.peersMu.RUnlock()

	for _, addr := range cachedAddrs {
		d.sendAnnounceTo(addr, tcpPort)
	}
}

func (d *Discovery) sendAnnounceTo(addr *net.UDPAddr, tcpPort int) {
	if d.conn == nil {
		return
	}
	pkt := protocol.AnnouncePacket{
		V:         1,
		Type:      "ANNOUNCE",
		PubKey:    d.idMgr.PublicKeyBase64(),
		Name:      d.idMgr.DisplayName(),
		Port:      tcpPort,
		Seq:       d.seqCounter.Add(1),
		Timestamp: time.Now().Unix(),
		OfficeKey: d.officeKey,
	}
	pkt.Sig = d.idMgr.SignBase64(pkt.SigBytes())

	data, err := json.Marshal(pkt)
	if err != nil {
		return
	}

	_, _ = d.conn.WriteToUDP(data, addr)
}

func (d *Discovery) calculateBroadcastTargets() []*net.UDPAddr {
	var targets []*net.UDPAddr
	ifaces, err := net.Interfaces()
	if err != nil {
		return targets
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		targets = append(targets, broadcastAddresses(addrs, d.udpPort)...)
	}

	targets = append(targets, &net.UDPAddr{IP: net.IPv4(239, 255, 42, 101), Port: d.udpPort})
	return targets
}

func broadcastAddresses(addrs []net.Addr, port int) []*net.UDPAddr {
	var targets []*net.UDPAddr
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok || ipnet.IP.To4() == nil || len(ipnet.Mask) != 4 {
			continue
		}
		ip, mask := ipnet.IP.To4(), ipnet.Mask
		broadcast := net.IPv4(ip[0]|^mask[0], ip[1]|^mask[1], ip[2]|^mask[2], ip[3]|^mask[3])
		targets = append(targets, &net.UDPAddr{IP: broadcast, Port: port})
	}
	return targets
}

func (d *Discovery) readLoop() {
	defer d.wg.Done()

	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-d.ctx.Done():
				return
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		d.handleIncomingPacket(buf[:n], remoteAddr)
	}
}

func (d *Discovery) handleIncomingPacket(data []byte, remoteAddr *net.UDPAddr) {
	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		return
	}

	msgType, _ := generic["type"].(string)
	switch msgType {
	case "ANNOUNCE":
		var pkt protocol.AnnouncePacket
		if err := json.Unmarshal(data, &pkt); err != nil {
			return
		}
		d.handleAnnounce(pkt, remoteAddr)

	case "ROOM_QUERY":
		var pkt protocol.RoomQueryPacket
		if err := json.Unmarshal(data, &pkt); err != nil {
			return
		}
		if d.onRoomQuery != nil {
			d.onRoomQuery(pkt, remoteAddr)
		}

	case "ROOM_HERE":
		var pkt protocol.RoomHerePacket
		if err := json.Unmarshal(data, &pkt); err != nil {
			return
		}
		if d.onRoomHere != nil {
			d.onRoomHere(pkt, remoteAddr)
		}
	}
}

func (d *Discovery) announcePeerID(pkt protocol.AnnouncePacket) (string, bool) {
	if d.officeKey != "" && pkt.OfficeKey != d.officeKey {
		return "", false
	}

	now := time.Now().Unix()
	if pkt.Timestamp < now-60 || pkt.Timestamp > now+60 {
		return "", false
	}

	if !identity.VerifyBase64(pkt.PubKey, pkt.SigBytes(), pkt.Sig) {
		return "", false
	}

	pubBytes, err := identity.DecodeBase64PubKey(pkt.PubKey)
	if err != nil {
		return "", false
	}

	senderPeerID := identity.Fingerprint(pubBytes)
	if senderPeerID == d.idMgr.PeerID() {
		return "", false
	}
	return senderPeerID, true
}

func (d *Discovery) handleAnnounce(pkt protocol.AnnouncePacket, remoteAddr *net.UDPAddr) {
	senderPeerID, valid := d.announcePeerID(pkt)
	if !valid {
		return
	}

	remoteIP := remoteAddr.IP.String()

	d.peersMu.Lock()
	lastSeq, exists := d.peerSeqs[senderPeerID]
	if exists && pkt.Seq <= lastSeq && pkt.Timestamp <= time.Now().Unix()-15 {
		d.peersMu.Unlock()
		return
	}
	d.peerSeqs[senderPeerID] = pkt.Seq

	existing, found := d.peers[senderPeerID]
	info := PeerInfo{
		PeerID:      senderPeerID,
		DisplayName: pkt.Name,
		PubKey:      pkt.PubKey,
		IP:          remoteIP,
		Port:        pkt.Port,
		UDPPort:     remoteAddr.Port,
		LastSeen:    time.Now(),
		IsOnline:    true,
	}

	ipChanged := false
	if found {
		if existing.IP != remoteIP || existing.Port != pkt.Port {
			ipChanged = true
		}
		existing.IP = remoteIP
		existing.Port = pkt.Port
		existing.UDPPort = remoteAddr.Port
		existing.DisplayName = pkt.Name
		existing.LastSeen = time.Now()
		existing.IsOnline = true
	} else {
		d.peers[senderPeerID] = &info
	}
	d.peersMu.Unlock()

	if !found && d.onPeerDiscovered != nil {
		d.onPeerDiscovered(info)
	} else if ipChanged && d.onPeerUpdated != nil {
		d.onPeerUpdated(info)
	}
}

func (d *Discovery) BroadcastRoomQuery(roomName string, myTCPPort int) {
	if d.conn == nil {
		return
	}
	pkt := protocol.RoomQueryPacket{
		V:        1,
		Type:     "ROOM_QUERY",
		RoomName: roomName,
		FromPeer: d.idMgr.PeerID(),
		FromPort: myTCPPort,
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return
	}

	targets := d.calculateBroadcastTargets()
	for _, target := range targets {
		_, _ = d.conn.WriteToUDP(data, target)
	}

	d.peersMu.RLock()
	var cachedAddrs []*net.UDPAddr
	for _, p := range d.peers {
		if addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", p.IP, d.peerUDPPort(p))); err == nil {
			cachedAddrs = append(cachedAddrs, addr)
		}
	}
	d.peersMu.RUnlock()

	for _, addr := range cachedAddrs {
		_, _ = d.conn.WriteToUDP(data, addr)
	}
}

func (d *Discovery) SendRoomHere(toAddr *net.UDPAddr, roomName string, gen int64, masterID string, myTCPPort int) {
	if d.conn == nil {
		return
	}
	pkt := protocol.RoomHerePacket{
		V:        1,
		Type:     "ROOM_HERE",
		RoomName: roomName,
		Gen:      gen,
		MasterID: masterID,
		FromPeer: d.idMgr.PeerID(),
		FromPort: myTCPPort,
	}
	data, err := json.Marshal(pkt)
	if err != nil {
		return
	}

	_, _ = d.conn.WriteToUDP(data, toAddr)
}
