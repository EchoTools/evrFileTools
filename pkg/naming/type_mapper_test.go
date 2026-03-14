package naming

import "testing"

func TestTypeNameDDSTexture(t *testing.T) {
	tests := []struct {
		symbol TypeSymbol
		want   string
	}{
		{TypeDDSTexture, "texture_dds"},
		{TypeRawBCTexture, "texture_bc_raw"},
		{TypeTextureMetadata, "texture_meta"},
		{TypeAudioReference, "audio_ref"},
		{TypeAssetReference, "asset_ref"},
	}

	for _, tt := range tests {
		if got := TypeName(tt.symbol); got != tt.want {
			t.Errorf("TypeName(%d) = %q, want %q", tt.symbol, got, tt.want)
		}
	}
}

func TestTypeCategory(t *testing.T) {
	tests := []struct {
		symbol   TypeSymbol
		category string
	}{
		{TypeDDSTexture, "textures"},
		{TypeRawBCTexture, "textures"},
		{TypeTextureMetadata, "textures"},
		{TypeAudioReference, "audio"},
		{TypeAssetReference, "assets"},
	}

	for _, tt := range tests {
		if got := TypeCategory(tt.symbol); got != tt.category {
			t.Errorf("TypeCategory(%d) = %q, want %q", tt.symbol, got, tt.category)
		}
	}
}

func TestFileExtension(t *testing.T) {
	tests := []struct {
		symbol    TypeSymbol
		extension string
	}{
		{TypeDDSTexture, ".dds"},
		{TypeRawBCTexture, ".dds"},
		{TypeTextureMetadata, ".tmeta"},
		{TypeAudioReference, ".aref"},
		{TypeAssetReference, ".aref"},
	}

	for _, tt := range tests {
		if got := FileExtension(tt.symbol); got != tt.extension {
			t.Errorf("FileExtension(%d) = %q, want %q", tt.symbol, got, tt.extension)
		}
	}
}

func TestIsTextureFormat(t *testing.T) {
	tests := []struct {
		symbol    TypeSymbol
		isTexture bool
	}{
		{TypeDDSTexture, true},
		{TypeRawBCTexture, true},
		{TypeTextureMetadata, true},
		{TypeAudioReference, false},
		{TypeAssetReference, false},
	}

	for _, tt := range tests {
		if got := IsTextureFormat(tt.symbol); got != tt.isTexture {
			t.Errorf("IsTextureFormat(%d) = %v, want %v", tt.symbol, got, tt.isTexture)
		}
	}
}

func TestIsKnownType(t *testing.T) {
	tests := []struct {
		symbol  TypeSymbol
		isKnown bool
	}{
		{TypeDDSTexture, true},
		{TypeRawBCTexture, true},
		{TypeTextureMetadata, true},
		{TypeAudioReference, true},
		{TypeAssetReference, true},
		{TypeSymbol(0), false},
		{TypeSymbol(12345), false},
	}

	for _, tt := range tests {
		if got := IsKnownType(tt.symbol); got != tt.isKnown {
			t.Errorf("IsKnownType(%d) = %v, want %v", tt.symbol, got, tt.isKnown)
		}
	}
}

func TestAllKnownTypes(t *testing.T) {
	types := AllKnownTypes()
	if len(types) != 5 {
		t.Errorf("AllKnownTypes() returned %d types, want 5", len(types))
	}

	expectedTypes := []TypeSymbol{
		TypeDDSTexture,
		TypeRawBCTexture,
		TypeTextureMetadata,
		TypeAudioReference,
		TypeAssetReference,
	}

	for i, expected := range expectedTypes {
		if i >= len(types) {
			t.Errorf("AllKnownTypes() missing type %d", expected)
			continue
		}
		if types[i] != expected {
			t.Errorf("AllKnownTypes()[%d] = %d, want %d", i, types[i], expected)
		}
	}
}

func TestTypeSymbolString(t *testing.T) {
	symbol := TypeDDSTexture
	expected := "texture_dds"
	if got := symbol.String(); got != expected {
		t.Errorf("TypeSymbol.String() = %q, want %q", got, expected)
	}
}
