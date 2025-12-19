package archive

import (
	"bytes"
	"testing"
)

func TestHeader(t *testing.T) {
	t.Run("MarshalUnmarshal", func(t *testing.T) {
		original := &Header{
			Magic:            Magic,
			HeaderLength:     16,
			Length:           1024,
			CompressedLength: 512,
		}

		data, err := original.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		decoded := &Header{}
		if err := decoded.UnmarshalBinary(data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if *decoded != *original {
			t.Errorf("mismatch: got %+v, want %+v", decoded, original)
		}
	})

	t.Run("InvalidMagic", func(t *testing.T) {
		h := &Header{
			Magic:            [4]byte{0x00, 0x00, 0x00, 0x00},
			HeaderLength:     16,
			Length:           1024,
			CompressedLength: 512,
		}
		if err := h.Validate(); err == nil {
			t.Error("expected error for invalid magic")
		}
	})

	t.Run("ZeroLength", func(t *testing.T) {
		h := &Header{
			Magic:            Magic,
			HeaderLength:     16,
			Length:           0,
			CompressedLength: 512,
		}
		if err := h.Validate(); err == nil {
			t.Error("expected error for zero length")
		}
	})
}

func TestReadWrite(t *testing.T) {
	original := []byte("Hello, World! This is test data for compression.")

	t.Run("EncodeDecodeRoundTrip", func(t *testing.T) {
		var buf bytes.Buffer

		ws := &seekableBuffer{Buffer: &buf}

		if err := Encode(ws, original); err != nil {
			t.Fatalf("encode: %v", err)
		}

		rs := bytes.NewReader(buf.Bytes())
		decoded, err := ReadAll(rs)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		if !bytes.Equal(decoded, original) {
			t.Errorf("data mismatch: got %q, want %q", decoded, original)
		}
	})
}

type seekableBuffer struct {
	*bytes.Buffer
	pos int64
}

func (s *seekableBuffer) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case 0:
		newPos = offset
	case 1:
		newPos = s.pos + offset
	case 2:
		newPos = int64(s.Buffer.Len()) + offset
	}
	s.pos = newPos
	return newPos, nil
}

func (s *seekableBuffer) Write(p []byte) (n int, err error) {
	for int64(s.Buffer.Len()) < s.pos {
		s.Buffer.WriteByte(0)
	}
	if s.pos < int64(s.Buffer.Len()) {
		data := s.Buffer.Bytes()
		n = copy(data[s.pos:], p)
		if n < len(p) {
			m, err := s.Buffer.Write(p[n:])
			n += m
			if err != nil {
				return n, err
			}
		}
	} else {
		n, err = s.Buffer.Write(p)
	}
	s.pos += int64(n)
	return n, err
}
