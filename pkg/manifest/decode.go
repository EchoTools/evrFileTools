package manifest

import (
	"bytes"
	"fmt"

	"github.com/EchoTools/evrFileTools/pkg/audio"
	"github.com/EchoTools/evrFileTools/pkg/texture"
)

// decodeRawBCTexture converts a TypeRawBCTexture payload to a full DDS file.
// Returns the original data unchanged if no metadata is available.
func decodeRawBCTexture(rawBC []byte, meta *texture.TextureMetadata) ([]byte, error) {
	if meta == nil {
		return rawBC, nil
	}
	// Patch the metadata's RawFileSize to match actual data length, which may
	// differ from what the metadata records (game sometimes stores rounded values).
	metaCopy := *meta
	metaCopy.RawFileSize = uint32(len(rawBC))
	return texture.ConvertRawBCToDDS(rawBC, &metaCopy)
}

// sniffExtension inspects the leading bytes of data and returns a file extension.
// Returns ".bin" if the format is not recognised.
func sniffExtension(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}

	switch {
	// DDS: magic "DDS " (little-endian 0x20534444)
	case data[0] == 0x44 && data[1] == 0x44 && data[2] == 0x53 && data[3] == 0x20:
		return ".dds"
	// PNG: \x89PNG
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return ".png"
	// JPEG: \xff\xd8\xff
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return ".jpg"
	// RIFF WAVE
	case len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x41 && data[10] == 0x56 && data[11] == 0x45:
		return ".wav"
	// ZSTD: magic 0xFD2FB528
	case data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD:
		return ".zst"
	// JSON object or array
	case data[0] == '{' || data[0] == '[':
		return ".json"
	}

	// Use the audio package for OGG / MP3 / FLAC detection.
	if af := audio.DetectFormat(data); af != audio.FormatUnknown {
		return af.Extension()
	}

	return ".bin"
}

// parseTextureMetadata parses a TextureMetadata from a decompressed frame slice.
func parseTextureMetadata(data []byte) (*texture.TextureMetadata, error) {
	if len(data) < texture.MetadataSize {
		return nil, fmt.Errorf("texture metadata too short: %d bytes", len(data))
	}
	return texture.ParseMetadata(bytes.NewReader(data[:texture.MetadataSize]))
}
