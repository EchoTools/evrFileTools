package tool

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/DataDog/zstd"
)

const zstdCompressionLevel = zstd.BestSpeed

type ArchiveHeader struct { // seems to be the same across every manifest
	Magic            [4]byte
	HeaderLength     uint32
	Length           uint64
	CompressedLength uint64
}

func (c ArchiveHeader) Len() int {
	return binary.Size(c)
}

// Validate checks the header for validity.
func (c ArchiveHeader) Validate() error {
	if c.Magic != [4]byte{0x5a, 0x53, 0x54, 0x44} {
		return fmt.Errorf("invalid magic number")
	}
	if c.HeaderLength != 16 {
		return fmt.Errorf("invalid header length")
	}
	if c.Length == 0 {
		return fmt.Errorf("uncompressed size is zero")
	}
	if c.CompressedLength == 0 {
		return fmt.Errorf("compressed size is zero")
	}
	return nil
}

func (c ArchiveHeader) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, c); err != nil {
		return nil, fmt.Errorf("failed to marshal header: %w", err)
	}
	return buf.Bytes(), nil
}

func (c *ArchiveHeader) UnmarshalBinary(data []byte) error {
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, c); err != nil {
		return fmt.Errorf("failed to unmarshal header: %w", err)
	}

	// Validate the header
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}

	return nil
}

// NewArchiveReader creates a new reader for the package file.
func NewArchiveReader(r io.ReadSeeker) (reader io.ReadCloser, length int, cLength int, err error) {
	// Read the header
	header := &ArchiveHeader{}

	// Use UnmarshalBinary to read the header
	headerBytes := make([]byte, header.Len())
	if _, err := r.Read(headerBytes); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read header: %w", err)
	}

	if err := header.UnmarshalBinary(headerBytes); err != nil {
		return nil, 0, 0, fmt.Errorf("failed to unmarshal header: %w", err)
	}

	// Use a reader to avoid reading the entire file into memory
	uncompressed := zstd.NewReader(r)

	return uncompressed, int(header.Length), int(header.CompressedLength), nil
}

// ArchiveDecode reads a compressed file and returns the uncompressed data.
// It uses a zstd reader to decompress the data and returns the uncompressed bytes.
// The function also handles the header of the compressed file.
func ArchiveDecode(compressed io.ReadSeeker) ([]byte, error) {

	reader, length, compressedLength, err := NewArchiveReader(compressed)
	if err != nil {
		return nil, fmt.Errorf("failed to create package reader: %w", err)
	}
	defer reader.Close()

	dst := make([]byte, length)

	// Read the compressed data
	if n, err := compressed.Read(dst); err != nil {
		return nil, fmt.Errorf("failed to read compressed data: %w", err)
	} else if n != int(compressedLength) {
		return nil, fmt.Errorf("expected %d bytes, got %d", length, n)
	}

	return dst[:length], nil
}

func ArchiveEncode(dst io.WriteSeeker, data []byte) error {

	// Write a placeholder for the compressed size
	header := ArchiveHeader{
		Magic:            [4]byte{0x5a, 0x53, 0x54, 0x44},
		HeaderLength:     16,
		Length:           uint64(len(data)),
		CompressedLength: 0, // Placeholder for compressed size
	}

	// Write the header
	headerBytes, err := header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}
	if _, err := dst.Write(headerBytes); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	writer := zstd.NewWriterLevel(dst, zstdCompressionLevel)
	defer writer.Close()

	compressedLength, err := writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write compressed data: %w", err)
	}

	// Write the compressed size to the header
	header.CompressedLength = uint64(compressedLength)
	headerBytes, err = header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Seek back to the beginning of the file and write the header again
	if _, err := dst.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek to beginning: %w", err)
	}
	if _, err := dst.Write(headerBytes); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	dst.Seek(int64(header.Len()+compressedLength), 0)

	return nil
}
