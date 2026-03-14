// Package naming provides asset name mappings for EVR assets.
package naming

// AssetNameMapper maps file symbols to human-readable asset names.
// This is populated with known assets and can be extended.
type AssetNameMapper struct {
	symbolToName map[int64]string
	nameToSymbol map[string]int64
}

// NewAssetNameMapper creates a new mapper with known assets.
func NewAssetNameMapper() *AssetNameMapper {
	m := &AssetNameMapper{
		symbolToName: make(map[int64]string),
		nameToSymbol: make(map[string]int64),
	}
	m.initializeKnownAssets()
	return m
}

// initializeKnownAssets populates the mapper with known asset names.
// These are discovered from game analysis and documentation.
func (m *AssetNameMapper) initializeKnownAssets() {
	// Level/Environment Assets (from game strings and analysis)
	// Store as uint64 then convert to int64 to handle the sign bit properly
	known := []struct {
		symbol uint64
		name   string
	}{
		// Social/Lobby Level
		{0x43e2da7914642604, "social_2.0_arena"},
		{0x43e2da7a0c623a19, "social_2.0_private"},
		{0xd09afd15b1c75c04, "social_2.0_npe"},

		// Common texture references
		{0xdf5ca7b7dfa383d4, "arena_environment"},
		{0xcb9977f7fc2b4526, "lobby_environment"},
		{0x576ed3f8428ebc4b, "courtyard_environment"},
		{0x3f9915d3001dc28e, "environment_lighting"},
		{0x3c8d74713ced8c3f, "environment_props"},
		{0xac360e41e4ede056, "environment_decals"},
		{0xe24c89df8235dd7a, "environment_particles"},
		{0x4d82118c7c91b6bb, "environment_effects"},

		// Note: Additional mappings can be added as they are discovered
		// through game binary analysis and community research
	}

	for _, item := range known {
		sym := int64(item.symbol)
		m.symbolToName[sym] = item.name
		m.nameToSymbol[item.name] = sym
	}
}

// GetName returns the name for a file symbol, or a fallback if unknown.
// If the symbol is unknown, returns a hex representation.
func (m *AssetNameMapper) GetName(fileSymbol int64) string {
	if name, ok := m.symbolToName[fileSymbol]; ok {
		return name
	}
	// Fallback: return hex representation
	return formatHexSymbol(fileSymbol)
}

// HasName returns true if a name is known for the given symbol.
func (m *AssetNameMapper) HasName(fileSymbol int64) bool {
	_, ok := m.symbolToName[fileSymbol]
	return ok
}

// GetSymbol returns the symbol for a known name, or 0 if not found.
func (m *AssetNameMapper) GetSymbol(name string) int64 {
	if sym, ok := m.nameToSymbol[name]; ok {
		return sym
	}
	return 0
}

// AddMapping adds or updates an asset name mapping.
func (m *AssetNameMapper) AddMapping(fileSymbol int64, name string) {
	m.symbolToName[fileSymbol] = name
	m.nameToSymbol[name] = fileSymbol
}

// AddMappings adds multiple asset name mappings at once.
func (m *AssetNameMapper) AddMappings(mappings map[int64]string) {
	for sym, name := range mappings {
		m.AddMapping(sym, name)
	}
}

// KnownSymbolCount returns the number of known symbol mappings.
func (m *AssetNameMapper) KnownSymbolCount() int {
	return len(m.symbolToName)
}

// formatHexSymbol returns a hex representation of a symbol.
func formatHexSymbol(symbol int64) string {
	return formatHex(uint64(symbol))
}

// formatHex returns a hex string representation of a uint64.
// This is intentionally simple - returns the 16-digit hex value.
func formatHex(value uint64) string {
	const hexChars = "0123456789abcdef"
	var result [16]byte

	for i := 15; i >= 0; i-- {
		result[i] = hexChars[value&0xf]
		value >>= 4
	}

	return string(result[:])
}

// GlobalAssetMapper is the default global mapper instance.
var globalAssetMapper = NewAssetNameMapper()

// GetAssetName returns the name for a file symbol using the global mapper.
func GetAssetName(fileSymbol int64) string {
	return globalAssetMapper.GetName(fileSymbol)
}

// HasAssetName returns true if a name is known using the global mapper.
func HasAssetName(fileSymbol int64) bool {
	return globalAssetMapper.HasName(fileSymbol)
}

// AddGlobalMapping adds a mapping to the global mapper.
func AddGlobalMapping(fileSymbol int64, name string) {
	globalAssetMapper.AddMapping(fileSymbol, name)
}

// AddGlobalMappings adds multiple mappings to the global mapper.
func AddGlobalMappings(mappings map[int64]string) {
	globalAssetMapper.AddMappings(mappings)
}
