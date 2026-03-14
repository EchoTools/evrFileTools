package main

import (
	"testing"
)

func TestParseHex(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"with 0x prefix", "0xbeac1969cb7b8861", 0xbeac1969cb7b8861, false},
		{"without prefix", "beac1969cb7b8861", 0xbeac1969cb7b8861, false},
		{"zero", "0", 0, false},
		{"max uint64", "ffffffffffffffff", 0xffffffffffffffff, false},
		{"empty string", "", 0, true},
		{"invalid hex", "not_hex", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHex(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseHex(%q) expected error, got %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHex(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseHex(%q) = 0x%x, want 0x%x", tc.input, got, tc.want)
			}
		})
	}
}
