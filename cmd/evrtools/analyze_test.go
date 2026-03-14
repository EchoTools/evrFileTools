package main

import (
	"math"
	"testing"
)

func TestDetectMagic_AllSignatures(t *testing.T) {
	// Track which magic byte patterns we have already seen. Entries that
	// share identical (offset, magic) with an earlier entry are shadowed
	// and can never be the first match -- skip them (the RIFF duplicate
	// bug is covered by TestDetectMagic_DuplicateRIFF).
	type magicKey struct {
		offset int
		magic  string
	}
	seen := make(map[magicKey]string)

	for _, sig := range magicSignatures {
		key := magicKey{sig.offset, string(sig.magic)}
		if first, ok := seen[key]; ok {
			t.Logf("skipping %q: shadowed by earlier entry %q (same magic)", sig.name, first)
			continue
		}
		seen[key] = sig.name

		t.Run(sig.name, func(t *testing.T) {
			// Build a data slice with the magic bytes at the correct offset,
			// padded with zeros before and after.
			data := make([]byte, sig.offset+len(sig.magic)+4)
			copy(data[sig.offset:], sig.magic)

			got := detectMagic(data)
			if got != sig.name {
				t.Errorf("detectMagic() = %q, want %q", got, sig.name)
			}
		})
	}
}

// TestDetectMagic_RIFFContainer verifies that all RIFF-based formats
// (WAV, AVI, etc.) are classified as "RIFF container" since sub-format
// detection is handled by sniffExtension in decode.go.
func TestDetectMagic_RIFFContainer(t *testing.T) {
	// Construct a RIFF/AVI header: "RIFF" + 4-byte size + "AVI "
	avi := []byte{
		0x52, 0x49, 0x46, 0x46, // "RIFF"
		0x00, 0x00, 0x00, 0x00, // size placeholder
		0x41, 0x56, 0x49, 0x20, // "AVI " sub-format
	}

	got := detectMagic(avi)
	if got != "RIFF container" {
		t.Errorf("detectMagic(AVI data) = %q, want %q", got, "RIFF container")
	}
}

func TestDetectMagic_UnknownBytes(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	got := detectMagic(data)
	if got != "unknown" {
		t.Errorf("detectMagic() = %q, want %q", got, "unknown")
	}
}

func TestShannonEntropy_AllSame(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = 0x41
	}
	got := shannonEntropy(data)
	if got != 0 {
		t.Errorf("shannonEntropy(all same) = %f, want 0", got)
	}
}

func TestShannonEntropy_Uniform(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	got := shannonEntropy(data)
	// Maximum entropy for 256 equally-likely symbols is exactly 8.0.
	if math.Abs(got-8.0) > 0.001 {
		t.Errorf("shannonEntropy(uniform) = %f, want ~8.0", got)
	}
}

func TestShannonEntropy_Empty(t *testing.T) {
	got := shannonEntropy(nil)
	if got != 0 {
		t.Errorf("shannonEntropy(empty) = %f, want 0", got)
	}
}

func TestTopFormat_Basic(t *testing.T) {
	formats := map[string]int{
		"DDS texture": 10,
		"PNG image":   3,
		"unknown":     1,
	}
	got := topFormat(formats)
	if got != "DDS texture" {
		t.Errorf("topFormat() = %q, want %q", got, "DDS texture")
	}
}

func TestTopFormat_Empty(t *testing.T) {
	got := topFormat(map[string]int{})
	if got != "unknown" {
		t.Errorf("topFormat(empty) = %q, want %q", got, "unknown")
	}
}
