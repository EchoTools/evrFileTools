package manifest

import (
	"fmt"
	"strconv"
	"testing"
)

// BenchmarkManifest benchmarks manifest operations.
func BenchmarkManifest(b *testing.B) {
	// Create a realistic manifest
	manifest := &Manifest{
		Header: Header{
			PackageCount: 3,
			FrameContents: Section{
				ElementSize: 32,
			},
			Metadata: Section{
				ElementSize: 40,
			},
			Frames: Section{
				ElementSize: 16,
			},
		},
		FrameContents: make([]FrameContent, 10000),
		Metadata:      make([]FileMetadata, 10000),
		Frames:        make([]Frame, 500),
	}

	// Fill with test data
	for i := range manifest.FrameContents {
		manifest.FrameContents[i] = FrameContent{
			TypeSymbol: int64(i % 100),
			FileSymbol: int64(i),
			FrameIndex: uint32(i % 500),
			DataOffset: uint32(i * 1024),
			Size:       1024,
			Alignment:  1,
		}
	}

	for i := range manifest.Metadata {
		manifest.Metadata[i] = FileMetadata{
			TypeSymbol: int64(i % 100),
			FileSymbol: int64(i),
		}
	}

	for i := range manifest.Frames {
		manifest.Frames[i] = Frame{
			PackageIndex:   uint32(i % 3),
			Offset:         uint32(i * 65536),
			CompressedSize: 32768,
			Length:         65536,
		}
	}

	// Update header sections
	manifest.Header.FrameContents.Count = uint64(len(manifest.FrameContents))
	manifest.Header.FrameContents.ElementCount = uint64(len(manifest.FrameContents))
	manifest.Header.FrameContents.Length = uint64(len(manifest.FrameContents)) * 32

	manifest.Header.Metadata.Count = uint64(len(manifest.Metadata))
	manifest.Header.Metadata.ElementCount = uint64(len(manifest.Metadata))
	manifest.Header.Metadata.Length = uint64(len(manifest.Metadata)) * 40

	manifest.Header.Frames.Count = uint64(len(manifest.Frames))
	manifest.Header.Frames.ElementCount = uint64(len(manifest.Frames))
	manifest.Header.Frames.Length = uint64(len(manifest.Frames)) * 16

	b.Run("Marshal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := manifest.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	data, _ := manifest.MarshalBinary()

	b.Run("Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := &Manifest{}
			err := m.UnmarshalBinary(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkLookupStrategies benchmarks different lookup key strategies.
func BenchmarkLookupStrategies(b *testing.B) {
	const entries = 10000

	// Strategy 1: Struct key (recommended)
	type symbolKey struct {
		typeSymbol int64
		fileSymbol int64
	}

	b.Run("StructKey", func(b *testing.B) {
		table := make(map[symbolKey]int, entries)
		for i := 0; i < entries; i++ {
			table[symbolKey{int64(i), int64(i * 2)}] = i
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % entries
			_ = table[symbolKey{int64(idx), int64(idx * 2)}]
		}
	})

	// Strategy 2: Combined int64 key
	b.Run("CombinedInt64Key", func(b *testing.B) {
		table := make(map[uint64]int, entries)
		for i := 0; i < entries; i++ {
			key := uint64(i)<<32 | uint64(i*2)&0xFFFFFFFF
			table[key] = i
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % entries
			key := uint64(idx)<<32 | uint64(idx*2)&0xFFFFFFFF
			_ = table[key]
		}
	})
}

// BenchmarkFrameIndex benchmarks frame content lookup strategies.
func BenchmarkFrameIndex(b *testing.B) {
	// Simulate 10000 files across 500 frames
	frameContents := make([]FrameContent, 10000)
	for i := range frameContents {
		frameContents[i] = FrameContent{
			TypeSymbol: int64(i % 100),
			FileSymbol: int64(i),
			FrameIndex: uint32(i % 500),
		}
	}

	b.Run("LinearScan", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			frameIdx := uint32(i % 500)
			count := 0
			for _, fc := range frameContents {
				if fc.FrameIndex == frameIdx {
					count++
				}
			}
		}
	})

	b.Run("PrebuiltIndex", func(b *testing.B) {
		// Build index once
		frameIndex := make(map[uint32][]FrameContent)
		for _, fc := range frameContents {
			frameIndex[fc.FrameIndex] = append(frameIndex[fc.FrameIndex], fc)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			frameIdx := uint32(i % 500)
			_ = frameIndex[frameIdx]
		}
	})
}

// BenchmarkHexFormatting benchmarks hex string formatting strategies.
func BenchmarkHexFormatting(b *testing.B) {
	symbols := make([]int64, 1000)
	for i := range symbols {
		symbols[i] = int64(i * 12345678)
	}

	b.Run("Sprintf", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = fmt.Sprintf("%x", symbols[i%len(symbols)])
		}
	})

	b.Run("FormatInt", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = strconv.FormatInt(symbols[i%len(symbols)], 16)
		}
	})
}
