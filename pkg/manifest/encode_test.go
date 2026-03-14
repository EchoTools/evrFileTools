package manifest

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/EchoTools/evrFileTools/pkg/naming"
	"github.com/EchoTools/evrFileTools/pkg/texture"
)

// makeDDSStandard creates a minimal DDS file with a standard 128-byte header
// (no DX10 extension) followed by the given pixel data.
func makeDDSStandard(pixelData []byte) []byte {
	header := make([]byte, 128)
	// Magic "DDS "
	binary.LittleEndian.PutUint32(header[0:4], 0x20534444)
	// dwSize = 124
	binary.LittleEndian.PutUint32(header[4:8], 124)
	// FourCC at offset 84: set to something other than DX10 (e.g., "DXT1" = 0x31545844)
	binary.LittleEndian.PutUint32(header[84:88], 0x31545844)
	return append(header, pixelData...)
}

// makeDDSDX10 creates a minimal DDS file with a 148-byte DX10 header
// followed by the given pixel data.
func makeDDSDX10(pixelData []byte) []byte {
	header := make([]byte, 148)
	// Magic "DDS "
	binary.LittleEndian.PutUint32(header[0:4], 0x20534444)
	// dwSize = 124
	binary.LittleEndian.PutUint32(header[4:8], 124)
	// FourCC at offset 84: "DX10" = 0x30315844
	binary.LittleEndian.PutUint32(header[84:88], 0x30315844)
	return append(header, pixelData...)
}

func TestStripDDSHeader_Standard(t *testing.T) {
	pixelData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE}
	dds := makeDDSStandard(pixelData)

	result, err := stripDDSHeader(dds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, pixelData) {
		t.Errorf("pixel data mismatch: got %x, want %x", result, pixelData)
	}
}

func TestStripDDSHeader_DX10(t *testing.T) {
	pixelData := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	dds := makeDDSDX10(pixelData)

	result, err := stripDDSHeader(dds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, pixelData) {
		t.Errorf("pixel data mismatch: got %x, want %x", result, pixelData)
	}
}

func TestStripDDSHeader_TooShort(t *testing.T) {
	data := []byte{0x44, 0x44, 0x53} // 3 bytes, less than 4
	_, err := stripDDSHeader(data)
	if err == nil {
		t.Fatal("expected error for data < 4 bytes, got nil")
	}
}

func TestStripDDSHeader_NotDDS(t *testing.T) {
	// Non-DDS data should be returned unchanged (not an error).
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	result, err := stripDDSHeader(data)
	if err != nil {
		t.Fatalf("unexpected error for non-DDS data: %v", err)
	}
	if !bytes.Equal(result, data) {
		t.Errorf("non-DDS data should be returned unchanged")
	}
}

func TestEncodeFile_NonTexture(t *testing.T) {
	data := []byte("some arbitrary file content")
	// Use a non-texture type symbol (0 is unknown).
	result, err := encodeFile(data, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, data) {
		t.Errorf("non-texture data should be returned unchanged")
	}
}

func TestEncodeFile_RawBCTexture_DDS(t *testing.T) {
	pixelData := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	dds := makeDDSStandard(pixelData)

	result, err := encodeFile(dds, int64(naming.TypeRawBCTexture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, pixelData) {
		t.Errorf("expected DDS header to be stripped; got %d bytes, want %d", len(result), len(pixelData))
	}
}

func TestEncodeFile_RawBCTexture_NotDDS(t *testing.T) {
	// Raw BC data (no DDS header) passed with texture type should be returned as-is.
	rawBC := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	result, err := encodeFile(rawBC, int64(naming.TypeRawBCTexture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, rawBC) {
		t.Errorf("non-DDS raw BC data should be returned unchanged")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Start with raw BC pixel data.
	originalRaw := make([]byte, 64)
	for i := range originalRaw {
		originalRaw[i] = byte(i * 3)
	}

	meta := &texture.TextureMetadata{
		Width:       4,
		Height:      4,
		MipLevels:   1,
		DXGIFormat:  texture.DXGI_FORMAT_BC7_UNORM,
		DDSFileSize: 148 + uint32(len(originalRaw)),
		RawFileSize: uint32(len(originalRaw)),
		ArraySize:   1,
	}

	// Decode: prepend DDS header (simulates extract).
	decoded, err := decodeRawBCTexture(originalRaw, meta)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	// Verify decoded is a DDS file.
	if len(decoded) < 4 {
		t.Fatalf("decoded too short")
	}
	magic := binary.LittleEndian.Uint32(decoded[0:4])
	if magic != 0x20534444 {
		t.Fatalf("decoded is not DDS: magic = 0x%08X", magic)
	}

	// Encode: strip DDS header (simulates pack).
	encoded, err := encodeFile(decoded, int64(naming.TypeRawBCTexture))
	if err != nil {
		t.Fatalf("encode error: %v", err)
	}

	// The round-trip should recover the exact original raw data.
	if !bytes.Equal(encoded, originalRaw) {
		t.Errorf("round-trip failed: got %d bytes, want %d bytes", len(encoded), len(originalRaw))
		// Show first few bytes for debugging.
		showLen := 16
		if len(encoded) < showLen {
			showLen = len(encoded)
		}
		t.Errorf("  got[:16]:  %x", encoded[:showLen])
		showLen = 16
		if len(originalRaw) < showLen {
			showLen = len(originalRaw)
		}
		t.Errorf("  want[:16]: %x", originalRaw[:showLen])
	}
}
