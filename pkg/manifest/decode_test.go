package manifest

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/EchoTools/evrFileTools/pkg/texture"
)

func TestSniffExtension_AllFormats(t *testing.T) {
	// Build a WAV header: RIFF at 0, then 4 bytes size, then WAVE at 8.
	wavData := make([]byte, 16)
	copy(wavData[0:4], []byte("RIFF"))
	copy(wavData[8:12], []byte("WAVE"))

	// Build an OGG header: "OggS"
	oggData := []byte("OggS" + "extradata")

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "DDS",
			data: []byte{0x44, 0x44, 0x53, 0x20, 0x00, 0x00},
			want: ".dds",
		},
		{
			name: "PNG",
			data: []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A},
			want: ".png",
		},
		{
			name: "JPEG",
			data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00},
			want: ".jpg",
		},
		{
			name: "OGG",
			data: oggData,
			want: ".ogg",
		},
		{
			name: "WAV",
			data: wavData,
			want: ".wav",
		},
		{
			name: "ZSTD",
			data: []byte{0x28, 0xB5, 0x2F, 0xFD, 0x00},
			want: ".zst",
		},
		{
			name: "JSON_object",
			data: []byte(`{"key": "value"}`),
			want: ".json",
		},
		{
			name: "JSON_array",
			data: []byte(`[1, 2, 3]`),
			want: ".json",
		},
		{
			name: "unknown_bytes",
			data: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			want: ".bin",
		},
		{
			name: "empty_data",
			data: []byte{},
			want: ".bin",
		},
		{
			name: "short_data_1_byte",
			data: []byte{0xAB},
			want: ".bin",
		},
		{
			name: "short_data_3_bytes",
			data: []byte{0xAB, 0xCD, 0xEF},
			want: ".bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sniffExtension(tt.data)
			if got != tt.want {
				t.Errorf("sniffExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeRawBCTexture_NilMeta(t *testing.T) {
	rawData := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}

	result, err := decodeRawBCTexture(rawData, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(result, rawData) {
		t.Errorf("expected original data returned unchanged")
	}
}

func TestDecodeRawBCTexture_WithMeta(t *testing.T) {
	// Create some fake raw BC data.
	rawData := make([]byte, 64)
	for i := range rawData {
		rawData[i] = byte(i)
	}

	meta := &texture.TextureMetadata{
		Width:       4,
		Height:      4,
		MipLevels:   1,
		DXGIFormat:  texture.DXGI_FORMAT_BC1_UNORM,
		DDSFileSize: 148 + uint32(len(rawData)),
		RawFileSize: uint32(len(rawData)),
		ArraySize:   1,
	}

	result, err := decodeRawBCTexture(rawData, meta)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The result should start with DDS magic.
	if len(result) < 4 {
		t.Fatalf("result too short: %d bytes", len(result))
	}
	magic := binary.LittleEndian.Uint32(result[0:4])
	if magic != 0x20534444 {
		t.Errorf("expected DDS magic 0x20534444, got 0x%08X", magic)
	}

	// ConvertRawBCToDDS creates a 148-byte DX10 header (4 magic + 124 header + 20 DX10).
	expectedSize := 148 + len(rawData)
	if len(result) != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, len(result))
	}

	// Verify the raw data is preserved at the end.
	if !bytes.Equal(result[148:], rawData) {
		t.Errorf("raw data not preserved after DDS header")
	}
}

func TestDecodeRawBCTexture_PatchesRawFileSize(t *testing.T) {
	// Create raw data whose length differs from meta.RawFileSize.
	rawData := make([]byte, 128)
	for i := range rawData {
		rawData[i] = byte(i % 256)
	}

	meta := &texture.TextureMetadata{
		Width:       8,
		Height:      8,
		MipLevels:   1,
		DXGIFormat:  texture.DXGI_FORMAT_BC3_UNORM,
		DDSFileSize: 148 + 999, // intentionally wrong
		RawFileSize: 999,       // does NOT match len(rawData)=128
		ArraySize:   1,
	}

	// decodeRawBCTexture patches RawFileSize before calling ConvertRawBCToDDS,
	// so this should succeed despite the mismatch.
	result, err := decodeRawBCTexture(rawData, meta)
	if err != nil {
		t.Fatalf("expected no error when RawFileSize differs, got: %v", err)
	}

	// Verify the original meta was not mutated (decodeRawBCTexture copies it).
	if meta.RawFileSize != 999 {
		t.Errorf("original metadata was mutated: RawFileSize = %d, want 999", meta.RawFileSize)
	}

	// Result should still be a valid DDS.
	if len(result) < 4 {
		t.Fatalf("result too short")
	}
	magic := binary.LittleEndian.Uint32(result[0:4])
	if magic != 0x20534444 {
		t.Errorf("expected DDS magic, got 0x%08X", magic)
	}
}
