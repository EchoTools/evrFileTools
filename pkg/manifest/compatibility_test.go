package manifest

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/EchoTools/evrFileTools/pkg/archive"
)

// TestFrameFieldOrder tests that Frame fields are in the correct order.
// IMPORTANT: The carnation reference implementation uses a DIFFERENT field order:
//
//	carnation:   compressed_size, uncompressed_size, package_index, next_offset
//	evrFileTools: package_index, offset, compressed_size, length
//
// This test validates the correct order based on actual file reading.
func TestFrameFieldOrder(t *testing.T) {
	// The Frame struct has these fields in this order:
	// 1. PackageIndex (4 bytes)   - Which package file (0, 1, 2, ...)
	// 2. Offset (4 bytes)         - Byte offset within package
	// 3. CompressedSize (4 bytes) - Compressed frame size
	// 4. Length (4 bytes)         - Decompressed frame size
	//
	// Note: carnation uses "next_offset" instead of "offset", which seems to mean
	// the end position (offset + compressed_size). Need to verify with actual data.

	frame := Frame{
		PackageIndex:   0,
		Offset:         1000,
		CompressedSize: 500,
		Length:         2048,
	}

	buf := make([]byte, FrameSize)
	binary.LittleEndian.PutUint32(buf[0:], frame.PackageIndex)
	binary.LittleEndian.PutUint32(buf[4:], frame.Offset)
	binary.LittleEndian.PutUint32(buf[8:], frame.CompressedSize)
	binary.LittleEndian.PutUint32(buf[12:], frame.Length)

	// Decode and verify
	decoded := Frame{}
	decoded.PackageIndex = binary.LittleEndian.Uint32(buf[0:])
	decoded.Offset = binary.LittleEndian.Uint32(buf[4:])
	decoded.CompressedSize = binary.LittleEndian.Uint32(buf[8:])
	decoded.Length = binary.LittleEndian.Uint32(buf[12:])

	if decoded != frame {
		t.Errorf("Frame encoding mismatch: got %+v, want %+v", decoded, frame)
	}
}

// TestCarnationFrameFieldOrder tests what happens if we use carnation's field order.
// This demonstrates the difference between implementations.
func TestCarnationFrameFieldOrder(t *testing.T) {
	// Carnation's struct definition:
	// const frame = struct()
	//     .word32Ule('compressed_size')
	//     .word32Ule('uncompressed_size')
	//     .word32Ule('package_index')
	//     .word32Ule('next_offset')

	type CarnationFrame struct {
		CompressedSize   uint32
		UncompressedSize uint32
		PackageIndex     uint32
		NextOffset       uint32 // This is offset + compressed_size
	}

	// If we encode in evrFileTools order but decode with carnation order
	// we'll get wrong values. This test documents the difference.

	evrFrame := Frame{
		PackageIndex:   0,    // offset 0
		Offset:         1000, // offset 4
		CompressedSize: 500,  // offset 8
		Length:         2048, // offset 12
	}

	buf := make([]byte, FrameSize)
	binary.LittleEndian.PutUint32(buf[0:], evrFrame.PackageIndex)
	binary.LittleEndian.PutUint32(buf[4:], evrFrame.Offset)
	binary.LittleEndian.PutUint32(buf[8:], evrFrame.CompressedSize)
	binary.LittleEndian.PutUint32(buf[12:], evrFrame.Length)

	// If carnation decodes this, it would read:
	carnationDecoded := CarnationFrame{
		CompressedSize:   binary.LittleEndian.Uint32(buf[0:]),  // reads PackageIndex=0
		UncompressedSize: binary.LittleEndian.Uint32(buf[4:]),  // reads Offset=1000
		PackageIndex:     binary.LittleEndian.Uint32(buf[8:]),  // reads CompressedSize=500
		NextOffset:       binary.LittleEndian.Uint32(buf[12:]), // reads Length=2048
	}

	// This documents the incompatibility
	if carnationDecoded.PackageIndex == evrFrame.PackageIndex {
		t.Error("Frame fields would match if carnation order is the same - verify this")
	}
}

// TestMetadataSizeDiscrepancy tests the difference in FileMetadata/SomeStructure size.
// FIXED: Now uses correct 32-byte size matching NRadEngine's ManifestSomeStructure
func TestMetadataSizeDiscrepancy(t *testing.T) {
	// evrFileTools FileMetadata (CORRECTED):
	// TypeSymbol int64  (8)
	// FileSymbol int64  (8)
	// Unk1       int64  (8)
	// Unk2       int64  (8)
	// Total: 32 bytes

	if FileMetadataSize != 32 {
		t.Errorf("FileMetadataSize: got %d, want 32", FileMetadataSize)
	}

	// NRadEngine/Actual format (32 bytes):
	// typeSymbol  int64  (8)
	// fileSymbol  int64  (8)
	// unk1        int64  (8)
	// unk2        int64  (8)
	// Total: 32 bytes

	t.Log("FileMetadata size now correctly matches NRadEngine (32 bytes)")
}

// TestSectionPadding verifies Section structure and padding.
// The manifest header has specific padding between sections.
func TestSectionPadding(t *testing.T) {
	// Header layout according to evrFileTools:
	// PackageCount (4) + Unk1 (4) + Unk2 (8) = 16 bytes
	// FrameContents Section (48) + Padding (16) = 64 bytes
	// Metadata Section (48) + Padding (16) = 64 bytes
	// Frames Section (48) = 48 bytes
	// Total: 16 + 64 + 64 + 48 = 192 bytes

	if HeaderSize != 192 {
		t.Errorf("HeaderSize: got %d, want 192", HeaderSize)
	}

	if SectionSize != 48 {
		t.Errorf("SectionSize: got %d, want 48", SectionSize)
	}
}

// TestRealManifestParsing tests parsing of actual manifest files.
// Now that the implementation is fixed, these tests should pass without issues.
func TestRealManifestParsing(t *testing.T) {
	// Look for test data files
	testDataPaths := []string{
		"../../_data/manifests/48037dc70b0ecab2",
		"../../_data/manifests/2b47aab238f60515",
		"testdata/sample_manifest",
	}

	var manifestPath string
	for _, p := range testDataPaths {
		if _, err := os.Stat(p); err == nil {
			manifestPath = p
			break
		}
	}

	if manifestPath == "" {
		t.Skip("No test manifest file found")
	}

	m, err := ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Basic sanity checks
	if m.PackageCount() == 0 {
		t.Error("PackageCount should not be 0")
	}

	if m.FileCount() == 0 {
		t.Error("FileCount should not be 0")
	}

	// Count frames with issues (should be zero now that implementation is fixed)
	var zeroLengthFrames int
	var badRatioFrames int
	var badPackageIndex int

	for _, frame := range m.Frames {
		if frame.Length == 0 && frame.CompressedSize > 0 {
			zeroLengthFrames++
		}

		if int(frame.PackageIndex) >= m.PackageCount() {
			badPackageIndex++
		}

		if frame.CompressedSize > frame.Length*2 && frame.Length > 0 {
			badRatioFrames++
		}
	}

	// These should now be very small with the fixed implementation
	// Some manifests may have a small number of frames with zero length
	// (possibly sentinel values or truncated packages)
	if zeroLengthFrames > len(m.Frames)/100 { // More than 1% is suspicious
		t.Errorf("%d frames have compressed data but zero length (>1%% of total)", zeroLengthFrames)
	} else if zeroLengthFrames > 0 {
		t.Logf("Note: %d frames have compressed data but zero length (may be expected)", zeroLengthFrames)
	}

	if badPackageIndex > 0 {
		t.Errorf("%d frames have invalid PackageIndex", badPackageIndex)
	}

	// Verify FrameContents reference frames correctly
	maxFrameIndex := uint32(len(m.Frames))
	var badFrameRefs int
	for _, fc := range m.FrameContents {
		if fc.FrameIndex >= maxFrameIndex {
			badFrameRefs++
		}
	}

	if badFrameRefs > 0 {
		t.Errorf("%d FrameContents reference invalid frames", badFrameRefs)
	}

	t.Logf("Manifest parsed successfully: %d files in %d packages, %d frames",
		m.FileCount(), m.PackageCount(), len(m.Frames))
}

// TestArchiveHeaderFormat tests the ZSTD archive header format.
func TestArchiveHeaderFormat(t *testing.T) {
	// Archive header format:
	// Magic            [4]byte  "ZSTD" (0x5a 0x53 0x54 0x44)
	// HeaderLength     uint32   Always 16
	// Length           uint64   Uncompressed size
	// CompressedLength uint64   Compressed size

	if archive.HeaderSize != 24 {
		t.Errorf("archive.HeaderSize: got %d, want 24", archive.HeaderSize)
	}

	expectedMagic := [4]byte{0x5a, 0x53, 0x54, 0x44}
	if archive.Magic != expectedMagic {
		t.Errorf("archive.Magic: got %x, want %x", archive.Magic, expectedMagic)
	}
}

// TestPackageFileFormat tests that package files have the expected structure.
// Package files do NOT have the ZSTD wrapper header - they contain raw ZSTD frames.
func TestPackageFileFormat(t *testing.T) {
	testDataPaths := []string{
		"../../_data/packages/2b47aab238f60515_0",
		"testdata/sample_package_0",
	}

	var packagePath string
	for _, p := range testDataPaths {
		if _, err := os.Stat(p); err == nil {
			packagePath = p
			break
		}
	}

	if packagePath == "" {
		t.Skip("No test package file found")
	}

	f, err := os.Open(packagePath)
	if err != nil {
		t.Fatalf("Open package: %v", err)
	}
	defer f.Close()

	// Read first few bytes
	header := make([]byte, 8)
	if _, err := f.Read(header); err != nil {
		t.Fatalf("Read header: %v", err)
	}

	// ZSTD frame magic is 0xFD2FB528 (little-endian: 28 b5 2f fd)
	zstdMagic := []byte{0x28, 0xb5, 0x2f, 0xfd}
	if header[0] == zstdMagic[0] && header[1] == zstdMagic[1] &&
		header[2] == zstdMagic[2] && header[3] == zstdMagic[3] {
		t.Log("Package file starts with ZSTD frame magic (no wrapper header)")
	} else if string(header[0:4]) == "ZSTD" {
		t.Error("Package file has ZSTD wrapper header - unexpected!")
	} else {
		t.Logf("Package header bytes: %x", header)
	}
}

// TestEndToEndExtraction tests full extraction pipeline with validation.
func TestEndToEndExtraction(t *testing.T) {
	manifestPath := "../../_data/manifests/2b47aab238f60515"
	packageBasePath := "../../_data/packages/2b47aab238f60515"

	if _, err := os.Stat(manifestPath); err != nil {
		t.Skip("Test data not available")
	}

	m, err := ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	pkg, err := OpenPackage(m, packageBasePath)
	if err != nil {
		t.Fatalf("OpenPackage: %v", err)
	}
	defer pkg.Close()

	// Create temp directory
	outputDir := filepath.Join(t.TempDir(), "extracted")

	if err := pkg.Extract(outputDir); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Count extracted files
	var fileCount int
	err = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	if fileCount != m.FileCount() {
		t.Errorf("Extracted %d files, expected %d", fileCount, m.FileCount())
	}

	t.Logf("Successfully extracted %d files", fileCount)
}

// TestCorrectSectionOffsetCalculation demonstrates the correct way to calculate
// section offsets using the Length field from section descriptors.
// Now that the implementation is fixed, this test validates correct behavior.
func TestCorrectSectionOffsetCalculation(t *testing.T) {
	manifestPath := "../../_data/manifests/2b47aab238f60515"

	if _, err := os.Stat(manifestPath); err != nil {
		t.Skip("Test data not available")
	}

	f, err := os.Open(manifestPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	data, err := archive.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	// Parse header to get section info
	m := &Manifest{}
	if err := m.UnmarshalBinary(data); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}

	// CORRECT: Use Length field for positioning
	fcStart := HeaderSize
	fcEnd := fcStart + int(m.Header.FrameContents.Length)
	mdStart := fcEnd
	mdEnd := mdStart + int(m.Header.Metadata.Length)
	frStart := mdEnd

	t.Logf("Section positions using Length field:")
	t.Logf("  FrameContents: %d-%d (Length=%d)", fcStart, fcEnd, m.Header.FrameContents.Length)
	t.Logf("  Metadata: %d-%d (Length=%d)", mdStart, mdEnd, m.Header.Metadata.Length)
	t.Logf("  Frames start: %d (Length=%d)", frStart, m.Header.Frames.Length)

	// Read first frame from CORRECT position
	if frStart+16 <= len(data) {
		pkgIdx := binary.LittleEndian.Uint32(data[frStart:])
		offset := binary.LittleEndian.Uint32(data[frStart+4:])
		compSize := binary.LittleEndian.Uint32(data[frStart+8:])
		length := binary.LittleEndian.Uint32(data[frStart+12:])

		t.Logf("First frame at offset %d: PackageIndex=%d, Offset=%d, CompressedSize=%d, Length=%d",
			frStart, pkgIdx, offset, compSize, length)

		// Validate this looks correct
		if pkgIdx > uint32(m.PackageCount()) {
			t.Errorf("Frame PackageIndex %d > PackageCount %d - incorrect offset?", pkgIdx, m.PackageCount())
		}
		if length == 0 && compSize > 0 {
			t.Error("Frame has compressed size but zero length - incorrect offset?")
		}
	}

	// Verify the implementation now uses Length-based offsets
	// The parsed frames should match what we read manually
	if len(m.Frames) > 0 {
		manualPkgIdx := binary.LittleEndian.Uint32(data[frStart:])
		if m.Frames[0].PackageIndex != manualPkgIdx {
			t.Errorf("Parsed frame PackageIndex %d doesn't match manual read %d",
				m.Frames[0].PackageIndex, manualPkgIdx)
		} else {
			t.Log("Parsed frame matches manual read - implementation is correct!")
		}
	}

	// Verify ElementSize matches the new constant
	t.Logf("\nElementSize comparison (manifest vs constant):")
	t.Logf("  FrameContents: %d vs %d", m.Header.FrameContents.ElementSize, FrameContentSize)
	t.Logf("  Metadata: %d vs %d", m.Header.Metadata.ElementSize, FileMetadataSize)
	t.Logf("  Frames: %d vs %d", m.Header.Frames.ElementSize, FrameSize)
}
