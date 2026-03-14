package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHexSymbol(t *testing.T) {
	wantVal := int64(int64(-4707359568332879775))
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{"with 0x prefix", "0xbeac1969cb7b8861", wantVal, false},
		{"without prefix", "beac1969cb7b8861", wantVal, false},
		{"empty string", "", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHexSymbol(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("parseHexSymbol(%q) expected error, got %d", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHexSymbol(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("parseHexSymbol(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "source.bin")
	dstPath := filepath.Join(dir, "dest.bin")

	content := []byte("test file contents for copy")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("copyFile() dest contents = %q, want %q", got, content)
	}
}

func TestCopyFile_SourceMissing(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "nonexistent.bin")
	dstPath := filepath.Join(dir, "dest.bin")

	err := copyFile(srcPath, dstPath)
	if err == nil {
		t.Error("copyFile() with missing source expected error, got nil")
	}
}
