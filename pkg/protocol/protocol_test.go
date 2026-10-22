package protocol

import (
	"bytes"
	"testing"
)

func TestFramingRoundTrip(t *testing.T) {
	var buf bytes.Buffer

	frameType := TypeMsg
	payload := []byte(`{"v":1,"body":"test message"}`)

	if err := WriteFrame(&buf, frameType, payload); err != nil {
		t.Fatalf("failed to write frame: %v", err)
	}

	readType, readPayload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("failed to read frame: %v", err)
	}

	if readType != frameType {
		t.Errorf("expected frame type %d, got %d", frameType, readType)
	}

	if !bytes.Equal(readPayload, payload) {
		t.Errorf("payload mismatch: expected %s, got %s", string(payload), string(readPayload))
	}
}

func TestFrameSizeLimitEnforcement(t *testing.T) {
	var buf bytes.Buffer
	oversizedPayload := make([]byte, MaxFrameLength+1)

	err := WriteFrame(&buf, TypeChunk, oversizedPayload)
	if err != ErrFrameTooLarge {
		t.Errorf("expected ErrFrameTooLarge on write, got: %v", err)
	}
}
