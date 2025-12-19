// Package archive provides types and functions for working with ZSTD compressed archives.
package archive

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Magic bytes identifying a ZSTD archive header.
var Magic = [4]byte{0x5a, 0x53, 0x54, 0x44} // "ZSTD"

// Header represents the header of a compressed archive file.
type Header struct {
	Magic            [4]byte
	HeaderLength     uint32
	Length           uint64 // Uncompressed size
	CompressedLength uint64 // Compressed size
}

// Size returns the binary size of the header.
func (h *Header) Size() int {
	return binary.Size(h)
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
func (h *Header) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, h); err != nil {
		return nil, fmt.Errorf("marshal header: %w", err)
	}
	return buf.Bytes(), nil
}

// UnmarshalBinary decodes the header from binary format.
func (h *Header) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, h); err != nil {
		return fmt.Errorf("unmarshal header: %w", err)
	}
	return h.Validate()
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
