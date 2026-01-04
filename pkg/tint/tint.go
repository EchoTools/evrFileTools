// Package tint provides structures and utilities for Echo VR tint assets.
//
// Tints are cosmetic color schemes applied to player chassis. They consist of
// multiple color values (RGBA floats) that are stored within CR15NetRewardItemCS
// component data, not as separate asset files.
//
// Based on Ghidra analysis of echovr.exe:
// - Tint registration: CR15NetRewardItemCS_RegisterTint @ 0x140cf23c0
// - Tint override: R15NetCustomization_OverrideTint @ 0x140d20710
// - Binary search: CSymbolTable_BinarySearch @ 0x140c682f0
//
// Global tint tables at runtime:
// - g_TintTable_ItemIDs @ 0x1420d3ac0 (primary lookup by resourceID)
// - g_TintTable_Secondary @ 0x1420d3ac8 (secondary tint values)
// - g_TintTable_Tertiary @ 0x1420d3ad0 (tertiary tint values)
package tint

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Color represents an RGBA color with float32 components (0.0-1.0).
type Color struct {
	R, G, B, A float32
}

// ColorFromBytes reads a Color from 16 bytes (4 float32s in little-endian).
func ColorFromBytes(data []byte) Color {
	if len(data) < 16 {
		return Color{}
	}
	return Color{
		R: math.Float32frombits(binary.LittleEndian.Uint32(data[0:4])),
		G: math.Float32frombits(binary.LittleEndian.Uint32(data[4:8])),
		B: math.Float32frombits(binary.LittleEndian.Uint32(data[8:12])),
		A: math.Float32frombits(binary.LittleEndian.Uint32(data[12:16])),
	}
}

// ToBytes writes a Color to 16 bytes (4 float32s in little-endian).
func (c Color) ToBytes() []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(c.R))
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(c.G))
	binary.LittleEndian.PutUint32(data[8:12], math.Float32bits(c.B))
	binary.LittleEndian.PutUint32(data[12:16], math.Float32bits(c.A))
	return data
}

// String returns a human-readable color representation.
func (c Color) String() string {
	return fmt.Sprintf("RGBA(%.3f, %.3f, %.3f, %.3f)", c.R, c.G, c.B, c.A)
}

// Hex returns the color as a hex string (#RRGGBBAA).
func (c Color) Hex() string {
	r := uint8(clamp(c.R, 0, 1) * 255)
	g := uint8(clamp(c.G, 0, 1) * 255)
	b := uint8(clamp(c.B, 0, 1) * 255)
	a := uint8(clamp(c.A, 0, 1) * 255)
	return fmt.Sprintf("#%02X%02X%02X%02X", r, g, b, a)
}

func clamp(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// TintEntry represents a tint item as stored in CR15NetRewardItemCS component.
// Each entry is 0x60 (96) bytes containing:
// - Symbol64 resourceID at offset 0 (8 bytes)
// - 6 color blocks of 16 bytes each at offsets +0x8, +0x18, +0x28, +0x38, +0x48, +0x58
//
// The 6 color blocks likely represent:
// - Main color and accent color for 3 tint slots (alignment_a, alignment_b, body)
// Or:
// - 2 colors (main/accent) × 3 material layers
type TintEntry struct {
	ResourceID uint64   // Symbol64 hash identifying this tint
	Colors     [6]Color // 6 RGBA color values
}

// TintEntrySize is the size of a TintEntry in bytes (0x60 = 96).
const TintEntrySize = 0x60

// ReadTintEntry reads a TintEntry from binary data.
func ReadTintEntry(r io.Reader) (*TintEntry, error) {
	data := make([]byte, TintEntrySize)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}
	return TintEntryFromBytes(data), nil
}

// TintEntryFromBytes parses a TintEntry from 96 bytes.
func TintEntryFromBytes(data []byte) *TintEntry {
	if len(data) < TintEntrySize {
		return nil
	}
	entry := &TintEntry{
		ResourceID: binary.LittleEndian.Uint64(data[0:8]),
	}
	// Color blocks at offsets 0x08, 0x18, 0x28, 0x38, 0x48, 0x58
	offsets := []int{0x08, 0x18, 0x28, 0x38, 0x48, 0x58}
	for i, off := range offsets {
		entry.Colors[i] = ColorFromBytes(data[off : off+16])
	}
	return entry
}

// ToBytes serializes a TintEntry to 96 bytes.
func (e *TintEntry) ToBytes() []byte {
	data := make([]byte, TintEntrySize)
	binary.LittleEndian.PutUint64(data[0:8], e.ResourceID)
	offsets := []int{0x08, 0x18, 0x28, 0x38, 0x48, 0x58}
	for i, off := range offsets {
		copy(data[off:off+16], e.Colors[i].ToBytes())
	}
	return data
}

// String returns a human-readable representation of the tint entry.
func (e *TintEntry) String() string {
	return fmt.Sprintf("Tint[%016x]: Main=%s Accent=%s", e.ResourceID, e.Colors[0], e.Colors[1])
}

// TintTableEntry_Primary represents an entry in g_TintTable_ItemIDs (0x18 bytes).
// Used for binary search lookup by resourceID.
type TintTableEntry_Primary struct {
	ResourceID uint64 // Symbol64 hash for lookup
	ItemData   uint64 // Pointer to source data (runtime only)
	ItemIndex  uint32 // Index in reward item array
	Padding    uint32 // Alignment padding
}

// TintTableEntryPrimarySize is the size of a primary table entry (0x18 = 24).
const TintTableEntryPrimarySize = 0x18

// TintTableEntry_Secondary represents an entry in g_TintTable_Secondary/Tertiary (0x20 bytes).
type TintTableEntry_Secondary struct {
	TintValue  uint64 // Tint color value or Symbol64
	ResourceID uint64 // Symbol64 hash
	ItemData   uint64 // Pointer to source data (runtime only)
	Flags      uint64 // Flags or metadata
}

// TintTableEntrySecondarySize is the size of a secondary table entry (0x20 = 32).
const TintTableEntrySecondarySize = 0x20

// KnownTints maps Symbol64 hashes to tint names (from nakama server data).
var KnownTints = map[uint64]string{
	0x0bf4c0e4d2eaa06c: "rwd_tint_s2_a_default",
	0x19b3ed5723fadbda: "orange_tint_tab_seen",
	0x19f7bd5fef66c280: "pattern_tint_0",
	0x1c73d6d8e28c446e: "pattern_tint_100",
	0x3e474b60a9416aca: "rwd_tint_s1_a_default",
	0x43ac219540f9df74: "rwd_tint_s1_b_default",
	0x5d468c4263c586b8: "rwd_tint_s2_c_default",
	0x68f507c6186e4c1e: "rwd_tint_s1_c_default",
	0x69466570d92546d2: "layer2_albedo_tint_color",
	0x6f14fe299a8f9c02: "blue_tint_tab_seen",
	0x74d228d09dc5dc80: "rwd_tint_0006",
	0x74d228d09dc5dc81: "rwd_tint_0007",
	0x74d228d09dc5dc82: "rwd_tint_0004",
	0x74d228d09dc5dc83: "rwd_tint_0005",
	0x74d228d09dc5dc84: "rwd_tint_0002",
	0x74d228d09dc5dc85: "rwd_tint_0003",
	0x74d228d09dc5dc86: "rwd_tint_0000",
	0x74d228d09dc5dc87: "rwd_tint_0001",
	0x74d228d09dc5dc8e: "rwd_tint_0008",
	0x74d228d09dc5dc8f: "rwd_tint_0009",
	0x74d228d09dc5dd80: "rwd_tint_0016",
	0x74d228d09dc5dd81: "rwd_tint_0017",
	0x74d228d09dc5dd82: "rwd_tint_0014",
	0x74d228d09dc5dd83: "rwd_tint_0015",
	0x74d228d09dc5dd84: "rwd_tint_0012",
	0x74d228d09dc5dd85: "rwd_tint_0013",
	0x74d228d09dc5dd86: "rwd_tint_0010",
	0x74d228d09dc5dd87: "rwd_tint_0011",
	0x74d228d09dc5dd8e: "rwd_tint_0018",
	0x74d228d09dc5dd8f: "rwd_tint_0019",
	0x74d228d09dc5de80: "rwd_tint_0026",
	0x74d228d09dc5de81: "rwd_tint_0027",
	0x74d228d09dc5de82: "rwd_tint_0024",
	0x74d228d09dc5de83: "rwd_tint_0025",
	0x74d228d09dc5de84: "rwd_tint_0022",
	0x74d228d09dc5de85: "rwd_tint_0023",
	0x74d228d09dc5de86: "rwd_tint_0020",
	0x74d228d09dc5de87: "rwd_tint_0021",
	0x74d228d09dc5de8e: "rwd_tint_0028",
	0x74d228d09dc5de8f: "rwd_tint_0029",
	0x761faa113b5215d2: "rwd_tint_s2_b_default",
	0x80b08485fb665a7a: "layer1_albedo_tint_color",
	0xa11587a1254c9502: "rwd_tint_s3_tint_d",
	0xa11587a1254c9503: "rwd_tint_s3_tint_e",
	0xa11587a1254c9504: "rwd_tint_s3_tint_b",
	0xa11587a1254c9505: "rwd_tint_s3_tint_c",
	0xa11587a1254c9507: "rwd_tint_s3_tint_a",
	0xb87af47e9388b408: "rwd_tint_s1_d_default",
}

// LookupTintName returns the name for a tint Symbol64, or empty string if unknown.
func LookupTintName(symbol uint64) string {
	return KnownTints[symbol]
}
