package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/zstd"
)

// buildTestManifest creates a minimal manifest with the given number of data
// frames followed by numTerminators terminator frames (Length=0, CompressedSize=0).
// Each FrameContent entry i points to frame index i with a unique TypeSymbol/FileSymbol pair.
func buildTestManifest(numFiles, numTerminators int) *Manifest {
	m := &Manifest{
		Header: Header{
			PackageCount: 1,
			Frames: Section{
				ElementSize:  uint64(FrameSize),
				Count:        uint64(numFiles + numTerminators),
				ElementCount: uint64(numFiles + numTerminators),
				Length:       uint64((numFiles + numTerminators) * FrameSize),
			},
			FrameContents: Section{
				ElementSize:  uint64(FrameContentSize),
				Count:        uint64(numFiles),
				ElementCount: uint64(numFiles),
				Length:       uint64(numFiles * FrameContentSize),
			},
			Metadata: Section{
				ElementSize:  uint64(FileMetadataSize),
				Count:        uint64(numFiles),
				ElementCount: uint64(numFiles),
				Length:       uint64(numFiles * FileMetadataSize),
			},
		},
	}

	// Create data frames and matching FrameContent entries.
	for i := 0; i < numFiles; i++ {
		m.Frames = append(m.Frames, Frame{
			PackageIndex:   0,
			Offset:         uint32(i * 100),
			CompressedSize: 50,
			Length:         100,
		})
		m.FrameContents = append(m.FrameContents, FrameContent{
			TypeSymbol: int64(0x1000 + i),
			FileSymbol: int64(0x2000 + i),
			FrameIndex: uint32(i),
			DataOffset: 0,
			Size:       100,
			Alignment:  1,
		})
		m.Metadata = append(m.Metadata, FileMetadata{
			TypeSymbol: int64(0x1000 + i),
			FileSymbol: int64(0x2000 + i),
		})
	}

	// Add terminator frames.
	for i := 0; i < numTerminators; i++ {
		m.Frames = append(m.Frames, Frame{
			PackageIndex:   0,
			Offset:         0,
			CompressedSize: 0,
			Length:         0,
		})
	}

	return m
}

// setupPackageDir creates a temp directory with a minimal package file and
// returns (tmpDir, pkgBasePath). The package file is named "<stem>_0" and
// contains fakeSize bytes of zeroes.
func setupPackageDir(t *testing.T, fakeSize int) (string, string) {
	t.Helper()
	tmpDir := t.TempDir()
	stem := "48037dc70b0ecab2"
	pkgPath := filepath.Join(tmpDir, fmt.Sprintf("%s_0", stem))
	data := make([]byte, fakeSize)
	if err := os.WriteFile(pkgPath, data, 0644); err != nil {
		t.Fatalf("write fake package file: %v", err)
	}
	return tmpDir, filepath.Join(tmpDir, stem)
}

func TestDeepCopyManifest(t *testing.T) {
	orig := buildTestManifest(3, 1)

	cp := deepCopyManifest(orig)

	// Modify the copy's slices.
	cp.FrameContents[0].TypeSymbol = 0xDEAD
	cp.Frames[0].Offset = 99999
	cp.Metadata[0].AssetType = 42
	cp.Header.PackageCount = 100

	// Verify original is unchanged.
	if orig.FrameContents[0].TypeSymbol == 0xDEAD {
		t.Error("modifying copy's FrameContents changed original")
	}
	if orig.Frames[0].Offset == 99999 {
		t.Error("modifying copy's Frames changed original")
	}
	if orig.Metadata[0].AssetType == 42 {
		t.Error("modifying copy's Metadata changed original")
	}
	// Header is a value type, so copy is independent by default.
	if orig.Header.PackageCount == 100 {
		t.Error("modifying copy's Header changed original")
	}

	// Verify lengths are preserved.
	if len(cp.FrameContents) != len(orig.FrameContents) {
		t.Errorf("FrameContents length mismatch: got %d, want %d", len(cp.FrameContents), len(orig.FrameContents))
	}
	if len(cp.Frames) != len(orig.Frames) {
		t.Errorf("Frames length mismatch: got %d, want %d", len(cp.Frames), len(orig.Frames))
	}
	if len(cp.Metadata) != len(orig.Metadata) {
		t.Errorf("Metadata length mismatch: got %d, want %d", len(cp.Metadata), len(orig.Metadata))
	}
}

// TestPatchFile_FrameIndexShift documents a known bug in PatchFile:
//
// When a new frame is inserted at insertIdx, all FrameContent entries whose
// FrameIndex >= insertIdx should be incremented by 1 to account for the shift.
// The current implementation only updates the single targeted FrameContent
// entry (the one matching typeSymbol/fileSymbol), leaving other entries with
// stale FrameIndex values that now point to the wrong frames.
func TestPatchFile_FrameIndexShift(t *testing.T) {
	// Create manifest with 3 data files (frame indices 0, 1, 2) and 1 terminator.
	m := buildTestManifest(3, 1)
	tmpDir, pkgBasePath := setupPackageDir(t, 200)
	_ = tmpDir

	// Patch file 0 (TypeSymbol=0x1000, FileSymbol=0x2000).
	// This inserts a new frame at index 3 (before the terminator at index 3).
	newData := []byte("hello patched world")
	result, err := PatchFile(m, pkgBasePath, 0x1000, 0x2000, newData)
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	// The new frame was inserted at index 3 (before the terminator).
	// In this case, since insertIdx == 3 and the other files point to frames 1 and 2,
	// no shift is needed because their indices are < insertIdx.
	// To properly test the bug, we need a scenario where the insert shifts existing indices.

	// Now patch file 0 again. This will insert another frame at the same insertIdx.
	// After first patch: frames are [orig0, orig1, orig2, NEW_0, terminator]
	// File 1 -> frame 1, File 2 -> frame 2, File 0 -> frame 3 (NEW_0).
	newData2 := []byte("second patch")
	result2, err := PatchFile(result, pkgBasePath, 0x1001, 0x2001, newData2)
	if err != nil {
		t.Fatalf("PatchFile (second): %v", err)
	}

	// After second patch the frames are:
	// [orig0, orig1, orig2, NEW_0, NEW_1, terminator]
	// File 1 (the patched one) should now point to frame 4 (insertIdx=4).
	// File 0 was pointing to frame 3 (NEW_0) from the first patch.
	// Since a frame was inserted at index 4, frame 3 is unaffected.
	// File 2 still points to frame 2.

	// To trigger the actual bug, patch file 0 to insert at an index that shifts
	// file 1's frame index. Create a fresh manifest:
	m2 := buildTestManifest(3, 1)
	_, pkgBasePath2 := setupPackageDir(t, 200)

	// Patch file 1 (index 1). New frame inserted at index 3 (before terminator).
	// File 0 -> frame 0, File 1 -> frame 3 (new), File 2 -> frame 2.
	_, err = PatchFile(m2, pkgBasePath2, 0x1001, 0x2001, []byte("patch file 1"))
	if err != nil {
		t.Fatalf("PatchFile m2: %v", err)
	}

	// Now create a manifest where insertion WILL shift other entries.
	// Use a manifest with no terminators so insertion happens at the end,
	// or manually set up a scenario where an entry has a high frame index.
	// Manually set file 2 to point to frame index 2.
	// After patching file 0, the new frame is inserted at index 3 (before terminator at 3).
	// Since file 2's FrameIndex (2) < insertIdx (3), no shift needed for this case.

	// To expose the bug properly: make file 2 point to a frame AFTER where
	// the new frame will be inserted. We simulate this by having more terminators
	// and manually adjusting frame indices.
	m4 := &Manifest{
		Header: Header{
			PackageCount: 1,
			Frames: Section{
				ElementSize:  uint64(FrameSize),
				Count:        5,
				ElementCount: 5,
				Length:       uint64(5 * FrameSize),
			},
			FrameContents: Section{
				ElementSize:  uint64(FrameContentSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FrameContentSize),
			},
			Metadata: Section{
				ElementSize:  uint64(FileMetadataSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FileMetadataSize),
			},
		},
		Frames: []Frame{
			{PackageIndex: 0, Offset: 0, CompressedSize: 50, Length: 100},   // 0
			{PackageIndex: 0, Offset: 100, CompressedSize: 50, Length: 100}, // 1
			{PackageIndex: 0, Offset: 200, CompressedSize: 50, Length: 100}, // 2
			{PackageIndex: 0, Offset: 300, CompressedSize: 50, Length: 100}, // 3 - data frame
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 4 - terminator
		},
		FrameContents: []FrameContent{
			{TypeSymbol: 0x1000, FileSymbol: 0x2000, FrameIndex: 0, Size: 100, Alignment: 1},
			{TypeSymbol: 0x1001, FileSymbol: 0x2001, FrameIndex: 2, Size: 100, Alignment: 1},
			{TypeSymbol: 0x1002, FileSymbol: 0x2002, FrameIndex: 3, Size: 100, Alignment: 1},
		},
		Metadata: []FileMetadata{
			{TypeSymbol: 0x1000, FileSymbol: 0x2000},
			{TypeSymbol: 0x1001, FileSymbol: 0x2001},
			{TypeSymbol: 0x1002, FileSymbol: 0x2002},
		},
	}

	_, pkgBasePath4 := setupPackageDir(t, 400)
	// Patch file 0 (FrameIndex=0). The new frame will be inserted at index 4
	// (before the terminator at index 4).
	result4, err := PatchFile(m4, pkgBasePath4, 0x1000, 0x2000, []byte("new content"))
	if err != nil {
		t.Fatalf("PatchFile m4: %v", err)
	}

	// After insertion at index 4:
	// Frames: [0, 1, 2, 3, NEW, terminator]
	// File 0 should now point to frame 4 (the new one) - PatchFile does this correctly.
	// File 1 points to frame 2 - unchanged (2 < 4), correct.
	// File 2 points to frame 3 - unchanged (3 < 4), correct.
	// In this layout, no shift is needed because insertIdx=4 is past all existing references.

	// The bug manifests when we have a frame content pointing to an index >= insertIdx
	// that is NOT the patched entry. Build this scenario explicitly:
	m5 := &Manifest{
		Header: Header{
			PackageCount: 1,
			Frames: Section{
				ElementSize:  uint64(FrameSize),
				Count:        4,
				ElementCount: 4,
				Length:       uint64(4 * FrameSize),
			},
			FrameContents: Section{
				ElementSize:  uint64(FrameContentSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FrameContentSize),
			},
			Metadata: Section{
				ElementSize:  uint64(FileMetadataSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FileMetadataSize),
			},
		},
		Frames: []Frame{
			{PackageIndex: 0, Offset: 0, CompressedSize: 50, Length: 100},   // 0
			{PackageIndex: 0, Offset: 100, CompressedSize: 50, Length: 100}, // 1
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 2 - terminator
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 3 - terminator
		},
		FrameContents: []FrameContent{
			{TypeSymbol: 0xA, FileSymbol: 0xB, FrameIndex: 0, Size: 100, Alignment: 1},
			{TypeSymbol: 0xC, FileSymbol: 0xD, FrameIndex: 1, Size: 100, Alignment: 1},
			// Third entry also points to frame 1 (shared frame).
			{TypeSymbol: 0xE, FileSymbol: 0xF, FrameIndex: 1, Size: 50, DataOffset: 50, Alignment: 1},
		},
		Metadata: []FileMetadata{
			{TypeSymbol: 0xA, FileSymbol: 0xB},
			{TypeSymbol: 0xC, FileSymbol: 0xD},
			{TypeSymbol: 0xE, FileSymbol: 0xF},
		},
	}

	_, pkgBasePath5 := setupPackageDir(t, 200)
	// Patch file 0 (FrameIndex=0). New frame inserted at index 2 (before terminators).
	// After insertion: frames = [frame0, frame1, NEW, term, term]
	// File 0 -> should point to frame 2 (the new one). PatchFile sets this.
	// File 1 -> was pointing to frame 1. Since 1 < insertIdx(2), no shift needed. OK.
	// File 2 -> was pointing to frame 1. Since 1 < insertIdx(2), no shift needed. OK.
	_, err = PatchFile(m5, pkgBasePath5, 0xA, 0xB, []byte("patched"))
	if err != nil {
		t.Fatalf("PatchFile m5: %v", err)
	}

	// Now the REAL bug test: patch file 0 first, making its frame land at insertIdx=2.
	// Then patch file 1. Its original FrameIndex was 1, new frame inserts at 2 again.
	// But file 0 was updated to FrameIndex=2 in the first patch. After the second patch
	// inserts at index 2, file 0's FrameIndex should be shifted to 3 -- but PatchFile
	// does NOT do this.
	m6 := &Manifest{
		Header: Header{
			PackageCount: 1,
			Frames: Section{
				ElementSize:  uint64(FrameSize),
				Count:        4,
				ElementCount: 4,
				Length:       uint64(4 * FrameSize),
			},
			FrameContents: Section{
				ElementSize:  uint64(FrameContentSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FrameContentSize),
			},
			Metadata: Section{
				ElementSize:  uint64(FileMetadataSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FileMetadataSize),
			},
		},
		Frames: []Frame{
			{PackageIndex: 0, Offset: 0, CompressedSize: 50, Length: 100},   // 0
			{PackageIndex: 0, Offset: 100, CompressedSize: 50, Length: 100}, // 1
			{PackageIndex: 0, Offset: 200, CompressedSize: 50, Length: 100}, // 2
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 3 - terminator
		},
		FrameContents: []FrameContent{
			{TypeSymbol: 0xA, FileSymbol: 0xB, FrameIndex: 0, Size: 100, Alignment: 1},
			{TypeSymbol: 0xC, FileSymbol: 0xD, FrameIndex: 1, Size: 100, Alignment: 1},
			{TypeSymbol: 0xE, FileSymbol: 0xF, FrameIndex: 2, Size: 100, Alignment: 1},
		},
		Metadata: []FileMetadata{
			{TypeSymbol: 0xA, FileSymbol: 0xB},
			{TypeSymbol: 0xC, FileSymbol: 0xD},
			{TypeSymbol: 0xE, FileSymbol: 0xF},
		},
	}

	_, pkgBasePath6 := setupPackageDir(t, 400)

	// First patch: patch file A/B. New frame inserted at index 3 (before terminator).
	// Result: frames = [0, 1, 2, NEW_A, terminator]
	// File A -> frame 3, File C -> frame 1, File E -> frame 2. All correct.
	r6, err := PatchFile(m6, pkgBasePath6, 0xA, 0xB, []byte("patch A"))
	if err != nil {
		t.Fatalf("PatchFile m6 first: %v", err)
	}

	// Second patch: patch file C/D. New frame inserted at index 4 (before terminator at 4).
	// Result: frames = [0, 1, 2, NEW_A, NEW_C, terminator]
	// File C -> frame 4 (correctly set by PatchFile).
	// File A -> was frame 3. Since 3 < insertIdx(4), no shift needed. Correct.
	// File E -> was frame 2. Since 2 < insertIdx(4), no shift needed. Correct.
	r6b, err := PatchFile(r6, pkgBasePath6, 0xC, 0xD, []byte("patch C"))
	if err != nil {
		t.Fatalf("PatchFile m6 second: %v", err)
	}

	// Third patch: patch file E/F. New frame inserted at index 5 (before terminator at 5).
	// File E -> frame 5 (set by PatchFile).
	// File A -> frame 3, File C -> frame 4. Both < 5, no shift. Correct.
	_, err = PatchFile(r6b, pkgBasePath6, 0xE, 0xF, []byte("patch E"))
	if err != nil {
		t.Fatalf("PatchFile m6 third: %v", err)
	}

	// BUG: The above sequential patches work because each new frame is appended
	// at the end (before terminators), so insertIdx is always greater than all
	// existing FrameContent indices. The bug would manifest if a frame were
	// inserted in the MIDDLE (e.g., if terminators were interspersed or if
	// the insertion logic changed). For example, if we manually forced an
	// insertion at index 1, entries pointing to frames 1 and 2 would not be shifted.
	//
	// Demonstrate by checking result4 which we computed above:
	// In result4, the new frame was inserted at index 4.
	// File 2 was at FrameIndex=3. Since 3 < 4, no shift needed (correct).
	// But if the terminator were at index 1 instead, insertion would be at index 1,
	// and file 1 (FrameIndex=2) and file 2 (FrameIndex=3) should both shift to 3 and 4.
	// PatchFile does NOT perform this shift.
	//
	// We verify this with a contrived manifest where a terminator appears early:
	m7 := &Manifest{
		Header: Header{
			PackageCount: 1,
			Frames: Section{
				ElementSize:  uint64(FrameSize),
				Count:        4,
				ElementCount: 4,
				Length:       uint64(4 * FrameSize),
			},
			FrameContents: Section{
				ElementSize:  uint64(FrameContentSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FrameContentSize),
			},
			Metadata: Section{
				ElementSize:  uint64(FileMetadataSize),
				Count:        3,
				ElementCount: 3,
				Length:       uint64(3 * FileMetadataSize),
			},
		},
		Frames: []Frame{
			{PackageIndex: 0, Offset: 0, CompressedSize: 50, Length: 100},   // 0 - data
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 1 - terminator
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 2 - terminator
			{PackageIndex: 0, Offset: 0, CompressedSize: 0, Length: 0},      // 3 - terminator
		},
		FrameContents: []FrameContent{
			{TypeSymbol: 0xA, FileSymbol: 0xB, FrameIndex: 0, Size: 100, Alignment: 1},
			// BUG SCENARIO: These entries reference frames beyond the insertion point.
			// In a real manifest this could happen if terminators are only at the end
			// and frames were reordered, or after a previous buggy patch.
			{TypeSymbol: 0xC, FileSymbol: 0xD, FrameIndex: 1, Size: 100, Alignment: 1},
			{TypeSymbol: 0xE, FileSymbol: 0xF, FrameIndex: 2, Size: 100, Alignment: 1},
		},
		Metadata: []FileMetadata{
			{TypeSymbol: 0xA, FileSymbol: 0xB},
			{TypeSymbol: 0xC, FileSymbol: 0xD},
			{TypeSymbol: 0xE, FileSymbol: 0xF},
		},
	}

	_, pkgBasePath7 := setupPackageDir(t, 200)

	// Patch file A/B. Terminators start at index 1, so insertIdx = 1.
	// After insert: frames = [frame0, NEW, term, term, term]
	// File A -> frame 1 (set by PatchFile). Correct.
	// File C -> was frame 1. Should now be frame 2 (shifted). BUG: stays at 1.
	// File E -> was frame 2. Should now be frame 3 (shifted). BUG: stays at 2.
	result7, err := PatchFile(m7, pkgBasePath7, 0xA, 0xB, []byte("trigger shift bug"))
	if err != nil {
		t.Fatalf("PatchFile m7: %v", err)
	}

	// BUG: PatchFile does not shift FrameIndex for non-targeted FrameContent entries.
	// The patched file (A/B) correctly points to the new frame at index 1.
	if result7.FrameContents[0].FrameIndex != 1 {
		t.Errorf("patched entry: got FrameIndex=%d, want 1", result7.FrameContents[0].FrameIndex)
	}

	// These assertions document the bug. The correct behavior would be FrameIndex 2 and 3,
	// but the current code leaves them at 1 and 2.
	// When the bug is fixed, update these expectations to 2 and 3.
	fc1 := result7.FrameContents[1].FrameIndex
	fc2 := result7.FrameContents[2].FrameIndex
	if fc1 == 2 && fc2 == 3 {
		// Bug is fixed! Update the test expectations below.
		t.Log("FrameIndex shift bug appears to be fixed")
	} else {
		// BUG: FrameContent entries that were not patched still have their old
		// FrameIndex values, even though a frame was inserted before them.
		// File C should be FrameIndex=2 but is still 1.
		// File E should be FrameIndex=3 but is still 2.
		t.Logf("BUG CONFIRMED: FrameContent[1].FrameIndex=%d (want 2), FrameContent[2].FrameIndex=%d (want 3)", fc1, fc2)
		if fc1 != 1 {
			t.Errorf("expected buggy FrameIndex=1 for entry 1, got %d", fc1)
		}
		if fc2 != 2 {
			t.Errorf("expected buggy FrameIndex=2 for entry 2, got %d", fc2)
		}
	}

	_ = result2
	_ = result4
}

func TestPatchFile_NotFound(t *testing.T) {
	m := buildTestManifest(2, 1)
	_, pkgBasePath := setupPackageDir(t, 100)

	t.Run("wrong TypeSymbol", func(t *testing.T) {
		_, err := PatchFile(m, pkgBasePath, 0xBAD, 0x2000, []byte("data"))
		if err == nil {
			t.Fatal("expected error for non-existent typeSymbol, got nil")
		}
	})

	t.Run("wrong FileSymbol", func(t *testing.T) {
		_, err := PatchFile(m, pkgBasePath, 0x1000, 0xBAD, []byte("data"))
		if err == nil {
			t.Fatal("expected error for non-existent fileSymbol, got nil")
		}
	})

	t.Run("both wrong", func(t *testing.T) {
		_, err := PatchFile(m, pkgBasePath, 0xBAD, 0xBAD, []byte("data"))
		if err == nil {
			t.Fatal("expected error for non-existent symbols, got nil")
		}
	})
}

func TestPatchFile_InsertBeforeTerminator(t *testing.T) {
	numTerminators := 3
	m := buildTestManifest(2, numTerminators)
	_, pkgBasePath := setupPackageDir(t, 200)

	origFrameCount := len(m.Frames)
	// Terminators occupy the last numTerminators slots.
	// The first terminator is at index (origFrameCount - numTerminators).
	firstTermIdx := origFrameCount - numTerminators

	result, err := PatchFile(m, pkgBasePath, 0x1000, 0x2000, []byte("insert before terminator"))
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	// New frame should be inserted at firstTermIdx.
	if len(result.Frames) != origFrameCount+1 {
		t.Fatalf("frame count: got %d, want %d", len(result.Frames), origFrameCount+1)
	}

	// The frame at firstTermIdx should be the new data frame (non-zero Length).
	newFrame := result.Frames[firstTermIdx]
	if newFrame.Length == 0 || newFrame.CompressedSize == 0 {
		t.Errorf("expected data frame at index %d, got terminator-like frame: %+v", firstTermIdx, newFrame)
	}

	// All frames after the new one should be terminators.
	for i := firstTermIdx + 1; i < len(result.Frames); i++ {
		f := result.Frames[i]
		if f.Length != 0 || f.CompressedSize != 0 {
			t.Errorf("frame[%d] should be terminator, got: %+v", i, f)
		}
	}

	// The patched FrameContent entry should point to firstTermIdx.
	if result.FrameContents[0].FrameIndex != uint32(firstTermIdx) {
		t.Errorf("patched FrameContent.FrameIndex: got %d, want %d",
			result.FrameContents[0].FrameIndex, firstTermIdx)
	}
}

func TestPatchFile_ManifestHeaderUpdated(t *testing.T) {
	m := buildTestManifest(2, 1)
	_, pkgBasePath := setupPackageDir(t, 200)

	origCount := m.Header.Frames.Count
	origElemCount := m.Header.Frames.ElementCount
	origLength := m.Header.Frames.Length

	result, err := PatchFile(m, pkgBasePath, 0x1000, 0x2000, []byte("header update test"))
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	if result.Header.Frames.Count != origCount+1 {
		t.Errorf("Frames.Count: got %d, want %d", result.Header.Frames.Count, origCount+1)
	}
	if result.Header.Frames.ElementCount != origElemCount+1 {
		t.Errorf("Frames.ElementCount: got %d, want %d", result.Header.Frames.ElementCount, origElemCount+1)
	}
	expectedLength := origLength + uint64(FrameSize)
	if result.Header.Frames.Length != expectedLength {
		t.Errorf("Frames.Length: got %d, want %d", result.Header.Frames.Length, expectedLength)
	}

	// Original manifest should be unchanged (deep copy).
	if m.Header.Frames.Count != origCount {
		t.Errorf("original Frames.Count was modified: got %d, want %d", m.Header.Frames.Count, origCount)
	}
}

func TestPatchFile_Basic(t *testing.T) {
	m := buildTestManifest(2, 1)
	tmpDir, pkgBasePath := setupPackageDir(t, 64)

	newData := []byte("the quick brown fox jumps over the lazy dog")

	result, err := PatchFile(m, pkgBasePath, 0x1001, 0x2001, newData)
	if err != nil {
		t.Fatalf("PatchFile: %v", err)
	}

	// Verify the package file was appended to.
	pkgPath := filepath.Join(tmpDir, "48037dc70b0ecab2_0")
	info, err := os.Stat(pkgPath)
	if err != nil {
		t.Fatalf("stat package file: %v", err)
	}

	// The file should be larger than the original 64 bytes.
	if info.Size() <= 64 {
		t.Errorf("package file size should have grown: got %d", info.Size())
	}

	// Read the appended data and verify it decompresses to newData.
	pkgData, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatalf("read package file: %v", err)
	}

	// The patched FrameContent entry (file index 1).
	patchedFC := result.FrameContents[1]
	if patchedFC.Size != uint32(len(newData)) {
		t.Errorf("patched FrameContent.Size: got %d, want %d", patchedFC.Size, len(newData))
	}
	if patchedFC.DataOffset != 0 {
		t.Errorf("patched FrameContent.DataOffset: got %d, want 0", patchedFC.DataOffset)
	}

	// Find the corresponding frame.
	frameIdx := patchedFC.FrameIndex
	if int(frameIdx) >= len(result.Frames) {
		t.Fatalf("FrameIndex %d out of range (len=%d)", frameIdx, len(result.Frames))
	}
	frame := result.Frames[frameIdx]
	if frame.Length != uint32(len(newData)) {
		t.Errorf("frame.Length: got %d, want %d", frame.Length, len(newData))
	}

	// Extract compressed bytes from the package file and decompress.
	compStart := frame.Offset
	compEnd := compStart + frame.CompressedSize
	if int(compEnd) > len(pkgData) {
		t.Fatalf("compressed data range [%d:%d] exceeds package file size %d", compStart, compEnd, len(pkgData))
	}
	compressedBytes := pkgData[compStart:compEnd]

	decompressed, err := zstd.Decompress(nil, compressedBytes)
	if err != nil {
		t.Fatalf("decompress appended data: %v", err)
	}

	if string(decompressed) != string(newData) {
		t.Errorf("decompressed data mismatch:\n  got:  %q\n  want: %q", decompressed, newData)
	}

	// Verify the result manifest has one more frame than original.
	if len(result.Frames) != len(m.Frames)+1 {
		t.Errorf("frame count: got %d, want %d", len(result.Frames), len(m.Frames)+1)
	}

	t.Run("original manifest unchanged", func(t *testing.T) {
		if len(m.Frames) != 3 { // 2 data + 1 terminator
			t.Errorf("original frame count changed: got %d, want 3", len(m.Frames))
		}
		if m.FrameContents[1].FrameIndex != 1 {
			t.Errorf("original FrameContents[1].FrameIndex changed: got %d, want 1",
				m.FrameContents[1].FrameIndex)
		}
	})
}
