package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EchoTools/evrFileTools/pkg/naming"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := formatBytes(tc.input)
			if got != tc.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCountFiles(t *testing.T) {
	dir := t.TempDir()

	// Create regular files with known sizes.
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory (should be excluded from count).
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	count, totalBytes, err := countFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("countFiles count = %d, want 3", count)
	}
	// Each file contains "hello" = 5 bytes, total = 15.
	if totalBytes != 15 {
		t.Errorf("countFiles totalBytes = %d, want 15", totalBytes)
	}
}

func TestCountKnownTypes(t *testing.T) {
	stats := []typeStats{
		{typeSymbol: naming.TypeDDSTexture, count: 10},   // known
		{typeSymbol: naming.TypeAudioReference, count: 5}, // known
		{typeSymbol: naming.TypeSymbol(0x1234), count: 3}, // unknown
		{typeSymbol: naming.TypeSymbol(0x5678), count: 1}, // unknown
	}

	got := countKnownTypes(stats)
	if got != 2 {
		t.Errorf("countKnownTypes() = %d, want 2", got)
	}
}
