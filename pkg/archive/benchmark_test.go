package archive

import (
	"bytes"
	"testing"

	"github.com/DataDog/zstd"
)

// BenchmarkCompression benchmarks compression with different configurations.
func BenchmarkCompression(b *testing.B) {
	data := make([]byte, 256*1024) // 256KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.Run("Compress_BestSpeed", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := zstd.CompressLevel(nil, data, zstd.BestSpeed)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Compress_Default", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := zstd.CompressLevel(nil, data, zstd.DefaultCompression)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDecompression benchmarks decompression with context reuse.
func BenchmarkDecompression(b *testing.B) {
	original := make([]byte, 64*1024) // 64KB
	for i := range original {
		original[i] = byte(i % 256)
	}

	compressed, _ := zstd.Compress(nil, original)

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
}

// BenchmarkHeader benchmarks header operations.
func BenchmarkHeader(b *testing.B) {
	header := &Header{
		Magic:            Magic,
		HeaderLength:     16,
		Length:           1024 * 1024,
		CompressedLength: 512 * 1024,
	}

	b.Run("Marshal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := header.MarshalBinary()
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("EncodeTo", func(b *testing.B) {
		buf := make([]byte, HeaderSize)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			header.EncodeTo(buf)
		}
	})

	data, _ := header.MarshalBinary()

	b.Run("Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			h := &Header{}
			err := h.UnmarshalBinary(data)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DecodeFrom", func(b *testing.B) {
		h := &Header{}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			h.DecodeFrom(data)
		}
	})
}

// BenchmarkEncodeDecode benchmarks full encode/decode cycle.
func BenchmarkEncodeDecode(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.Run("Encode", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var buf bytes.Buffer
			ws := &benchSeekableBuffer{Buffer: &buf}
			if err := Encode(ws, data); err != nil {
				b.Fatal(err)
			}
		}
	})

	// Pre-encode for decode benchmark
	var buf bytes.Buffer
	ws := &benchSeekableBuffer{Buffer: &buf}
	_ = Encode(ws, data)
	encoded := buf.Bytes()

	b.Run("Decode", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			rs := bytes.NewReader(encoded)
			_, err := ReadAll(rs)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

type benchSeekableBuffer struct {
	*bytes.Buffer
	pos int64
}

func (s *benchSeekableBuffer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		s.pos = offset
	case 1:
		s.pos += offset
	case 2:
		s.pos = int64(s.Buffer.Len()) + offset
	}
	return s.pos, nil
}

func (s *benchSeekableBuffer) Write(p []byte) (n int, err error) {
	for int64(s.Buffer.Len()) < s.pos {
		s.Buffer.WriteByte(0)
	}
	if s.pos < int64(s.Buffer.Len()) {
		data := s.Buffer.Bytes()
		n = copy(data[s.pos:], p)
		if n < len(p) {
			m, _ := s.Buffer.Write(p[n:])
			n += m
		}
	} else {
		n, err = s.Buffer.Write(p)
	}
	s.pos += int64(n)
	return n, err
}
