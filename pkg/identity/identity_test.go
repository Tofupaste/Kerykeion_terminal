package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityGenerationAndPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerxt-test-id-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr1, err := LoadOrCreate(tempDir, "Alice")
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	peerID1 := mgr1.PeerID()
	if len(peerID1) != 64 {
		t.Errorf("expected peerID length 64 (sha256 hex), got %d: %s", len(peerID1), peerID1)
	}

	keyPath := filepath.Join(tempDir, "identity.key")
	fi, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key file does not exist: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions on identity.key, got: %o", fi.Mode().Perm())
	}

	// Reload same directory, must get same peer_id
	mgr2, err := LoadOrCreate(tempDir, "Alice Reloaded")
	if err != nil {
		t.Fatalf("failed to reload identity: %v", err)
	}

	if mgr2.PeerID() != peerID1 {
		t.Errorf("expected persistent peer_id %s, got %s", peerID1, mgr2.PeerID())
	}
}

func TestSigningAndVerification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kerxt-test-sig-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	mgr, err := LoadOrCreate(tempDir, "Signer")
	if err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	payload := []byte("hello office lan")
	sig := mgr.Sign(payload)

	if !Verify(mgr.PublicKey(), payload, sig) {
		t.Errorf("signature verification failed for valid message")
	}

	tampered := []byte("hello office wan")
	if Verify(mgr.PublicKey(), tampered, sig) {
		t.Errorf("signature verification succeeded for tampered message")
	}

	sigBase64 := mgr.SignBase64(payload)
	if !VerifyBase64(mgr.PublicKeyBase64(), payload, sigBase64) {
		t.Errorf("base64 verification failed for valid message")
	}
}

func TestDisplayNameFormatting(t *testing.T) {
	peerID := "1234567890abcdef"
	formattedNoDup := FormatDisplayName("Budi", peerID, false)
	if formattedNoDup != "Budi" {
		t.Errorf("expected 'Budi', got '%s'", formattedNoDup)
	}

	formattedDup := FormatDisplayName("Budi", peerID, true)
	if formattedDup != "Budi#12345" {
		t.Errorf("expected 'Budi#12345', got '%s'", formattedDup)
	}
}
