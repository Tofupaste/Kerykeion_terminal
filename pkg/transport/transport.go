package transport

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"sync"
	"time"

	"kerykeion/pkg/identity"
	"kerykeion/pkg/protocol"
)

type FrameHandler func(connID string, frameType byte, payload []byte, remotePeerID string)

type ConnWrapper struct {
	net.Conn
	mu           sync.Mutex
	remotePeerID string
	connType     string // "chat" or "file"
	isOutbound   bool
	done         chan struct{}
	closeOnce    sync.Once
}

func (cw *ConnWrapper) Close() error {
	err := cw.Conn.Close()
	cw.closeOnce.Do(func() {
		if cw.done != nil {
			close(cw.done)
		}
	})
	return err
}

func (cw *ConnWrapper) Done() <-chan struct{} { return cw.done }

func (cw *ConnWrapper) WriteFrame(frameType byte, payload []byte) error {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	return protocol.WriteFrame(cw.Conn, frameType, payload)
}

func (cw *ConnWrapper) RemotePeerID() string {
	return cw.remotePeerID
}

type Manager struct {
	idMgr        *identity.Manager
	tcpPort      int
	tlsCert      tls.Certificate
	listener     net.Listener
	mu           sync.RWMutex
	chatConns    map[string]*ConnWrapper // peerID -> connection
	fileConns    map[string]*ConnWrapper // connID -> connection
	frameHandler FrameHandler
	ctx          context.Context
	cancelCtx    context.CancelFunc
	wg           sync.WaitGroup
}

func New(idMgr *identity.Manager, tcpPort int) (*Manager, error) {
	cert, err := generateSelfSignedCert(idMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to generate TLS certificate: %w", err)
	}

	return &Manager{
		idMgr:     idMgr,
		tcpPort:   tcpPort,
		tlsCert:   cert,
		chatConns: make(map[string]*ConnWrapper),
		fileConns: make(map[string]*ConnWrapper),
	}, nil
}

func generateSelfSignedCert(idMgr *identity.Manager) (tls.Certificate, error) {
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: idMgr.PeerID(),
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, idMgr.PublicKey(), idMgr.PrivateKey())
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  idMgr.PrivateKey(),
	}, nil
}

func (m *Manager) SetFrameHandler(handler FrameHandler) {
	m.frameHandler = handler
}

func (m *Manager) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancelCtx = cancel

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{m.tlsCert},
		MinVersion:         tls.VersionTLS13,
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			return nil
		},
	}

	var ln net.Listener
	var err error
	basePort := m.tcpPort
	for attempt := 0; attempt < 10; attempt++ {
		candidatePort := basePort + attempt
		ln, err = tls.Listen("tcp4", fmt.Sprintf(":%d", candidatePort), tlsConfig)
		if err == nil {
			m.tcpPort = ln.Addr().(*net.TCPAddr).Port
			break
		}
	}

	if err != nil {
		return fmt.Errorf("failed to listen on TCP port %d (and subsequent ports): %w", basePort, err)
	}
	m.listener = ln

	m.wg.Add(1)
	go m.acceptLoop()

	return nil
}

func (m *Manager) Port() int {
	return m.tcpPort
}

func (m *Manager) Stop() {
	if m.cancelCtx != nil {
		m.cancelCtx()
	}
	if m.listener != nil {
		_ = m.listener.Close()
	}

	m.mu.Lock()
	for _, c := range m.chatConns {
		_ = c.Close()
	}
	for _, c := range m.fileConns {
		_ = c.Close()
	}
	m.mu.Unlock()

	m.wg.Wait()
}

func (m *Manager) acceptLoop() {
	defer m.wg.Done()

	for {
		conn, err := m.listener.Accept()
		if err != nil {
			select {
			case <-m.ctx.Done():
				return
			default:
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}

		go m.handleInboundConn(conn)
	}
}

func (m *Manager) handleInboundConn(rawConn net.Conn) {
	tlsConn, ok := rawConn.(*tls.Conn)
	if !ok {
		_ = rawConn.Close()
		return
	}

	if err := tlsConn.Handshake(); err != nil {
		_ = rawConn.Close()
		return
	}

	var remotePeerID string
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) > 0 {
		peerCert := state.PeerCertificates[0]
		if pubKey, ok := peerCert.PublicKey.(ed25519.PublicKey); ok {
			remotePeerID = identity.Fingerprint(pubKey)
		}
	}

	cw := &ConnWrapper{
		done:         make(chan struct{}),
		Conn:         tlsConn,
		remotePeerID: remotePeerID,
		connType:     "chat",
	}

	m.readConn(cw)
}

func (m *Manager) DialPeer(targetPeerID, ip string, port int, connType string) (*ConnWrapper, error) {
	m.mu.RLock()
	if connType == "chat" {
		if existing, ok := m.chatConns[targetPeerID]; ok {
			m.mu.RUnlock()
			return existing, nil
		}
	}
	m.mu.RUnlock()

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{m.tlsCert},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("no peer certificates presented")
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return err
			}
			pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
			if !ok {
				return errors.New("peer certificate key is not ed25519")
			}
			fp := identity.Fingerprint(pubKey)
			if targetPeerID != "" && fp != targetPeerID {
				return fmt.Errorf("peer fingerprint mismatch: expected %s, got %s", targetPeerID, fp)
			}
			return nil
		},
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	addr := fmt.Sprintf("%s:%d", ip, port)
	rawConn, err := tls.DialWithDialer(dialer, "tcp4", addr, tlsConfig)
	if err != nil {
		return nil, err
	}

	cw := &ConnWrapper{
		done:         make(chan struct{}),
		Conn:         rawConn,
		remotePeerID: targetPeerID,
		connType:     connType,
		isOutbound:   true,
	}

	if connType == "chat" {
		m.mu.Lock()
		m.chatConns[targetPeerID] = cw
		m.mu.Unlock()
	}

	helloPayload, _ := protocol.MarshalPayload(protocol.HelloPayload{
		ConnType:    connType,
		V:           1,
		PeerID:      m.idMgr.PeerID(),
		DisplayName: m.idMgr.DisplayName(),
	})
	_ = cw.WriteFrame(protocol.TypeHello, helloPayload)

	go m.readConn(cw)
	return cw, nil
}

func (m *Manager) readConn(cw *ConnWrapper) {
	connID := fmt.Sprintf("%p", cw)
	m.mu.Lock()
	m.fileConns[connID] = cw
	m.mu.Unlock()

	defer m.removeConnection(connID, cw)

	for {
		frameType, payload, err := protocol.ReadFrame(cw.Conn)
		if err != nil {
			return
		}

		if frameType == protocol.TypeHello {
			m.handleHello(cw, payload)
			continue
		}

		if frameType == protocol.TypePing {
			_ = cw.WriteFrame(protocol.TypePing, []byte("PONG"))
			continue
		}

		if m.frameHandler != nil {
			m.frameHandler(connID, frameType, payload, cw.remotePeerID)
		}
	}
}

func (m *Manager) SendToPeer(peerID string, frameType byte, payload []byte) error {
	m.mu.RLock()
	conn, ok := m.chatConns[peerID]
	m.mu.RUnlock()

	if !ok || conn == nil {
		return errors.New("peer connection not active")
	}

	return conn.WriteFrame(frameType, payload)
}

func (m *Manager) removeConnection(connID string, conn *ConnWrapper) {
	_ = conn.Close()
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.fileConns, connID)
	if conn.connType != "chat" || conn.remotePeerID == "" {
		return
	}
	if current, exists := m.chatConns[conn.remotePeerID]; exists && current == conn {
		delete(m.chatConns, conn.remotePeerID)
	}
}

func (m *Manager) handleHello(conn *ConnWrapper, payload []byte) {
	var hello protocol.HelloPayload
	if err := protocol.UnmarshalPayload(payload, &hello); err != nil || hello.PeerID == "" {
		return
	}
	if conn.remotePeerID != "" && conn.remotePeerID != hello.PeerID {
		return
	}
	if conn.remotePeerID == "" {
		conn.remotePeerID = hello.PeerID
	}
	if !conn.isOutbound && hello.ConnType == "file" {
		conn.connType = "file"
	}
	m.mu.Lock()
	if conn.connType == "chat" {
		m.chatConns[hello.PeerID] = conn
	}
	m.mu.Unlock()
	if conn.isOutbound {
		return
	}
	reply, _ := protocol.MarshalPayload(protocol.HelloPayload{
		ConnType: conn.connType,
		V:        1, PeerID: m.idMgr.PeerID(), DisplayName: m.idMgr.DisplayName(),
	})
	_ = conn.WriteFrame(protocol.TypeHello, reply)
}

func (m *Manager) IsConnected(peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.chatConns[peerID]
	return ok
}

func (m *Manager) SendToConn(connID string, frameType byte, payload []byte) error {
	m.mu.RLock()
	cw, ok := m.fileConns[connID]
	if !ok {
		for _, c := range m.chatConns {
			if fmt.Sprintf("%p", c) == connID {
				cw = c
				ok = true
				break
			}
		}
	}
	m.mu.RUnlock()

	if !ok || cw == nil {
		return errors.New("connection not found")
	}
	return cw.WriteFrame(frameType, payload)
}

func (m *Manager) Connection(connID string) (*ConnWrapper, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.fileConns[connID]
	return conn, ok
}
