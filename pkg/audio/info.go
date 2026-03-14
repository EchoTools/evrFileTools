package audio

import (
	"bytes"
	"encoding/binary"
)

// AudioInfo contains basic metadata for an audio file.
type AudioInfo struct {
	Format     AudioFormat
	Channels   uint16
	SampleRate uint32
	Duration   float64 // seconds, 0 if unknown
	BitDepth   uint16  // 0 if unknown (e.g. OGG)
}

// ParseInfo extracts basic audio metadata from file data.
// Returns nil if the format is unknown or parsing fails.
func ParseInfo(data []byte) *AudioInfo {
	format := DetectFormat(data)
	if format == FormatUnknown {
		return nil
	}

	switch format {
	case FormatWAV:
		return parseWAV(data)
	case FormatOGG:
		return parseOGG(data)
	default:
		return &AudioInfo{Format: format}
	}
}

// parseWAV parses a WAV/RIFF file to extract audio metadata.
// WAV fmt chunk layout (PCM):
//
//	offset 20: audio format (uint16 LE) — 1 = PCM
//	offset 22: num channels (uint16 LE)
//	offset 24: sample rate (uint32 LE)
//	offset 28: byte rate (uint32 LE)
//	offset 32: block align (uint16 LE)
//	offset 34: bits per sample (uint16 LE)
func parseWAV(data []byte) *AudioInfo {
	info := &AudioInfo{Format: FormatWAV}

	// Need at least enough bytes to read fmt chunk fields
	if len(data) < 36 {
		return info
	}

	info.Channels = binary.LittleEndian.Uint16(data[22:24])
	info.SampleRate = binary.LittleEndian.Uint32(data[24:28])
	info.BitDepth = binary.LittleEndian.Uint16(data[34:36])

	// Look for the "data" chunk to compute duration
	// The fmt chunk header starts at offset 12: "fmt " + size (8 bytes) + 16-byte fmt = offset 36
	// We scan forward for the "data" chunk
	info.Duration = findWAVDuration(data, info.Channels, info.SampleRate, info.BitDepth)

	return info
}

// findWAVDuration scans the RIFF chunks for a "data" chunk and computes duration.
func findWAVDuration(data []byte, channels uint16, sampleRate uint32, bitDepth uint16) float64 {
	if channels == 0 || sampleRate == 0 || bitDepth == 0 {
		return 0
	}

	// Scan chunks starting after "RIFF....WAVE" (12 bytes)
	offset := 12
	for offset+8 <= len(data) {
		chunkID := data[offset : offset+4]
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])

		if bytes.Equal(chunkID, []byte("data")) {
			bytesPerSample := uint32(bitDepth) / 8
			if bytesPerSample == 0 {
				return 0
			}
			bytesPerFrame := bytesPerSample * uint32(channels)
			if bytesPerFrame == 0 {
				return 0
			}
			numFrames := float64(chunkSize) / float64(bytesPerFrame)
			return numFrames / float64(sampleRate)
		}

		// Advance to next chunk (8-byte header + chunk data, padded to even size)
		advance := 8 + int(chunkSize)
		if chunkSize%2 != 0 {
			advance++
		}
		offset += advance
	}

	return 0
}

// parseOGG parses an Ogg Vorbis file to extract channel count and sample rate.
// OGG page structure:
//
//	bytes  0-3:  capture_pattern "OggS"
//	byte   4:    version (0)
//	byte   5:    header_type
//	bytes  6-13: granule_position (int64 LE)
//	bytes 14-17: serial (uint32 LE)
//	bytes 18-21: page_sequence (uint32 LE)
//	bytes 22-25: checksum (uint32 LE)
//	byte  26:    page_segments
//	bytes 27...: lace_values[page_segments]
//
// Vorbis identification header (first packet in first page):
//
//	byte 0:      packet_type (1 = ident)
//	bytes 1-6:   "vorbis"
//	bytes 7-10:  vorbis_version (uint32 LE)
//	byte  11:    audio_channels
//	bytes 12-15: audio_sample_rate (uint32 LE)
func parseOGG(data []byte) *AudioInfo {
	info := &AudioInfo{Format: FormatOGG}

	// Need at least the OGG page header
	if len(data) < 27 {
		return info
	}

	pageSegments := int(data[26])
	// lace_values start at offset 27
	packetDataOffset := 27 + pageSegments
	if packetDataOffset+16 > len(data) {
		return info
	}

	// Vorbis ident header: \x01vorbis (7 bytes)
	vorbisIdent := []byte{0x01, 'v', 'o', 'r', 'b', 'i', 's'}
	packet := data[packetDataOffset:]
	if len(packet) < 16 || !bytes.HasPrefix(packet, vorbisIdent) {
		return info
	}

	// After the 7-byte ident: vorbis_version (4), channels (1), sample_rate (4)
	// offset in packet: 7 = version, 11 = channels, 12 = sample_rate
	if len(packet) < 16 {
		return info
	}

	info.Channels = uint16(packet[11])
	info.SampleRate = binary.LittleEndian.Uint32(packet[12:16])

	return info
}
