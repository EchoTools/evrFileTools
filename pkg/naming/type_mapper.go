// Package naming provides type symbol and asset name mappings.
package naming

import "fmt"

// TypeSymbol represents a file type identifier in EVR packages.
// Uses int64 to match manifest.FrameContent.TypeSymbol
type TypeSymbol int64

// Known type symbols for EVR assets (stored as int64 in manifests).
const (
	// Textures and related
	TypeDDSTexture      = TypeSymbol(-4706379568332879927) // 0xbeac1969cb7b8861
	TypeRawBCTexture    = TypeSymbol(9152405269835556869)  // 0x7f5bc1cf8ce51ffd
	TypeTextureMetadata = TypeSymbol(3397970254627897141)  // 0x2f6e61706a2c8f35

	// References
	TypeAudioReference = TypeSymbol(4049816316449263978)  // 0x38ee951a26fb816a
	TypeAssetReference = TypeSymbol(-3860481509838504953) // 0xca6cd085401cbc87
)

// TypeName returns a human-readable name for a type symbol.
func (ts TypeSymbol) String() string {
	return TypeName(ts)
}

// TypeName returns the name for a given type symbol.
func TypeName(typeSymbol TypeSymbol) string {
	switch typeSymbol {
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
		return fmt.Sprintf("unknown_0x%016x", int64(typeSymbol))
	}
}

// TypeCategory returns the category of a type for organizational purposes.
func TypeCategory(typeSymbol TypeSymbol) string {
	switch typeSymbol {
	case TypeDDSTexture, TypeRawBCTexture:
		return "textures"
	case TypeTextureMetadata:
		return "textures"
	case TypeAudioReference:
		return "audio"
	case TypeAssetReference:
		return "assets"
	default:
		return "data"
	}
}

// FileExtension returns a suggested file extension for a type.
func FileExtension(typeSymbol TypeSymbol) string {
	switch typeSymbol {
	case TypeDDSTexture:
		return ".dds"
	case TypeRawBCTexture:
		return ".bc"
	case TypeTextureMetadata:
		return ".meta"
	case TypeAudioReference:
		return ".aref"
	case TypeAssetReference:
		return ".aref"
	default:
		return ".bin"
	}
}

// IsTextureFormat returns true if the type is texture-related.
func IsTextureFormat(typeSymbol TypeSymbol) bool {
	return typeSymbol == TypeDDSTexture ||
		typeSymbol == TypeRawBCTexture ||
		typeSymbol == TypeTextureMetadata
}

// IsKnownType returns true if the type symbol is recognized.
func IsKnownType(typeSymbol TypeSymbol) bool {
	switch typeSymbol {
	case TypeDDSTexture, TypeRawBCTexture, TypeTextureMetadata,
		TypeAudioReference, TypeAssetReference:
		return true
	default:
		return false
	}
}

// AllKnownTypes returns a slice of all known type symbols.
func AllKnownTypes() []TypeSymbol {
	return []TypeSymbol{
		TypeDDSTexture,
		TypeRawBCTexture,
		TypeTextureMetadata,
		TypeAudioReference,
		TypeAssetReference,
	}
}
