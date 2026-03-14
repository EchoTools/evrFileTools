package naming

import (
	"testing"
)

func TestNewAssetNameMapper(t *testing.T) {
	m := NewAssetNameMapper()
	if m == nil {
		t.Fatal("NewAssetNameMapper() returned nil")
	}

	// Should have some known assets
	if m.KnownSymbolCount() == 0 {
		t.Error("NewAssetNameMapper() has no known assets")
	}
}

func TestGetName(t *testing.T) {
	m := NewAssetNameMapper()

	// Test known symbol
	knownSym := int64(0x43e2da7914642604)
	name := m.GetName(knownSym)
	if name != "social_2.0_arena" {
		t.Errorf("GetName(%d) = %q, want 'social_2.0_arena'", knownSym, name)
	}

	// Test unknown symbol
	unknownSym := int64(0x1234567890abcdef)
	name = m.GetName(unknownSym)
	if name == "social_2.0_arena" {
		t.Error("GetName() returned known name for unknown symbol")
	}
}

func TestHasName(t *testing.T) {
	m := NewAssetNameMapper()

	knownSym := int64(0x43e2da7914642604)
	if !m.HasName(knownSym) {
		t.Errorf("HasName(%d) = false, want true", knownSym)
	}

	unknownSym := int64(0x1234567890abcdef)
	if m.HasName(unknownSym) {
		t.Errorf("HasName(%d) = true, want false", unknownSym)
	}
}

func TestGetSymbol(t *testing.T) {
	m := NewAssetNameMapper()

	// Test known name
	name := "social_2.0_arena"
	sym := m.GetSymbol(name)
	if sym != 0x43e2da7914642604 {
		t.Errorf("GetSymbol(%q) = 0x%016x, want 0x43e2da7914642604", name, sym)
	}

	// Test unknown name
	unknownName := "totally_unknown_asset"
	sym = m.GetSymbol(unknownName)
	if sym != 0 {
		t.Errorf("GetSymbol(%q) = 0x%016x, want 0", unknownName, sym)
	}
}

func TestAddMapping(t *testing.T) {
	m := NewAssetNameMapper()
	initialCount := m.KnownSymbolCount()

	// Add new mapping (use a uint64 that fits in int64)
	testSym := int64(0x123456789abcdef0)
	testName := "test_asset"
	m.AddMapping(testSym, testName)

	// Verify it was added
	if !m.HasName(testSym) {
		t.Errorf("AddMapping() failed - symbol not found")
	}

	if name := m.GetName(testSym); name != testName {
		t.Errorf("GetName() after AddMapping = %q, want %q", name, testName)
	}

	// Verify reverse lookup
	if sym := m.GetSymbol(testName); sym != testSym {
		t.Errorf("GetSymbol() after AddMapping = 0x%016x, want 0x%016x", sym, testSym)
	}

	// Should have one more known symbol
	if m.KnownSymbolCount() != initialCount+1 {
		t.Errorf("KnownSymbolCount() = %d, want %d", m.KnownSymbolCount(), initialCount+1)
	}
}

func TestAddMappings(t *testing.T) {
	m := NewAssetNameMapper()
	initialCount := m.KnownSymbolCount()

	// Add multiple mappings
	mappings := map[int64]string{
		0x1111111111111111: "test_asset_1",
		0x2222222222222222: "test_asset_2",
		0x3333333333333333: "test_asset_3",
	}

	m.AddMappings(mappings)

	// Verify all were added
	if m.KnownSymbolCount() != initialCount+3 {
		t.Errorf("KnownSymbolCount() = %d, want %d", m.KnownSymbolCount(), initialCount+3)
	}

	for sym, expectedName := range mappings {
		if name := m.GetName(sym); name != expectedName {
			t.Errorf("GetName(0x%016x) = %q, want %q", sym, name, expectedName)
		}
	}
}

func TestGlobalAssetName(t *testing.T) {
	// Test global functions
	knownSym := int64(0x43e2da7914642604)

	if !HasAssetName(knownSym) {
		t.Errorf("HasAssetName(%d) = false, want true", knownSym)
	}

	name := GetAssetName(knownSym)
	if name != "social_2.0_arena" {
		t.Errorf("GetAssetName(%d) = %q, want 'social_2.0_arena'", knownSym, name)
	}
}

func TestAddGlobalMapping(t *testing.T) {
	testSym := int64(0x7777777777777777)
	testName := "global_test_asset"

	AddGlobalMapping(testSym, testName)

	if !HasAssetName(testSym) {
		t.Errorf("AddGlobalMapping() failed")
	}

	if name := GetAssetName(testSym); name != testName {
		t.Errorf("GetAssetName() = %q, want %q", name, testName)
	}
}

func TestFormatHex(t *testing.T) {
	tests := []struct {
		value    uint64
		expected string
	}{
		{0x0, "0000000000000000"},
		{0xf, "000000000000000f"},
		{0xff, "00000000000000ff"},
		{0x123456789abcdef0, "123456789abcdef0"},
		{0xffffffffffffffff, "ffffffffffffffff"},
	}

	for _, tt := range tests {
		if got := formatHex(tt.value); got != tt.expected {
			t.Errorf("formatHex(0x%x) = %q, want %q", tt.value, got, tt.expected)
		}
	}
}
