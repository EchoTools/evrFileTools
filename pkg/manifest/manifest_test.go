package manifest

import (
	"testing"
)

func TestManifest(t *testing.T) {
	t.Run("MarshalUnmarshal", func(t *testing.T) {
		original := &Manifest{
			Header: Header{
				PackageCount: 2,
				FrameContents: Section{
					Length:       64,
					ElementSize:  32,
					Count:        2,
					ElementCount: 2,
				},
				Metadata: Section{
					Length:       80,
					ElementSize:  40,
					Count:        2,
					ElementCount: 2,
				},
				Frames: Section{
					Length:       32,
					ElementSize:  16,
					Count:        2,
					ElementCount: 2,
				},
			},
			FrameContents: []FrameContent{
				{TypeSymbol: 100, FileSymbol: 200, FrameIndex: 0, DataOffset: 0, Size: 1024, Alignment: 1},
				{TypeSymbol: 101, FileSymbol: 201, FrameIndex: 1, DataOffset: 0, Size: 2048, Alignment: 1},
			},
			Metadata: []FileMetadata{
				{TypeSymbol: 100, FileSymbol: 200},
				{TypeSymbol: 101, FileSymbol: 201},
			},
			Frames: []Frame{
				{PackageIndex: 0, Offset: 0, CompressedSize: 512, Length: 1024},
				{PackageIndex: 0, Offset: 512, CompressedSize: 1024, Length: 2048},
			},
		}

		data, err := original.MarshalBinary()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		decoded := &Manifest{}
		if err := decoded.UnmarshalBinary(data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if decoded.Header.PackageCount != original.Header.PackageCount {
			t.Errorf("PackageCount: got %d, want %d", decoded.Header.PackageCount, original.Header.PackageCount)
		}

		if len(decoded.FrameContents) != len(original.FrameContents) {
			t.Errorf("FrameContents len: got %d, want %d", len(decoded.FrameContents), len(original.FrameContents))
		}

		if len(decoded.Frames) != len(original.Frames) {
			t.Errorf("Frames len: got %d, want %d", len(decoded.Frames), len(original.Frames))
		}
	})

	t.Run("PackageCount", func(t *testing.T) {
		m := &Manifest{Header: Header{PackageCount: 5}}
		if m.PackageCount() != 5 {
			t.Errorf("PackageCount: got %d, want 5", m.PackageCount())
		}
	})

	t.Run("FileCount", func(t *testing.T) {
		m := &Manifest{
			FrameContents: make([]FrameContent, 100),
		}
		if m.FileCount() != 100 {
			t.Errorf("FileCount: got %d, want 100", m.FileCount())
		}
	})
}
