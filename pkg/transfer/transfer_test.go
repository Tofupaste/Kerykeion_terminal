package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal.pdf", "normal.pdf"},
		{"../../../etc/passwd", "passwd"},
		{"C:\\Windows\\System32\\cmd.exe", "cmd.exe"},
		{"CON.txt", "CON_file.txt"},
		{"NUL", "NUL_file"},
		{"AUX.dat", "AUX_file.dat"},
		{"", "downloaded_file"},
		{"  ..  ", "downloaded_file"},
	}

	for _, tc := range tests {
		actual := SanitizeFilename(tc.input)
		if actual != tc.expected {
			t.Errorf("SanitizeFilename(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}

func TestDangerousExtension(t *testing.T) {
	if !IsDangerous("setup.exe") {
		t.Errorf("expected setup.exe to be marked dangerous")
	}
	if !IsDangerous("script.sh") {
		t.Errorf("expected script.sh to be marked dangerous")
	}
	if !IsDangerous("run.bat") {
		t.Errorf("expected run.bat to be marked dangerous")
	}
	if IsDangerous("document.pdf") {
		t.Errorf("document.pdf should not be marked dangerous")
	}
}

func TestFileHashComputation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kantorchat-test-hash-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "sample.txt")
	content := []byte("KantorChat P2P office LAN file sharing test")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	shaHex, size, err := ComputeFileSHA256(filePath)
	if err != nil {
		t.Fatalf("ComputeFileSHA256 failed: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	if len(shaHex) != 64 {
		t.Errorf("expected sha256 hex length 64, got %d (%s)", len(shaHex), shaHex)
	}
}

func TestResumeWithDiskBitmap(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kantorchat-test-resume-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	partialDir := filepath.Join(tempDir, ".partial")
	_ = os.MkdirAll(partialDir, 0700)

	fileID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	metaPath := filepath.Join(partialDir, fileID+".meta")

	// Total 4 chunks (0, 1, 2, 3)
	// Suppose chunks 0 and 1 are already completed (bits 0 and 1 are 0), chunks 2 and 3 needed (bits 2 and 3 are 1)
	// Bitset byte: bits 2 and 3 are 1 -> (1<<2) | (1<<3) = 4 + 8 = 12 (0x0C)
	initialBitset := []byte{0x0C}
	if err := os.WriteFile(metaPath, initialBitset, 0600); err != nil {
		t.Fatalf("failed to write meta file: %v", err)
	}

	readBitset, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read meta file: %v", err)
	}

	if (readBitset[0] & (1 << 0)) != 0 {
		t.Errorf("chunk 0 should already be marked done")
	}
	if (readBitset[0] & (1 << 1)) != 0 {
		t.Errorf("chunk 1 should already be marked done")
	}
	if (readBitset[0] & (1 << 2)) == 0 {
		t.Errorf("chunk 2 should be marked needed")
	}
	if (readBitset[0] & (1 << 3)) == 0 {
		t.Errorf("chunk 3 should be marked needed")
	}

	// Now simulate receiving chunk 2
	readBitset[0] &^= (1 << 2)
	// Now simulate receiving chunk 3
	readBitset[0] &^= (1 << 3)

	// Transfer should now be complete!
	m := &Manager{}
	if !m.isTransferComplete(readBitset) {
		t.Errorf("transfer should be complete when all bits are cleared")
	}
}

