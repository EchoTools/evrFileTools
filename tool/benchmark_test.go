package tool

import (
	"bytes"
	"testing"

	"github.com/DataDog/zstd"
)

// BenchmarkZstdDecompressWithContext benchmarks zstd decompression with context reuse
func BenchmarkZstdDecompressWithContext(b *testing.B) {
	// Create test data
	original := make([]byte, 64*1024) // 64KB of data
	for i := range original {
		original[i] = byte(i % 256)
	}

	compressed, err := zstd.Compress(nil, original)
	if err != nil {
		b.Fatalf("failed to compress test data: %v", err)
	}

	b.Run("WithoutContext", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := zstd.Decompress(nil, compressed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithContext", func(b *testing.B) {
		ctx := zstd.NewCtx()
		dst := make([]byte, len(original))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := ctx.Decompress(dst, compressed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("WithContextReuseDst", func(b *testing.B) {
		ctx := zstd.NewCtx()
		dst := make([]byte, len(original))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := ctx.Decompress(dst[:0], compressed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkZstdCompressLevels benchmarks different compression levels
func BenchmarkZstdCompressLevels(b *testing.B) {
	// Create test data simulating real file content
	original := make([]byte, 256*1024) // 256KB
	for i := range original {
		original[i] = byte(i % 256)
	}

	levels := []int{
		zstd.BestSpeed,
		zstd.DefaultCompression,
		3,
		6,
	}

	for _, level := range levels {
		b.Run("Level_"+levelName(level), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := zstd.CompressLevel(nil, original, level)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func levelName(level int) string {
	switch level {
	case zstd.BestSpeed:
		return "BestSpeed"
	case zstd.DefaultCompression:
		return "Default"
	default:
		return string(rune('0' + level))
	}
}

// BenchmarkBufferAllocation benchmarks buffer allocation strategies
func BenchmarkBufferAllocation(b *testing.B) {
	size := 32 * 1024 * 1024 // 32MB

	b.Run("NewAllocation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := make([]byte, size)
			_ = buf
		}
	})

	b.Run("ReuseBuffer", func(b *testing.B) {
		buf := make([]byte, size)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulating reuse by clearing
			for j := range buf {
				buf[j] = 0
			}
		}
	})

	b.Run("BytesBuffer", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := bytes.NewBuffer(make([]byte, 0, size))
			_ = buf
		}
	})

	b.Run("BytesBufferReuse", func(b *testing.B) {
		buf := bytes.NewBuffer(make([]byte, 0, size))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf.Reset()
		}
	})
}

// BenchmarkManifestMarshal benchmarks manifest marshaling
func BenchmarkManifestMarshal(b *testing.B) {
	// Create a test manifest with realistic size
	manifest := &ManifestBase{
		Header: ManifestHeader{
			PackageCount: 3,
		},
		FrameContents: make([]FrameContents, 10000),
		SomeStructure: make([]SomeStructure, 10000),
		Frames:        make([]Frame, 500),
	}

	// Fill with test data
	for i := range manifest.FrameContents {
		manifest.FrameContents[i] = FrameContents{
			T:          int64(i % 100),
			FileSymbol: int64(i),
			FileIndex:  uint32(i % 500),
			DataOffset: uint32(i * 1024),
			Size:       1024,
		}
	}

	for i := range manifest.SomeStructure {
		manifest.SomeStructure[i] = SomeStructure{
			T:          int64(i % 100),
			FileSymbol: int64(i),
		}
	}

	for i := range manifest.Frames {
		manifest.Frames[i] = Frame{
			Index:          uint32(i % 3),
			Offset:         uint32(i * 65536),
			CompressedSize: 32768,
			Length:         65536,
		}
	}

	b.Run("MarshalBinary", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := manifest.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	// First marshal to get bytes for unmarshal benchmark
	data, _ := manifest.MarshalBinary()

	b.Run("UnmarshalBinary", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			m := &ManifestBase{}
			err := m.UnmarshalBinary(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkArchiveHeader benchmarks archive header operations
func BenchmarkArchiveHeader(b *testing.B) {
	header := ArchiveHeader{
		Magic:            [4]byte{0x5a, 0x53, 0x54, 0x44},
		HeaderLength:     16,
		Length:           1024 * 1024,
		CompressedLength: 512 * 1024,
	}

	b.Run("MarshalBinary", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := header.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	data, _ := header.MarshalBinary()

	b.Run("UnmarshalBinary", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			h := &ArchiveHeader{}
			err := h.UnmarshalBinary(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkLookupTable benchmarks different lookup key strategies
func BenchmarkLookupTable(b *testing.B) {
	const entries = 10000

	// Strategy 1: [128]byte key (current implementation)
	b.Run("ByteArrayKey", func(b *testing.B) {
		table := make(map[[16]byte]int, entries)
		for i := 0; i < entries; i++ {
			var key [16]byte
			key[0] = byte(i)
			key[8] = byte(i >> 8)
			table[key] = i
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var key [16]byte
			key[0] = byte(i % entries)
			key[8] = byte((i % entries) >> 8)
			_ = table[key]
		}
	})

	// Strategy 2: struct key
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

	// Strategy 3: string key
	b.Run("StringKey", func(b *testing.B) {
		table := make(map[string]int, entries)
		for i := 0; i < entries; i++ {
			key := string(rune(i)) + ":" + string(rune(i*2))
			table[key] = i
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			idx := i % entries
			key := string(rune(idx)) + ":" + string(rune(idx*2))
			_ = table[key]
		}
	})
}
