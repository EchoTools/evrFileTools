package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSidecarPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple file", "foo/bar.dds", "foo/bar.dds.evrmeta"},
		{"nested path", "a/b/c/file.txt", "a/b/c/file.txt.evrmeta"},
		{"no extension", "somefile", "somefile.evrmeta"},
		{"trailing slash", "dir/", "dir/.evrmeta"},
		{"empty string", "", ".evrmeta"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SidecarPath(tt.input)
			if got != tt.expected {
				t.Errorf("SidecarPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWriteReadSidecar_RoundTrip(t *testing.T) {
	// TypeDDSTexture = -4706379568332879927 is a known value from the codebase.
	// As uint64: 13740364505376671689 = 0xbeac1969cb7b8849
	tests := []struct {
		name       string
		typeSymbol int64
		fileSymbol int64
	}{
		{"positive values", 0x1234, 0x5678},
		{"zero values", 0, 0},
		{"max int64", 0x7fffffffffffffff, 0x7fffffffffffffff},
		{"negative type (DDS-like)", -4706379568332879927, 0x74d228d09dc5dd8f},
		{"both negative", -1, -2},
		{"min int64", -9223372036854775808, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fp := filepath.Join(dir, "test.dds")

			if err := WriteSidecar(fp, tt.typeSymbol, tt.fileSymbol); err != nil {
				t.Fatalf("WriteSidecar() error = %v", err)
			}

			gotType, gotFile, err := ReadSidecar(fp)
			if err != nil {
				t.Fatalf("ReadSidecar() error = %v", err)
			}
			if gotType != tt.typeSymbol {
				t.Errorf("typeSymbol = %d (0x%x), want %d (0x%x)",
					gotType, uint64(gotType), tt.typeSymbol, uint64(tt.typeSymbol))
			}
			if gotFile != tt.fileSymbol {
				t.Errorf("fileSymbol = %d (0x%x), want %d (0x%x)",
					gotFile, uint64(gotFile), tt.fileSymbol, uint64(tt.fileSymbol))
			}
		})
	}
}

func TestReadSidecar_NoFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "nonexistent.dds")

	typeS, fileS, err := ReadSidecar(fp)
	if err != nil {
		t.Fatalf("ReadSidecar() returned unexpected error: %v", err)
	}
	if typeS != 0 || fileS != 0 {
		t.Errorf("expected (0, 0), got (%d, %d)", typeS, fileS)
	}
}

func TestReadSidecar_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "bad.dds")
	metaPath := SidecarPath(fp)

	if err := os.WriteFile(metaPath, []byte("{not json at all!!!"), 0644); err != nil {
		t.Fatalf("failed to write garbage meta: %v", err)
	}

	_, _, err := ReadSidecar(fp)
	if err == nil {
		t.Fatal("ReadSidecar() expected error for malformed JSON, got nil")
	}
}

func TestReadSidecar_InvalidHex(t *testing.T) {
	tests := []struct {
		name       string
		typeSymbol string
		fileSymbol string
	}{
		{"non-hex typeSymbol", "not_a_hex_value!", "0000000000001234"},
		{"non-hex fileSymbol", "0000000000001234", "zzzzzzzzzzzzzzzz"},
		{"empty typeSymbol", "", "0000000000001234"},
		{"too-large hex", "1ffffffffffffffff", "0000000000001234"}, // 17 hex digits overflows uint64
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			fp := filepath.Join(dir, "file.dds")
			metaPath := SidecarPath(fp)

			sc := Sidecar{TypeSymbol: tt.typeSymbol, FileSymbol: tt.fileSymbol}
			data, err := json.Marshal(sc)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if err := os.WriteFile(metaPath, data, 0644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			_, _, err = ReadSidecar(fp)
			if err == nil {
				t.Fatal("ReadSidecar() expected error for invalid hex, got nil")
			}
		})
	}
}

func TestWriteSidecar_HexFormatting(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "fmt.dds")

	// Use a small positive value to verify zero-padding.
	if err := WriteSidecar(fp, 0xff, 0x1); err != nil {
		t.Fatalf("WriteSidecar() error = %v", err)
	}

	raw, err := os.ReadFile(SidecarPath(fp))
	if err != nil {
		t.Fatalf("reading meta file: %v", err)
	}

	var sc Sidecar
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Verify 16-char zero-padded lowercase hex.
	if len(sc.TypeSymbol) != 16 {
		t.Errorf("typeSymbol length = %d, want 16 (value: %q)", len(sc.TypeSymbol), sc.TypeSymbol)
	}
	if sc.TypeSymbol != "00000000000000ff" {
		t.Errorf("typeSymbol = %q, want %q", sc.TypeSymbol, "00000000000000ff")
	}
	if len(sc.FileSymbol) != 16 {
		t.Errorf("fileSymbol length = %d, want 16 (value: %q)", len(sc.FileSymbol), sc.FileSymbol)
	}
	if sc.FileSymbol != "0000000000000001" {
		t.Errorf("fileSymbol = %q, want %q", sc.FileSymbol, "0000000000000001")
	}
}

func TestWriteSidecar_NegativeSymbol(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "neg.dds")

	// -1 as int64 is 0xffffffffffffffff as uint64.
	original := int64(-1)
	if err := WriteSidecar(fp, original, 0); err != nil {
		t.Fatalf("WriteSidecar() error = %v", err)
	}

	// Verify the raw JSON contains the unsigned hex representation.
	raw, err := os.ReadFile(SidecarPath(fp))
	if err != nil {
		t.Fatalf("reading meta file: %v", err)
	}
	var sc Sidecar
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if sc.TypeSymbol != "ffffffffffffffff" {
		t.Errorf("raw hex = %q, want %q", sc.TypeSymbol, "ffffffffffffffff")
	}

	// Verify ReadSidecar recovers the original negative int64.
	gotType, _, err := ReadSidecar(fp)
	if err != nil {
		t.Fatalf("ReadSidecar() error = %v", err)
	}
	if gotType != original {
		t.Errorf("recovered typeSymbol = %d, want %d", gotType, original)
	}
}

func TestSidecarPath_MetaExtensionCollision(t *testing.T) {
	// With .evrmeta extension, .meta files no longer cause a collision.
	input := "assets/config.meta"
	got := SidecarPath(input)
	expected := "assets/config.meta.evrmeta"
	if got != expected {
		t.Errorf("SidecarPath(%q) = %q, want %q", input, got, expected)
	}

	// Verify the full round-trip works.
	dir := t.TempDir()
	fp := filepath.Join(dir, "config.meta")
	// Create the data file so it exists on disk.
	if err := os.WriteFile(fp, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	typeVal := int64(42)
	fileVal := int64(99)
	if err := WriteSidecar(fp, typeVal, fileVal); err != nil {
		t.Fatalf("WriteSidecar() error = %v", err)
	}

	gotType, gotFile, err := ReadSidecar(fp)
	if err != nil {
		t.Fatalf("ReadSidecar() error = %v", err)
	}
	if gotType != typeVal || gotFile != fileVal {
		t.Errorf("round-trip: got (%d, %d), want (%d, %d)",
			gotType, gotFile, typeVal, fileVal)
	}

	// Confirm the sidecar file is named .evrmeta on disk.
	metaPath := SidecarPath(fp)
	if filepath.Ext(metaPath) != ".evrmeta" {
		t.Errorf("sidecar extension = %q, want .evrmeta", filepath.Ext(metaPath))
	}
	if filepath.Ext(fp) != ".meta" {
		t.Errorf("original file extension = %q, want .meta", filepath.Ext(fp))
	}
	if _, err := os.Stat(metaPath); err != nil {
		t.Errorf("sidecar file %q does not exist: %v", metaPath, err)
	}
}
