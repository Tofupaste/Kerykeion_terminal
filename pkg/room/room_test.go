package room

import (
	"testing"
)

func TestRoomNameNormalization(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		shouldError bool
	}{
		{"Ngobs", "ngobs", false},
		{"ngobs-kantor", "ngobs-kantor", false},
		{"room_123", "room_123", false},
		{"a", "", true},                                   // too short (< 2)
		{"this-room-name-is-way-too-long-to-be-valid-for-a-room", "", true}, // too long (> 32)
		{"room with space", "", true},                     // space not allowed
		{"room@kantor", "", true},                         // special chars
	}

	for _, tc := range tests {
		actual, err := NormalizeName(tc.input)
		if tc.shouldError && err == nil {
			t.Errorf("NormalizeName(%q) expected error, got nil (result: %q)", tc.input, actual)
		} else if !tc.shouldError && err != nil {
			t.Errorf("NormalizeName(%q) unexpected error: %v", tc.input, err)
		} else if !tc.shouldError && actual != tc.expected {
			t.Errorf("NormalizeName(%q) = %q; expected %q", tc.input, actual, tc.expected)
		}
	}
}
