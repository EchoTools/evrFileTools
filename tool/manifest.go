package tool

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
)

type Manifest interface {
	UnmarshalBinary([]byte) error
	MarshalBinary() ([]byte, error)
}

type ManifestBase struct {
	Header        ManifestHeader
	FrameContents []FrameContents
	SomeStructure []SomeStructure
	Frames        []Frame
}

func (m ManifestBase) PackageCount() int {
	return int(m.Header.PackageCount)
}

func (m *ManifestBase) UnmarshalBinary(b []byte) error {
	reader := bytes.NewReader(b)

	if err := binary.Read(reader, binary.LittleEndian, &m.Header); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	m.FrameContents = make([]FrameContents, m.Header.FrameContents.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.FrameContents); err != nil {
		return fmt.Errorf("failed to read frame contents: %w", err)
	}

	m.SomeStructure = make([]SomeStructure, m.Header.SomeStructure.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.SomeStructure); err != nil {
		return fmt.Errorf("failed to read some structure: %w", err)
	}

	m.Frames = make([]Frame, m.Header.Frames.ElementCount)
	if err := binary.Read(reader, binary.LittleEndian, &m.Frames); err != nil {
		return fmt.Errorf("failed to read frames: %w", err)
	}

	return nil
}

func (m *ManifestBase) MarshalBinary() ([]byte, error) {
	wbuf := bytes.NewBuffer(nil)

	var data = []any{
		m.Header,
		m.FrameContents,
		m.SomeStructure,
		m.Frames,
	}

	for _, v := range data {
		err := binary.Write(wbuf, binary.LittleEndian, v)
		if err != nil {
			fmt.Println("binary.Write failed:", err)
		}
	}

	manifestBytes := wbuf.Bytes()
	return manifestBytes, nil // hack
}

func ManifestReadFile(manifestFilePath string) (*ManifestBase, error) {
	// Allocate the destination buffer

	manifestFile, err := os.OpenFile(manifestFilePath, os.O_RDWR, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer manifestFile.Close()

	archiveReader, length, _, err := NewArchiveReader(manifestFile)
	if err != nil {
		fmt.Println("Failed to create package reader")
	}

	b := make([]byte, length)

	// Read the compressed data
	if n, err := archiveReader.Read(b); err != nil {
		return nil, fmt.Errorf("failed to read compressed data: %w", err)
	} else if n != int(length) {
		return nil, fmt.Errorf("expected %d bytes, got %d", length, n)
	}
	defer archiveReader.Close()

	manifest := ManifestBase{}
	if err := manifest.UnmarshalBinary(b); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &manifest, nil
}

// end evrManifest definition

// note: i have a sneaking suspicion that there's only one manifest version.
// the ones i've looked at so far can either be extracted by 5932408047-LE2 or 5932408047-EVR
// i think i remember being told this but i need to do more research

// every manifest version will be defined in it's own file
// each file should have functions to convert from evrManifest to it's type, and vice versa
// each file should also have a function to read and write itself to []byte

type manifestConverter interface {
	evrmFromBytes(data []byte) (ManifestBase, error)
	bytesFromEvrm(m ManifestBase) ([]byte, error)
}

/*
// this should take given manifestType and manifest []byte data, and call the appropriate function for that type, and return the result
func MarshalManifest(data []byte, manifestType string) (EvrManifest, error) {
	var converter manifestConverter

	// switch based on manifestType
	switch manifestType {
	case "5932408047-LE2":
		converter = manifest_5932408047_LE2{}
	case "5932408047-EVR":
		converter = Manifest5932408047{}
	case "5868485946-EVR":
		converter = manifest_5868485946_EVR{}
	default:
		return EvrManifest{}, errors.New("unimplemented manifest type")
	}

	return converter.evrmFromBytes(data)
}

func UnmarshalManifest(m EvrManifest, manifestType string) ([]byte, error) {
	switch manifestType {
	case "5932408047-LE2":
		m5932408047_LE2 := manifest_5932408047_LE2{}
		return m5932408047_LE2.bytesFromEvrm(m)
	case "5932408047-EVR":
		m5932408047_EVR := Manifest5932408047{}
		return m5932408047_EVR.bytesFromEvrm(m)
	//case "5868485946-EVR":
	//	m5868485946_EVR := manifest_5868485946_EVR{}
	//	return m5868485946_EVR.bytesFromEvrm(m)
	default:
		return nil, errors.New("unimplemented manifest type")
	}
}

*/
