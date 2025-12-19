// Package manifest provides types and functions for working with EVR manifest files.
package manifest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/goopsie/evrFileTools/pkg/archive"
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
	Unk1          uint32   // Unknown - 524288 on latest builds
	Unk2          uint64   // Unknown - 0 on latest builds
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
func (m *Manifest) UnmarshalBinary(data []byte) error {
	reader := bytes.NewReader(data)

	if err := binary.Read(reader, binary.LittleEndian, &m.Header); err != nil {
		return fmt.Errorf("read header: %w", err)
	}

	m.FrameContents = make([]FrameContent, m.Header.FrameContents.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.FrameContents); err != nil {
		return fmt.Errorf("read frame contents: %w", err)
	}

	m.Metadata = make([]FileMetadata, m.Header.Metadata.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.Metadata); err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	m.Frames = make([]Frame, m.Header.Frames.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.Frames); err != nil {
		return fmt.Errorf("read frames: %w", err)
	}

	return nil
}

// MarshalBinary encodes a manifest to binary data.
func (m *Manifest) MarshalBinary() ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	sections := []any{
		m.Header,
		m.FrameContents,
		m.Metadata,
		m.Frames,
	}

	for _, section := range sections {
		if err := binary.Write(buf, binary.LittleEndian, section); err != nil {
			return nil, fmt.Errorf("write section: %w", err)
		}
	}

	return buf.Bytes(), nil
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
