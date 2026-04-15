package manifest

import (
	"encoding/binary"
	"fmt"
	"github.com/EchoTools/evrFileTools/pkg/naming"
)

// encodeFile prepares source-file bytes for packing.
//
// Most files are packed as-is. The one special case is TypeRawBCTexture: when
// the source was extracted as a .dds file (DDS header prepended on extract),
// we strip the header to recover the original raw BC payload before compression.
// TypeDDSTexture files are stored in the package WITH their DDS header and must
// NOT be stripped.
func encodeFile(data []byte, typeSymbol int64) ([]byte, error) {
	if naming.TypeSymbol(typeSymbol) == naming.TypeRawBCTexture {
		return stripDDSHeader(data)
	}
	return data, nil
}

// stripDDSHeader removes the DDS file header from DDS data, returning the pixel data.
// If the data does not start with the DDS magic bytes it is returned unchanged.
func stripDDSHeader(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return data, nil
	}
	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != 0x20534444 { // "DDS "
		return data, nil
	}
	if len(data) < 128 {
		return nil, fmt.Errorf("DDS data too short for header (%d bytes)", len(data))
	}
	fourCC := binary.LittleEndian.Uint32(data[84:88])
	headerSize := 128
	if fourCC == 0x30315844 { // "DX10"
		headerSize = 148
	}
	if len(data) <= headerSize {
		return nil, fmt.Errorf("DDS file has no pixel data after header")
	}
	return data[headerSize:], nil
}
