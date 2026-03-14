package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/zstd"
)

// PatchFile replaces one file (identified by typeSymbol+fileSymbol) in a package.
// It reads the original package, compresses the new data, appends a new frame,
// updates the FrameContent entry in a copy of the manifest, and writes the
// updated package and manifest to outDir.
//
// pkgBasePath is the package base path without the _N suffix (e.g. /out/packages/48037dc70b0ecab2).
// The caller is responsible for writing the returned manifest with WriteFile.
//
// Returns the updated Manifest (the caller should write it with WriteFile).
func PatchFile(m *Manifest, pkgBasePath string, typeSymbol, fileSymbol int64, newData []byte) (*Manifest, error) {
	// Step 1: Find the FrameContent entry matching typeSymbol and fileSymbol.
	fcIdx := -1
	for i, fc := range m.FrameContents {
		if fc.TypeSymbol == typeSymbol && fc.FileSymbol == fileSymbol {
			fcIdx = i
			break
		}
	}
	if fcIdx < 0 {
		return nil, fmt.Errorf("no FrameContent entry found for typeSymbol=%016x fileSymbol=%016x", uint64(typeSymbol), uint64(fileSymbol))
	}

	// Step 2: Compress newData with ZSTD at DefaultCompressionLevel.
	compressed, err := zstd.CompressLevel(nil, newData, DefaultCompressionLevel)
	if err != nil {
		return nil, fmt.Errorf("compress new data: %w", err)
	}

	// Step 3: Deep-copy the manifest.
	newManifest := deepCopyManifest(m)

	// Step 4: Determine where to write.
	// Find the last package file size.
	dir := filepath.Dir(pkgBasePath)
	stem := filepath.Base(pkgBasePath)
	currentCount := int(newManifest.Header.PackageCount)
	lastIdx := currentCount - 1
	lastPkgPath := filepath.Join(dir, fmt.Sprintf("%s_%d", stem, lastIdx))

	info, err := os.Stat(lastPkgPath)
	if err != nil {
		return nil, fmt.Errorf("stat last package file %s: %w", lastPkgPath, err)
	}
	currentSize := info.Size()

	var (
		writePackageIdx int
		writeOffset     uint32
		pkgPath         string
	)

	if currentSize+int64(len(compressed)) > int64(MaxPackageSize) {
		// Need a new package file.
		writePackageIdx = currentCount
		writeOffset = 0
		pkgPath = filepath.Join(dir, fmt.Sprintf("%s_%d", stem, writePackageIdx))
		newManifest.Header.PackageCount++
	} else {
		writePackageIdx = lastIdx
		writeOffset = uint32(currentSize)
		pkgPath = lastPkgPath
	}

	// Step 5: Open the package file for appending and write compressed bytes.
	f, err := os.OpenFile(pkgPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open package file %s: %w", pkgPath, err)
	}

	// Seek to end to confirm the actual offset (handles newly created files too).
	actualOffset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("seek to end of package file: %w", err)
	}
	writeOffset = uint32(actualOffset)

	if _, err := f.Write(compressed); err != nil {
		f.Close()
		return nil, fmt.Errorf("write compressed data to package: %w", err)
	}
	f.Close()

	// Step 6: Add a new Frame to the copied manifest.
	newFrame := Frame{
		PackageIndex:   uint32(writePackageIdx),
		Offset:         writeOffset,
		CompressedSize: uint32(len(compressed)),
		Length:         uint32(len(newData)),
	}

	// Insert the new frame before any terminator frames at the end.
	// Terminators are frames with Length == 0 and CompressedSize == 0.
	insertIdx := len(newManifest.Frames)
	for insertIdx > 0 {
		prev := newManifest.Frames[insertIdx-1]
		if prev.Length == 0 && prev.CompressedSize == 0 {
			insertIdx--
		} else {
			break
		}
	}

	newFrames := make([]Frame, 0, len(newManifest.Frames)+1)
	newFrames = append(newFrames, newManifest.Frames[:insertIdx]...)
	newFrames = append(newFrames, newFrame)
	newFrames = append(newFrames, newManifest.Frames[insertIdx:]...)
	newManifest.Frames = newFrames

	newFrameIndex := uint32(insertIdx)

	// Step 7: Update the FrameContent entry.
	newManifest.FrameContents[fcIdx].FrameIndex = newFrameIndex
	newManifest.FrameContents[fcIdx].DataOffset = 0
	newManifest.FrameContents[fcIdx].Size = uint32(len(newData))

	// Step 8: Update manifest header section counts/lengths for Frames.
	newManifest.Header.Frames.Count++
	newManifest.Header.Frames.ElementCount++
	newManifest.Header.Frames.Length += uint64(newManifest.Header.Frames.ElementSize)

	return newManifest, nil
}

// deepCopyManifest returns a deep copy of a Manifest.
func deepCopyManifest(m *Manifest) *Manifest {
	cp := &Manifest{
		Header: m.Header,
	}

	cp.FrameContents = make([]FrameContent, len(m.FrameContents))
	copy(cp.FrameContents, m.FrameContents)

	cp.Metadata = make([]FileMetadata, len(m.Metadata))
	copy(cp.Metadata, m.Metadata)

	cp.Frames = make([]Frame, len(m.Frames))
	copy(cp.Frames, m.Frames)

	return cp
}
