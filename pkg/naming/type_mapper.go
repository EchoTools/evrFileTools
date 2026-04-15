package naming

import "fmt"

// TypeSymbol represents a file type identifier in EVR packages.
type TypeSymbol int64

// Known type symbols for EVR assets.
const (
	TypeDDSTexture      = TypeSymbol(-4706379568332879927) // 0xbeac1969cb7b8861
	TypeRawBCTexture    = TypeSymbol(9152405269835556869)  // 0x7f5bc1cf8ce51ffd
	TypeTextureMetadata = TypeSymbol(3397970254627897141)  // 0x2f6e61706a2c8f35
	TypeAudioReference  = TypeSymbol(4049816316449263978)  // 0x38ee951a26fb816a
	TypeAssetReference  = TypeSymbol(-3860481509838504953) // 0xca6cd085401cbc87
)

// TypeName returns a human-readable name for a type symbol.
func (ts TypeSymbol) String() string {
	switch ts {
	case TypeDDSTexture:
		return "texture_dds"
	case TypeRawBCTexture:
		return "texture_bc_raw"
	case TypeTextureMetadata:
		return "texture_meta"
	case TypeAudioReference:
		return "audio_ref"
	case TypeAssetReference:
		return "asset_ref"
	default:
		return fmt.Sprintf("unknown_0x%016x", uint64(ts))
	}
}

// IsTextureFormat returns true if the type is texture-related.
func IsTextureFormat(ts TypeSymbol) bool {
	return ts == TypeDDSTexture || ts == TypeRawBCTexture || ts == TypeTextureMetadata
}
