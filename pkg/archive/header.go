// Package archive provides types and functions for working with ZSTD compressed archives.
package archive

import (
	"encoding/binary"
	"fmt"
)

// Magic bytes identifying a ZSTD archive header.
var Magic = [4]byte{0x5a, 0x53, 0x54, 0x44} // "ZSTD"

// HeaderSize is the fixed binary size of an archive header.
const HeaderSize = 24 // 4 + 4 + 8 + 8 bytes

// Header represents the header of a compressed archive file.
type Header struct {
	Magic            [4]byte
	HeaderLength     uint32
	Length           uint64 // Uncompressed size
	CompressedLength uint64 // Compressed size
}

// Size returns the binary size of the header.
func (h *Header) Size() int {
	return HeaderSize
}

// Validate checks the header for validity.
func (h *Header) Validate() error {
	if h.Magic != Magic {
		return fmt.Errorf("invalid magic: expected %x, got %x", Magic, h.Magic)
	}
	if h.HeaderLength != 16 {
		return fmt.Errorf("invalid header length: expected 16, got %d", h.HeaderLength)
	}
	if h.Length == 0 {
		return fmt.Errorf("uncompressed size is zero")
	}
	if h.CompressedLength == 0 {
		return fmt.Errorf("compressed size is zero")
	}
	return nil
}

// MarshalBinary encodes the header to binary format.
// Uses direct encoding to avoid allocations.
func (h *Header) MarshalBinary() ([]byte, error) {
	buf := make([]byte, HeaderSize)
	h.EncodeTo(buf)
	return buf, nil
}

// EncodeTo writes the header to the given buffer.
// The buffer must be at least HeaderSize bytes.
func (h *Header) EncodeTo(buf []byte) {
	copy(buf[0:4], h.Magic[:])
	binary.LittleEndian.PutUint32(buf[4:8], h.HeaderLength)
	binary.LittleEndian.PutUint64(buf[8:16], h.Length)
	binary.LittleEndian.PutUint64(buf[16:24], h.CompressedLength)
}

// UnmarshalBinary decodes the header from binary format.
// Uses direct decoding to avoid allocations.
func (h *Header) UnmarshalBinary(data []byte) error {
	if len(data) < HeaderSize {
		return fmt.Errorf("header data too short: need %d, got %d", HeaderSize, len(data))
	}
	h.DecodeFrom(data)
	return h.Validate()
}

// DecodeFrom reads the header from the given buffer.
// Does not validate - use UnmarshalBinary for validation.
func (h *Header) DecodeFrom(data []byte) {
	copy(h.Magic[:], data[0:4])
	h.HeaderLength = binary.LittleEndian.Uint32(data[4:8])
	h.Length = binary.LittleEndian.Uint64(data[8:16])
	h.CompressedLength = binary.LittleEndian.Uint64(data[16:24])
}

// NewHeader creates a new archive header with the given sizes.
func NewHeader(uncompressedSize, compressedSize uint64) *Header {
	return &Header{
		Magic:            Magic,
		HeaderLength:     16,
		Length:           uncompressedSize,
		CompressedLength: compressedSize,
	}
}
