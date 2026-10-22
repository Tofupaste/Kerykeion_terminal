package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Manager struct {
	mu          sync.RWMutex
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	peerID      string
	displayName string
	keyPath     string
}

func Fingerprint(pub ed25519.PublicKey) string {
	h := sha256.Sum256(pub)
	return hex.EncodeToString(h[:])
}

func ShortID(peerID string) string {
	if len(peerID) <= 4 {
		return peerID
	}
	return peerID[:4]
}

func FormatDisplayName(name, peerID string, hasDuplicate bool) string {
	if name == "" {
		name = "anon"
	}
	if hasDuplicate {
		return fmt.Sprintf("%s#%s", name, ShortID(peerID))
	}
	return name
}

func LoadOrCreate(configDir, displayName string) (*Manager, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}

	keyPath := filepath.Join(configDir, "identity.key")
	var privKey ed25519.PrivateKey

	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == ed25519.PrivateKeySize {
		privKey = ed25519.PrivateKey(data)
	} else {
		_, newPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
		}
		if err := os.WriteFile(keyPath, newPriv, 0600); err != nil {
			return nil, fmt.Errorf("failed to save identity key: %w", err)
		}
		privKey = newPriv
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	peerID := Fingerprint(pubKey)

	trimmedName := strings.TrimSpace(displayName)
	if trimmedName == "" {
		trimmedName = "user"
	}

	return &Manager{
		privateKey:  privKey,
		publicKey:   pubKey,
		peerID:      peerID,
		displayName: trimmedName,
		keyPath:     keyPath,
	}, nil
}

func (m *Manager) PeerID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.peerID
}

func (m *Manager) PublicKey() ed25519.PublicKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.publicKey
}

func (m *Manager) PrivateKey() ed25519.PrivateKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.privateKey
}

func (m *Manager) PublicKeyBase64() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return base64.StdEncoding.EncodeToString(m.publicKey)
}

func (m *Manager) DisplayName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.displayName
}

func (m *Manager) SetDisplayName(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		m.displayName = trimmed
	}
}

func (m *Manager) Sign(message []byte) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return ed25519.Sign(m.privateKey, message)
}

func (m *Manager) SignBase64(message []byte) string {
	return base64.StdEncoding.EncodeToString(m.Sign(message))
}

func Verify(pub ed25519.PublicKey, message, signature []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(signature) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, message, signature)
}

func DecodeBase64PubKey(pubBase64 string) (ed25519.PublicKey, error) {
	data, err := base64.StdEncoding.DecodeString(pubBase64)
	if err != nil {
		return nil, err
	}
	if len(data) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key size: %d", len(data))
	}
	return ed25519.PublicKey(data), nil
}

func VerifyBase64(pubBase64 string, message []byte, sigBase64 string) bool {
	pub, err := DecodeBase64PubKey(pubBase64)
	if err != nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return false
	}
	return Verify(pub, message, sig)
}

