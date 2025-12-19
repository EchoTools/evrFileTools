// Package manifest provides types and functions for working with EVR manifest files.
package manifest

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/goopsie/evrFileTools/pkg/archive"
)

// Binary sizes for manifest structures
const (
	HeaderSize       = 192 // Fixed header size (4+4+8 + 48+16 + 48+16 + 48)
	SectionSize      = 48  // 6 * 8 bytes
	FrameContentSize = 32  // 8 + 8 + 4 + 4 + 4 + 4 bytes
	FileMetadataSize = 40  // 5 * 8 bytes
	FrameSize        = 16  // 4 * 4 bytes
)

// Manifest represents a parsed EVR manifest file.
type Manifest struct {
	Header        Header
	FrameContents []FrameContent
	Metadata      []FileMetadata
	Frames        []Frame
}

// Header contains manifest metadata and section information.
type Header struct {
	PackageCount  uint32
	Unk1          uint32 // Unknown - 524288 on latest builds
	Unk2          uint64 // Unknown - 0 on latest builds
	FrameContents Section
	_             [16]byte // Padding
	Metadata      Section
	_             [16]byte // Padding
	Frames        Section
}

// Section describes a section within the manifest.
type Section struct {
	Length       uint64 // Total byte length of section
	Unk1         uint64 // Unknown - 0 on latest builds
	Unk2         uint64 // Unknown - 4294967296 on latest builds
	ElementSize  uint64 // Byte size of single entry
	Count        uint64 // Number of elements
	ElementCount uint64 // Number of elements (can differ from Count)
}

// FrameContent describes a file within a frame.
type FrameContent struct {
	TypeSymbol int64  // File type identifier
	FileSymbol int64  // File identifier
	FrameIndex uint32 // Index into Frames array
	DataOffset uint32 // Byte offset within decompressed frame
	Size       uint32 // File size in bytes
	Alignment  uint32 // Alignment (can be set to 1)
}

// FileMetadata contains additional file metadata.
type FileMetadata struct {
	TypeSymbol int64 // File type identifier
	FileSymbol int64 // File identifier
	Unk1       int64 // Unknown - game launches with 0
	Unk2       int64 // Unknown - game launches with 0
	AssetType  int64 // Asset type identifier
}

// Frame describes a compressed data frame within a package.
type Frame struct {
	PackageIndex   uint32 // Package file index
	Offset         uint32 // Byte offset within package
	CompressedSize uint32 // Compressed frame size
	Length         uint32 // Decompressed frame size
}

// PackageCount returns the number of packages referenced by this manifest.
func (m *Manifest) PackageCount() int {
	return int(m.Header.PackageCount)
}

// FileCount returns the number of files in this manifest.
func (m *Manifest) FileCount() int {
	return len(m.FrameContents)
}

// UnmarshalBinary decodes a manifest from binary data.
// Uses direct decoding for better performance.
func (m *Manifest) UnmarshalBinary(data []byte) error {
	if len(data) < HeaderSize {
		return fmt.Errorf("data too short for header")
	}

	// Decode header inline
	offset := 0
	m.Header.PackageCount = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	m.Header.Unk1 = binary.LittleEndian.Uint32(data[offset:])
	offset += 4
	m.Header.Unk2 = binary.LittleEndian.Uint64(data[offset:])
	offset += 8

	// FrameContents section
	decodeSection(&m.Header.FrameContents, data[offset:])
	offset += SectionSize + 16 // +16 for padding

	// Metadata section
	decodeSection(&m.Header.Metadata, data[offset:])
	offset += SectionSize + 16 // +16 for padding

	// Frames section
	decodeSection(&m.Header.Frames, data[offset:])
	offset += SectionSize

	// Decode FrameContents
	count := int(m.Header.FrameContents.ElementCount)
	m.FrameContents = make([]FrameContent, count)
	for i := 0; i < count; i++ {
		m.FrameContents[i].TypeSymbol = int64(binary.LittleEndian.Uint64(data[offset:]))
		m.FrameContents[i].FileSymbol = int64(binary.LittleEndian.Uint64(data[offset+8:]))
		m.FrameContents[i].FrameIndex = binary.LittleEndian.Uint32(data[offset+16:])
		m.FrameContents[i].DataOffset = binary.LittleEndian.Uint32(data[offset+20:])
		m.FrameContents[i].Size = binary.LittleEndian.Uint32(data[offset+24:])
		m.FrameContents[i].Alignment = binary.LittleEndian.Uint32(data[offset+28:])
		offset += FrameContentSize
	}

	// Decode Metadata
	count = int(m.Header.Metadata.ElementCount)
	m.Metadata = make([]FileMetadata, count)
	for i := 0; i < count; i++ {
		m.Metadata[i].TypeSymbol = int64(binary.LittleEndian.Uint64(data[offset:]))
		m.Metadata[i].FileSymbol = int64(binary.LittleEndian.Uint64(data[offset+8:]))
		m.Metadata[i].Unk1 = int64(binary.LittleEndian.Uint64(data[offset+16:]))
		m.Metadata[i].Unk2 = int64(binary.LittleEndian.Uint64(data[offset+24:]))
		m.Metadata[i].AssetType = int64(binary.LittleEndian.Uint64(data[offset+32:]))
		offset += FileMetadataSize
	}

	// Decode Frames
	count = int(m.Header.Frames.ElementCount)
	m.Frames = make([]Frame, count)
	for i := 0; i < count; i++ {
		m.Frames[i].PackageIndex = binary.LittleEndian.Uint32(data[offset:])
		m.Frames[i].Offset = binary.LittleEndian.Uint32(data[offset+4:])
		m.Frames[i].CompressedSize = binary.LittleEndian.Uint32(data[offset+8:])
		m.Frames[i].Length = binary.LittleEndian.Uint32(data[offset+12:])
		offset += FrameSize
	}

	return nil
}

func decodeSection(s *Section, data []byte) {
	s.Length = binary.LittleEndian.Uint64(data[0:])
	s.Unk1 = binary.LittleEndian.Uint64(data[8:])
	s.Unk2 = binary.LittleEndian.Uint64(data[16:])
	s.ElementSize = binary.LittleEndian.Uint64(data[24:])
	s.Count = binary.LittleEndian.Uint64(data[32:])
	s.ElementCount = binary.LittleEndian.Uint64(data[40:])
}

// MarshalBinary encodes a manifest to binary data.
// Pre-allocates buffer for better performance.
func (m *Manifest) MarshalBinary() ([]byte, error) {
	buf := make([]byte, m.BinarySize())
	m.EncodeTo(buf)
	return buf, nil
}

// BinarySize returns the total binary size of the manifest.
func (m *Manifest) BinarySize() int {
	return HeaderSize +
		len(m.FrameContents)*FrameContentSize +
		len(m.Metadata)*FileMetadataSize +
		len(m.Frames)*FrameSize
}

// EncodeTo writes the manifest to the given buffer.
// The buffer must be at least BinarySize() bytes.
func (m *Manifest) EncodeTo(buf []byte) {
	offset := 0

	// Encode header
	binary.LittleEndian.PutUint32(buf[offset:], m.Header.PackageCount)
	offset += 4
	binary.LittleEndian.PutUint32(buf[offset:], m.Header.Unk1)
	offset += 4
	binary.LittleEndian.PutUint64(buf[offset:], m.Header.Unk2)
	offset += 8

	// FrameContents section
	encodeSection(&m.Header.FrameContents, buf[offset:])
	offset += SectionSize + 16

	// Metadata section
	encodeSection(&m.Header.Metadata, buf[offset:])
	offset += SectionSize + 16

	// Frames section
	encodeSection(&m.Header.Frames, buf[offset:])
	offset += SectionSize

	// Encode FrameContents
	for i := range m.FrameContents {
		binary.LittleEndian.PutUint64(buf[offset:], uint64(m.FrameContents[i].TypeSymbol))
		binary.LittleEndian.PutUint64(buf[offset+8:], uint64(m.FrameContents[i].FileSymbol))
		binary.LittleEndian.PutUint32(buf[offset+16:], m.FrameContents[i].FrameIndex)
		binary.LittleEndian.PutUint32(buf[offset+20:], m.FrameContents[i].DataOffset)
		binary.LittleEndian.PutUint32(buf[offset+24:], m.FrameContents[i].Size)
		binary.LittleEndian.PutUint32(buf[offset+28:], m.FrameContents[i].Alignment)
		offset += FrameContentSize
	}

	// Encode Metadata
	for i := range m.Metadata {
		binary.LittleEndian.PutUint64(buf[offset:], uint64(m.Metadata[i].TypeSymbol))
		binary.LittleEndian.PutUint64(buf[offset+8:], uint64(m.Metadata[i].FileSymbol))
		binary.LittleEndian.PutUint64(buf[offset+16:], uint64(m.Metadata[i].Unk1))
		binary.LittleEndian.PutUint64(buf[offset+24:], uint64(m.Metadata[i].Unk2))
		binary.LittleEndian.PutUint64(buf[offset+32:], uint64(m.Metadata[i].AssetType))
		offset += FileMetadataSize
	}

	// Encode Frames
	for i := range m.Frames {
		binary.LittleEndian.PutUint32(buf[offset:], m.Frames[i].PackageIndex)
		binary.LittleEndian.PutUint32(buf[offset+4:], m.Frames[i].Offset)
		binary.LittleEndian.PutUint32(buf[offset+8:], m.Frames[i].CompressedSize)
		binary.LittleEndian.PutUint32(buf[offset+12:], m.Frames[i].Length)
		offset += FrameSize
	}
}

func encodeSection(s *Section, buf []byte) {
	binary.LittleEndian.PutUint64(buf[0:], s.Length)
	binary.LittleEndian.PutUint64(buf[8:], s.Unk1)
	binary.LittleEndian.PutUint64(buf[16:], s.Unk2)
	binary.LittleEndian.PutUint64(buf[24:], s.ElementSize)
	binary.LittleEndian.PutUint64(buf[32:], s.Count)
	binary.LittleEndian.PutUint64(buf[40:], s.ElementCount)
}

// ReadFile reads and parses a manifest from a file.
func ReadFile(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()

	data, err := archive.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read archive: %w", err)
	}

	manifest := &Manifest{}
	if err := manifest.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return manifest, nil
}

// WriteFile writes a manifest to a file.
func WriteFile(path string, m *Manifest) error {
	data, err := m.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if err := archive.Encode(f, data); err != nil {
		return fmt.Errorf("encode archive: %w", err)
	}

	return nil
}
