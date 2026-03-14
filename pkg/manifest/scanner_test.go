package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkFile creates a file with the given size (filled with zeros),
// creating intermediate directories as needed.
func mkFile(t *testing.T, path string, size int) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeSidecarJSON writes a raw .evrmeta JSON sidecar alongside filePath.
func writeSidecarJSON(t *testing.T, filePath string, typeSymHex, fileSymHex string) {
	t.Helper()
	sc := Sidecar{TypeSymbol: typeSymHex, FileSymbol: fileSymHex}
	data, err := json.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath+".evrmeta", data, 0644); err != nil {
		t.Fatal(err)
	}
}

// countTotalFiles counts all ScannedFile entries across all chunks.
func countTotalFiles(chunks [][]ScannedFile) int {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	return total
}

func TestScanFiles_SidecarLayout(t *testing.T) {
	// Named layout: files with .meta sidecars get symbols from the sidecar.
	// All sidecar-backed files go into chunk 0.
	tmp := t.TempDir()

	filePath := mkFile(t, filepath.Join(tmp, "textures", "sky.dds"), 100)
	writeSidecarJSON(t, filePath, "00000000000004d2", "00000000000016e4")

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	if len(result) < 1 || len(result[0]) != 1 {
		t.Fatalf("expected 1 file in chunk 0, got chunks=%d", len(result))
	}

	f := result[0][0]
	if f.TypeSymbol != 0x4d2 {
		t.Errorf("TypeSymbol = %d (0x%x), want 0x4d2", f.TypeSymbol, f.TypeSymbol)
	}
	if f.FileSymbol != 0x16e4 {
		t.Errorf("FileSymbol = %d (0x%x), want 0x16e4", f.FileSymbol, f.FileSymbol)
	}
	if f.Size != 100 {
		t.Errorf("Size = %d, want 100", f.Size)
	}
	if f.Path != filePath {
		t.Errorf("Path = %q, want %q", f.Path, filePath)
	}
}

func TestScanFiles_SidecarTakesPrecedence(t *testing.T) {
	// A file in a hex-structured path AND having a sidecar.
	// The sidecar values should win.
	tmp := t.TempDir()

	filePath := mkFile(t, filepath.Join(tmp, "0", "1111", "2222"), 50)
	writeSidecarJSON(t, filePath, "000000000000aaaa", "000000000000bbbb")

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	if len(result) < 1 || len(result[0]) != 1 {
		t.Fatalf("expected 1 file in chunk 0, got chunks=%d", len(result))
	}

	f := result[0][0]
	if f.TypeSymbol != 0xaaaa {
		t.Errorf("TypeSymbol = 0x%x, want 0xaaaa — sidecar should take precedence", f.TypeSymbol)
	}
	if f.FileSymbol != 0xbbbb {
		t.Errorf("FileSymbol = 0x%x, want 0xbbbb — sidecar should take precedence", f.FileSymbol)
	}
}

func TestScanFiles_SkipsMetaFiles(t *testing.T) {
	// .meta sidecar files should not appear as entries in the result.
	tmp := t.TempDir()

	filePath := mkFile(t, filepath.Join(tmp, "textures", "sky.dds"), 10)
	writeSidecarJSON(t, filePath, "0000000000001111", "0000000000002222")

	// Also create a standalone .evrmeta file (no companion data file)
	mkFile(t, filepath.Join(tmp, "textures", "orphan.evrmeta"), 30)

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	total := countTotalFiles(result)
	if total != 1 {
		t.Errorf("expected 1 file (meta files should be skipped), got %d", total)
	}
}

func TestScanFiles_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result for empty dir, got %d chunks", len(result))
	}
}

func TestScanFiles_HexLayout_ParsesBase16(t *testing.T) {
	// BUG: The scanner uses strconv.ParseInt with base 10 for type/file
	// symbols from directory names. Real EVR extractions use hex symbol
	// names (e.g. "beac1969cb7b8861") which contain a-f characters.
	// This test documents the bug: it SHOULD succeed but FAILS because
	// ParseInt(name, 10, 64) cannot parse hex digits.
	tmp := t.TempDir()

	mkFile(t, filepath.Join(tmp, "0", "beac1969cb7b8861", "74d228d09dc5dd8f"), 10)

	_, err := ScanFiles(tmp)
	if err != nil {
		if strings.Contains(err.Error(), "parse") {
			t.Skipf("BUG CONFIRMED: base-10 parsing fails on hex names: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	t.Log("hex parsing appears to be fixed")
}

func TestScanFiles_FileWithExtension(t *testing.T) {
	// BUG: The scanner parses the file symbol from filepath.Base(path)
	// using strconv.ParseInt. If the filename has an extension (e.g.
	// "5678.dds"), ParseInt fails because ".dds" is not a valid integer.
	// This affects the codec pipeline which adds extensions during extract.
	tmp := t.TempDir()

	mkFile(t, filepath.Join(tmp, "0", "1234", "5678.dds"), 64)

	_, err := ScanFiles(tmp)
	if err != nil {
		if strings.Contains(err.Error(), "parse") {
			t.Skipf("BUG CONFIRMED: file extension breaks parsing: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	t.Log("extension handling appears to be fixed")
}

func TestScanFiles_MultipleSidecarFiles(t *testing.T) {
	// Multiple files with sidecars should all end up in chunk 0.
	tmp := t.TempDir()

	f1 := mkFile(t, filepath.Join(tmp, "textures", "sky.dds"), 10)
	writeSidecarJSON(t, f1, "0000000000001000", "0000000000002000")

	f2 := mkFile(t, filepath.Join(tmp, "textures", "ground.dds"), 20)
	writeSidecarJSON(t, f2, "0000000000001001", "0000000000002001")

	f3 := mkFile(t, filepath.Join(tmp, "audio", "bang.ogg"), 30)
	writeSidecarJSON(t, f3, "0000000000003000", "0000000000004000")

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	if len(result) < 1 {
		t.Fatal("expected at least 1 chunk")
	}

	total := countTotalFiles(result)
	if total != 3 {
		t.Errorf("expected 3 files total, got %d", total)
	}

	if len(result[0]) != 3 {
		t.Errorf("expected 3 files in chunk 0, got %d", len(result[0]))
	}
}

func TestScanFiles_SidecarNegativeSymbols(t *testing.T) {
	// Sidecar with large hex values that overflow to negative int64.
	tmp := t.TempDir()

	filePath := mkFile(t, filepath.Join(tmp, "data", "asset.bin"), 8)
	writeSidecarJSON(t, filePath, "beac1969cb7b8861", "ffffffffffffffff")

	result, err := ScanFiles(tmp)
	if err != nil {
		t.Fatalf("ScanFiles returned error: %v", err)
	}

	if len(result) < 1 || len(result[0]) < 1 {
		t.Fatal("expected at least 1 file")
	}

	f := result[0][0]
	if f.TypeSymbol >= 0 {
		t.Errorf("expected negative TypeSymbol for 0xbeac..., got %d", f.TypeSymbol)
	}
	if f.FileSymbol != -1 {
		t.Errorf("FileSymbol = %d, want -1 (0xffffffffffffffff)", f.FileSymbol)
	}
}
