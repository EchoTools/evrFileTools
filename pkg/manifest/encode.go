package manifest

import (
	"encoding/binary"
	"fmt"

	"github.com/EchoTools/evrFileTools/pkg/naming"
)

// encodeFile prepares source-file bytes for packing.
//
// Most files are packed as-is. The one special case is TypeRawBCTexture: when
// the source was extracted as a .dds file (DDS header prepended by decodeRawBCTexture),
// we strip the header to recover the original raw BC payload before compression.
func encodeFile(data []byte, typeSymbol int64) ([]byte, error) {
	if naming.TypeSymbol(typeSymbol) == naming.TypeRawBCTexture {
		return stripDDSHeader(data)
	}
	return data, nil
}

// stripDDSHeader removes the DDS file header from DDS data, returning the pixel data.
// If the data does not start with the DDS magic bytes it is returned unchanged
// (already raw, or an unknown format the user swapped in).
func stripDDSHeader(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("data too short")
	}
	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != 0x20534444 { // "DDS "
		// Not a DDS file — return as-is. The user may have replaced it with raw BC.
		return data, nil
	}
	if len(data) < 128 {
		return nil, fmt.Errorf("DDS data too short for header (%d bytes)", len(data))
	}
	// 4 magic + 124 DDS_HEADER = 128 bytes baseline.
	// If pixel format FourCC == "DX10" (0x30315844) there is a 20-byte extension.
	fourCC := binary.LittleEndian.Uint32(data[84:88])
	headerSize := 128
	if fourCC == 0x30315844 {
		headerSize = 148
	}
	if len(data) <= headerSize {
		return nil, fmt.Errorf("DDS file has no pixel data after %d-byte header", headerSize)
	}
	return data[headerSize:], nil
}
