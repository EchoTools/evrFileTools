package manifest

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/klauspost/compress/zstd"
)

var (
	readPool         = sync.Pool{New: func() interface{} { return make([]byte, 0, 1024*1024) }}
	decompPool       = sync.Pool{New: func() interface{} { return make([]byte, 0, 4*1024*1024) }}
	compPool         = sync.Pool{New: func() interface{} { return make([]byte, 0, 1024*1024) }}
	constructionPool = sync.Pool{New: func() interface{} { return bytes.NewBuffer(make([]byte, 0, 4*1024*1024)) }}
	encoderPool      = sync.Pool{
		New: func() interface{} {
			enc, _ := zstd.NewWriter(nil,
				zstd.WithEncoderCRC(false),
				zstd.WithSingleSegment(true),
			)
			return enc
		},
	}
	decoderPool = sync.Pool{
		New: func() interface{} {
			dec, _ := zstd.NewReader(nil)
			return dec
		},
	}
)

const MaxRepackFrameSize = 500 * 1024 // Strict 500KB engine streaming chunk buffer limit

type FrameMetadataUpdate struct {
	FCIndex    int
	DataOffset uint32
	Size       uint32
}

type compressedFrame struct {
	data             []byte
	decompressedSize uint32
	metadataUpdates  []FrameMetadataUpdate
}

type frameResult struct {
	index      int
	isModified bool
	err        error
	frames     []compressedFrame // Supports splitting one original frame into multiple
	shouldSkip bool
	rawReadBuf []byte // For pool return
	decompBuf  []byte // For pool return
}


type fcWrapper struct {
	index int
	fc    FrameContent
}

type packageWriter struct {
	fileHandle    *os.File
	pkgIndex      uint32
	outputDir     string
	pkgName       string
	manifest      *Manifest
	created       map[uint32]bool
	currentOffset int64
	minPkgIndex   uint32
}

func (pw *packageWriter) write(manifest *Manifest, data []byte, decompressedSize uint32) error {
	os.MkdirAll(fmt.Sprintf("%s/packages", pw.outputDir), 0755)

	cEntry := Frame{}
	if len(manifest.Frames) > 0 {
		cEntry = manifest.Frames[len(manifest.Frames)-1]
	}
	activePackageNum := cEntry.PackageIndex

	// Ensure we don't write to protected original packages
	if activePackageNum < pw.minPkgIndex {
		activePackageNum = pw.minPkgIndex
	}

	// Ensure manifest knows about this package
	if manifest.Header.PackageCount <= activePackageNum {
		manifest.Header.PackageCount = activePackageNum + 1
	}

	// Check if the current frame forces a rotation, BUT only if we are still in the same package.
	// If we moved to a new package (activePackageNum > cEntry.PackageIndex), the offset of cEntry is irrelevant.
	if activePackageNum == cEntry.PackageIndex {
		if int64(cEntry.Offset)+int64(cEntry.CompressedSize)+int64(len(data)) > math.MaxInt32 {
			activePackageNum++
			manifest.Header.PackageCount = activePackageNum + 1
		}
	}

	// Open file and verify size constraints (handling existing files or rotation)
	for {
		if pw.fileHandle == nil || pw.pkgIndex != activePackageNum {
			if pw.fileHandle != nil {
				pw.fileHandle.Close()
			}

			currentPackagePath := fmt.Sprintf("%s/packages/%s_%d", pw.outputDir, pw.pkgName, activePackageNum)
			flags := os.O_RDWR | os.O_CREATE

			if !pw.created[activePackageNum] {
				flags |= os.O_TRUNC
				pw.created[activePackageNum] = true
			}

			f, err := os.OpenFile(currentPackagePath, flags, 0644)
			if err != nil {
				return err
			}
			pw.fileHandle = f
			pw.pkgIndex = activePackageNum

			// Get the actual size/offset
			size, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				return fmt.Errorf("seek to end of package: %w", err)
			}

			// Maintain 1-byte alignment (essentially no padding between frames)
			// This matches original engine expectations for tight packing.
			pw.currentOffset = size
		}

		// Check if data fits in the current package
		if pw.currentOffset+int64(len(data)) > math.MaxInt32 {
			activePackageNum++
			manifest.Header.PackageCount = activePackageNum + 1
			continue // Retry with next package
		}
		break // Fits
	}

	if _, err := pw.fileHandle.Write(data); err != nil {
		return err
	}

	newEntry := Frame{
		PackageIndex:   activePackageNum,
		Offset:         uint32(pw.currentOffset),
		CompressedSize: uint32(len(data)),
		Length:         decompressedSize,
	}

	manifest.Frames = append(manifest.Frames, newEntry)
	incrementSection(&manifest.Header.Frames, 1)
	pw.currentOffset += int64(len(data))

	return nil
}

// writeRaw writes compressed data to the current package and returns where it
// was written (packageIndex, byteOffset) WITHOUT touching manifest.Frames.
// Used by QuickRepack to do true in-place frame updates.
func (pw *packageWriter) writeRaw(data []byte) (activePackageNum uint32, writeOffset uint32, err error) {
	if err = os.MkdirAll(fmt.Sprintf("%s/packages", pw.outputDir), 0755); err != nil {
		return 0, 0, err
	}

	cEntry := Frame{}
	if len(pw.manifest.Frames) > 0 {
		cEntry = pw.manifest.Frames[len(pw.manifest.Frames)-1]
	}
	activePackageNum = cEntry.PackageIndex

	// Enforce minPkgIndex â€” do not write into vanilla package files
	if activePackageNum < pw.minPkgIndex {
		activePackageNum = pw.minPkgIndex
	}

	// If our handle is open to the wrong package, close it
	if pw.fileHandle != nil && pw.pkgIndex != activePackageNum {
		pw.fileHandle.Close()
		pw.fileHandle = nil
		pw.currentOffset = 0
	}

	// Handle package rotation (MaxInt32 is roughly 2GB limit of original engine)
	if pw.fileHandle != nil && pw.currentOffset+int64(len(data)) > math.MaxInt32 {
		pw.fileHandle.Close()
		pw.fileHandle = nil
		activePackageNum++
		pw.currentOffset = 0
	}

	// pkgPath is computed AFTER all activePackageNum adjustments
	pkgPath := fmt.Sprintf("%s/packages/%s_%d", pw.outputDir, pw.pkgName, activePackageNum)

	if pw.fileHandle == nil {
		f, ferr := os.OpenFile(pkgPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if ferr != nil {
			return 0, 0, fmt.Errorf("open package %d: %w", activePackageNum, ferr)
		}
		size, serr := f.Seek(0, io.SeekEnd)
		if serr != nil {
			f.Close()
			return 0, 0, fmt.Errorf("seek package %d: %w", activePackageNum, serr)
		}
		pw.fileHandle = f
		pw.pkgIndex = activePackageNum
		pw.currentOffset = size
	}

	writeOffset = uint32(pw.currentOffset)
	if _, werr := pw.fileHandle.Write(data); werr != nil {
		return 0, 0, fmt.Errorf("write package %d: %w", activePackageNum, werr)
	}

	pw.currentOffset += int64(len(data))
	return activePackageNum, writeOffset, nil
}

func (pw *packageWriter) close() {
	if pw.fileHandle != nil {
		pw.fileHandle.Close()
		pw.fileHandle = nil
	}
}

func incrementSection(s *Section, count int) {
	s.Count += uint64(count)
	s.ElementCount += uint64(count)
	s.Length += s.ElementSize * uint64(count)
}

func Repack(manifest *Manifest, fileMap [][]ScannedFile, outputDir, packageName, dataDir string) error {
	fmt.Println("Mapping modified files...")

	totalFiles := 0
	for _, chunk := range fileMap {
		totalFiles += len(chunk)
	}

	modifiedFilesLookupTable := make(map[[128]byte]ScannedFile, totalFiles)
	frameContentsLookupTable := make(map[[128]byte]FrameContent, manifest.Header.FrameContents.ElementCount)
	modifiedFrames := make(map[uint32]bool)
	newFiles := make([]ScannedFile, 0)

	fileSymbolResolver := make(map[uint64]int64)

	for _, v := range manifest.FrameContents {
		buf := [128]byte{}
		binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
		binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))
		frameContentsLookupTable[buf] = v
		fileSymbolResolver[uint64(v.FileSymbol)] = v.TypeSymbol
	}

	for gIdx, fileGroup := range fileMap {
		for i, v := range fileGroup {
			if v.TypeSymbol == 0 {
				if ts, ok := fileSymbolResolver[uint64(v.FileSymbol)]; ok {
					v.TypeSymbol = ts
					fileMap[gIdx][i].TypeSymbol = ts
				}
			}

			buf := [128]byte{}
			binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
			binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))

			if content, ok := frameContentsLookupTable[buf]; ok {
				modifiedFrames[content.FrameIndex] = true
				modifiedFilesLookupTable[buf] = v
			} else {
				newFiles = append(newFiles, v)
			}
		}
	}
	fmt.Printf("Mapped %d files to modify.\n", len(modifiedFilesLookupTable))

	contentsByFrame := make(map[uint32][]fcWrapper)
	for k, v := range manifest.FrameContents {
		contentsByFrame[v.FrameIndex] = append(contentsByFrame[v.FrameIndex], fcWrapper{index: k, fc: v})
	}

	newManifest := *manifest
	newManifest.Frames = make([]Frame, 0)
	origFramesHeader := manifest.Header.Frames
	newManifest.Header.PackageCount = 1
	newManifest.Header.Frames = Section{
		Unk1:        origFramesHeader.Unk1,
		Unk2:        origFramesHeader.Unk2,
		ElementSize: 16,
	}

	packages := make(map[uint32]*os.File)
	for i := 0; i < int(manifest.Header.PackageCount); i++ {
		pFilePath := fmt.Sprintf("%s/packages/%s_%d", dataDir, packageName, i)
		f, err := os.Open(pFilePath)
		if err != nil {
			return fmt.Errorf("failed to open package %s: %v", pFilePath, err)
		}
		packages[uint32(i)] = f
		defer f.Close()
	}

	totalFrames := int(manifest.Header.Frames.ElementCount)
	lookaheadSize := runtime.NumCPU() * 16
	futureResults := make(chan chan frameResult, lookaheadSize)
	writer := &packageWriter{outputDir: outputDir, pkgName: packageName, created: make(map[uint32]bool)}
	defer writer.close()

	go func() {
		defer close(futureResults)
		for i := 0; i < totalFrames; i++ {
			resultChan := make(chan frameResult, 1)
			futureResults <- resultChan

			go func(idx int, ch chan frameResult) {
				v := manifest.Frames[idx]
				isMod := modifiedFrames[uint32(idx)]
				res := frameResult{index: idx, isModified: isMod}

				rawReadBuf := readPool.Get().([]byte)
				if cap(rawReadBuf) < int(v.CompressedSize) {
					rawReadBuf = make([]byte, int(v.CompressedSize))
				} else {
					rawReadBuf = rawReadBuf[:v.CompressedSize]
				}
				res.rawReadBuf = rawReadBuf

				activeFile := packages[v.PackageIndex]
				if v.CompressedSize > 0 {
					if _, err := activeFile.ReadAt(rawReadBuf, int64(v.Offset)); err != nil {
						if v.Length == 0 {
							// For out-of-bounds dummy frames with Len:0, preserve them without skipping to match exact engine structure.
							res.frames = []compressedFrame{{data: []byte{}, decompressedSize: 0}}
							ch <- res
							return
						}
						res.err = err
						ch <- res
						return
					}
				}

				if !isMod {
					res.frames = []compressedFrame{{
						data:             rawReadBuf,
						decompressedSize: v.Length,
					}}
					ch <- res
					return
				}

				decompBuf := decompPool.Get().([]byte)
				decoder := decoderPool.Get().(*zstd.Decoder)
				decompBytes, err := decoder.DecodeAll(rawReadBuf, decompBuf[:0])
				decoderPool.Put(decoder)

				if err != nil {
					res.err = err
					ch <- res
					return
				}
				res.decompBuf = decompBytes

				sorted := make([]fcWrapper, 0)
				if contents, ok := contentsByFrame[uint32(idx)]; ok {
					sorted = append(sorted, contents...)
				}
				sort.Slice(sorted, func(a, b int) bool {
					return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
				})

				bufObj := constructionPool.Get()
				constructionBuf := bufObj.(*bytes.Buffer)
				constructionBuf.Reset()
				defer constructionPool.Put(bufObj)

				currentOffset := uint32(0)
				currentMetadata := make([]FrameMetadataUpdate, 0)

				shipFrame := func() {
					if constructionBuf.Len() == 0 && len(currentMetadata) == 0 {
						return
					}
					compBuf := compPool.Get().([]byte)
					encoder := encoderPool.Get().(*zstd.Encoder)
					encodedData := encoder.EncodeAll(constructionBuf.Bytes(), compBuf[:0])
					encoderPool.Put(encoder)

					res.frames = append(res.frames, compressedFrame{
						data:             encodedData,
						decompressedSize: uint32(constructionBuf.Len()),
						metadataUpdates:  currentMetadata,
					})

					constructionBuf.Reset()
					currentOffset = 0
					currentMetadata = make([]FrameMetadataUpdate, 0)
				}

				for j := 0; j < len(sorted); j++ {
					buf := [128]byte{}
					binary.LittleEndian.PutUint64(buf[0:64], uint64(sorted[j].fc.TypeSymbol))
					binary.LittleEndian.PutUint64(buf[64:128], uint64(sorted[j].fc.FileSymbol))

					align := sorted[j].fc.Alignment
					if align == 0 {
						align = 1
					}
					padding := (align - (currentOffset % align)) % align

					// Re-read or get from memory (already in memory for modFiles)
					var encodedData []byte
					if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
						fileData, err := os.ReadFile(modFile.Path)
						if err != nil {
							res.err = err
							ch <- res
							return
						}
						encodedData, err = encodeFile(fileData, int64(sorted[j].fc.TypeSymbol))
						if err != nil {
							res.err = err
							ch <- res
							return
						}
					} else {
						start := sorted[j].fc.DataOffset
						end := start + sorted[j].fc.Size
						encodedData = decompBytes[start:end]
					}

					// SPLIT CHECK: If this asset pushes us over 500KB, ship current and start new frame.
					// We only split if there's already data in the current frame.
					if constructionBuf.Len() > 0 && uint32(constructionBuf.Len())+padding+uint32(len(encodedData)) > MaxRepackFrameSize {
						shipFrame()
						padding = 0 // Reset padding for start of new frame
					}

					if padding > 0 {
						constructionBuf.Write(make([]byte, padding))
						currentOffset += padding
					}

					currentMetadata = append(currentMetadata, FrameMetadataUpdate{
						FCIndex:    sorted[j].index,
						DataOffset: currentOffset,
						Size:       uint32(len(encodedData)),
					})

					constructionBuf.Write(encodedData)
					currentOffset += uint32(len(encodedData))
				}
				shipFrame()
				ch <- res
			}(i, resultChan)

		}
	}()

	fmt.Println("Starting repack...")
	for resultCh := range futureResults {
		res := <-resultCh
		if res.err != nil {
			return res.err
		}

		if res.shouldSkip {
			if res.rawReadBuf != nil {
				readPool.Put(res.rawReadBuf)
			}
			if res.decompBuf != nil {
				decompPool.Put(res.decompBuf)
			}
			continue
		}

		// Process each frame produced (might be more than one due to splitting)
		for _, frame := range res.frames {
			newFrameIdx := uint32(len(newManifest.Frames))

			// Update all FrameContents assigned to THIS frame chunk
			for _, update := range frame.metadataUpdates {
				fc := &newManifest.FrameContents[update.FCIndex]
				fc.FrameIndex = newFrameIdx
				fc.DataOffset = update.DataOffset
				fc.Size = update.Size
			}

			if err := writer.write(&newManifest, frame.data, frame.decompressedSize); err != nil {
				return err
			}

			// Return compression buffer to pool if it was fresh
			if res.isModified {
				compPool.Put(frame.data)
			}
		}

		// Cleanup common buffers
		if res.rawReadBuf != nil {
			readPool.Put(res.rawReadBuf)
		}
		if res.decompBuf != nil {
			decompPool.Put(res.decompBuf)
		}
	}


	// Update Header PackageCount to match what was actually written
	if len(newManifest.Frames) > 0 {
		newManifest.Header.PackageCount = newManifest.Frames[len(newManifest.Frames)-1].PackageIndex + 1
	}

	// Process new files (append to end of package)
	if len(newFiles) > 0 {
		fmt.Printf("Adding %d new files...\n", len(newFiles))

		sort.Slice(newFiles, func(i, j int) bool {
			if newFiles[i].TypeSymbol != newFiles[j].TypeSymbol {
				return newFiles[i].TypeSymbol < newFiles[j].TypeSymbol
			}
			return newFiles[i].FileSymbol < newFiles[j].FileSymbol
		})

		var currentFrame bytes.Buffer
		var currentFrameFiles []ScannedFile

		flushFrame := func() error {
			if currentFrame.Len() == 0 {
				return nil
			}

			compBuf := compPool.Get().([]byte)
			encoder := encoderPool.Get().(*zstd.Encoder)
			encodedData := encoder.EncodeAll(currentFrame.Bytes(), compBuf[:0])
			encoderPool.Put(encoder)

			if err := writer.write(&newManifest, encodedData, uint32(currentFrame.Len())); err != nil {
				return err
			}

			frameIdx := uint32(len(newManifest.Frames) - 1)
			currentOffset := uint32(0)

			for _, file := range currentFrameFiles {
				align := uint32(16) // Standardize to 16 for new assets
				padding := (align - (currentOffset % align)) % align
				
				if padding > 0 {
					// We must insert padding into the frame bytes BEFORE decompressing.
					// Wait, the newFiles loop writes to currentFrame buffer.
					// So alignment must happen there!
				}

				newManifest.FrameContents = append(newManifest.FrameContents, FrameContent{
					TypeSymbol: file.TypeSymbol,
					FileSymbol: file.FileSymbol,
					FrameIndex: frameIdx,
					DataOffset: currentOffset,
					Size:       file.Size,
					Alignment:  align,
				})


				newManifest.Metadata = append(newManifest.Metadata, FileMetadata{
					TypeSymbol: file.TypeSymbol,
					FileSymbol: file.FileSymbol,
				})

				currentOffset += file.Size
			}

			compPool.Put(encodedData)
			currentFrame.Reset()
			currentFrameFiles = nil
			return nil
		}

		for _, file := range newFiles {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				return fmt.Errorf("read new file %s: %w", file.Path, err)
			}

			encodedData, encErr := encodeFile(data, int64(file.TypeSymbol))
			if encErr != nil {
				return fmt.Errorf("encode new file %s: %w", file.Path, encErr)
			}

			align := uint32(16)
			padding := (align - (uint32(currentFrame.Len()) % align)) % align

			if currentFrame.Len() > 0 && uint32(currentFrame.Len())+padding+uint32(len(encodedData)) > MaxRepackFrameSize {
				if err := flushFrame(); err != nil {
					return err
				}
				padding = 0 // Reset padding for new frame
			}

			if padding > 0 {
				currentFrame.Write(make([]byte, padding))
			}

			currentFrame.Write(encodedData)

			// Update the file size so the manifest has the correct stripped size
			file.Size = uint32(len(encodedData))
			currentFrameFiles = append(currentFrameFiles, file)
		}

		if err := flushFrame(); err != nil {
			return err
		}

		incrementSection(&newManifest.Header.FrameContents, len(newFiles))
		incrementSection(&newManifest.Header.Metadata, len(newFiles))
	}

	writer.close()

	for i := uint32(0); i < newManifest.Header.PackageCount; i++ {
		path := fmt.Sprintf("%s/packages/%s_%d", outputDir, packageName, i)
		stats, err := os.Stat(path)
		if err != nil {
			continue
		}
		newEntry := Frame{
			PackageIndex:   i,
			Offset:         uint32(stats.Size()),
			CompressedSize: 0, Length: 0,
		}
		newManifest.Frames = append(newManifest.Frames, newEntry)
		incrementSection(&newManifest.Header.Frames, 1)
	}

	newManifest.Frames = append(newManifest.Frames, Frame{})
	incrementSection(&newManifest.Header.Frames, 1)

	manifestDir := filepath.Join(outputDir, "manifests")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	return WriteFile(filepath.Join(manifestDir, packageName), &newManifest)
}

// QuickRepack modifies the existing package files in-place by appending new frames
// and updating the manifest. This avoids rewriting the entire package set.
func QuickRepack(manifest *Manifest, fileMap [][]ScannedFile, dataDir, packageName string) error {
	manifestPath := filepath.Join(dataDir, "manifests", packageName)
	originalManifestPath := manifestPath + ".bak"

	// 1. Backup/Restore Logic: Ensure we have a clean original manifest
	// Check for legacy backup first
	if _, err := os.Stat(manifestPath + "_original"); err == nil {
		if _, err := os.Stat(originalManifestPath); os.IsNotExist(err) {
			os.Rename(manifestPath+"_original", originalManifestPath)
		}
	}

	if _, err := os.Stat(originalManifestPath); os.IsNotExist(err) {
		// No backup, create one from current (assumed original)
		fmt.Println("Creating backup of original manifest...")
		input, err := os.ReadFile(manifestPath)
		if err == nil {
			os.WriteFile(originalManifestPath, input, 0644)
		}
	} else {
		// Backup exists, load it as the source of truth
		fmt.Println("Loading original manifest from backup...")
		origM, err := ReadFile(originalManifestPath)
		if err != nil {
			return fmt.Errorf("failed to read backup manifest: %w", err)
		}
		*manifest = *origM
	}

	// 2. Delete any delta packages left over from a previous QuickRepack run.
	// Delta packages are all indices >= original PackageCount. We never touch
	// original packages (indices 0..PackageCount-1) â€” they are read-only sources.
	originalPkgCount := manifest.Header.PackageCount
	for i := originalPkgCount; ; i++ {
		deltaPkgPath := filepath.Join(dataDir, "packages", fmt.Sprintf("%s_%d", packageName, i))
		if _, err := os.Stat(deltaPkgPath); os.IsNotExist(err) {
			break // no more delta packages
		}
		fmt.Printf("Removing stale delta package %d from previous run...\n", i)
		if err := os.Remove(deltaPkgPath); err != nil {
			return fmt.Errorf("remove stale delta package %d: %w", i, err)
		}
	}

	// 3. Open Package â€” AFTER truncation so reads see clean original data
	pkgPath := filepath.Join(dataDir, "packages", packageName)
	srcPkg, err := OpenPackage(manifest, pkgPath)
	if err != nil {
		return fmt.Errorf("failed to open source package: %w", err)
	}
	defer srcPkg.Close()

	fmt.Println("Starting Quick Swap (In-Place Modification)...")

	totalFiles := 0
	for _, chunk := range fileMap {
		totalFiles += len(chunk)
	}

	modifiedFilesLookupTable := make(map[[128]byte]ScannedFile, totalFiles)
	frameContentsLookupTable := make(map[[128]byte]FrameContent, manifest.Header.FrameContents.ElementCount)
	fileSymbolResolver := make(map[uint64]int64)

	for _, v := range manifest.FrameContents {
		buf := [128]byte{}
		binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
		binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))
		frameContentsLookupTable[buf] = v
		fileSymbolResolver[uint64(v.FileSymbol)] = v.TypeSymbol
	}

	newFilesByGroup := make(map[int][]ScannedFile)
	for gIdx, fileGroup := range fileMap {
		for i, v := range fileGroup {
			if v.TypeSymbol == 0 {
				if ts, ok := fileSymbolResolver[uint64(v.FileSymbol)]; ok {
					v.TypeSymbol = ts
					fileMap[gIdx][i].TypeSymbol = ts
				}
			}

			buf := [128]byte{}
			binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
			binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))

			if _, ok := frameContentsLookupTable[buf]; ok {
				modifiedFilesLookupTable[buf] = v
			} else if v.TypeSymbol != 0 && v.FileSymbol != 0 {
				// Not in manifest â€” brand-new file, pack as a new frame
				newFilesByGroup[gIdx] = append(newFilesByGroup[gIdx], fileMap[gIdx][i])
			}
		}
	}

	if len(modifiedFilesLookupTable) == 0 && len(newFilesByGroup) == 0 {
		fmt.Println("No files changed or added. Nothing to repack.")
		return nil
	}
	fmt.Printf("Found %d file(s) to replace, %d group(s) of new file(s) to add.\n", len(modifiedFilesLookupTable), len(newFilesByGroup))

	affectedFrames := make(map[uint32]bool)
	for key := range modifiedFilesLookupTable {
		if fc, ok := frameContentsLookupTable[key]; ok {
			affectedFrames[fc.FrameIndex] = true
		}
	}
	fmt.Printf("Mapped %d files to modify across %d frames.\n", len(modifiedFilesLookupTable), len(affectedFrames))

	// Truncate Frames to remove old terminators and null frames from the end of the package before appending new ones.
	// Only strip frames where BOTH CompressedSize==0 AND Length==0 (true terminators).
	// Do NOT strip frames where only Length==0 â€” those may be valid zero-byte data entries.
	for len(manifest.Frames) > 0 {
		lastIdx := len(manifest.Frames) - 1
		f := manifest.Frames[lastIdx]
		if f.CompressedSize == 0 && f.Length == 0 {
			manifest.Frames = manifest.Frames[:lastIdx]
		} else {
			break
		}
	}

	allAssetsByFrame := make(map[uint32][]int)
	for k, v := range manifest.FrameContents {
		allAssetsByFrame[v.FrameIndex] = append(allAssetsByFrame[v.FrameIndex], k)
	}

	contentsByFrame := make(map[uint32][]fcWrapper)
	for k, v := range manifest.FrameContents {
		if affectedFrames[v.FrameIndex] {
			contentsByFrame[v.FrameIndex] = append(contentsByFrame[v.FrameIndex], fcWrapper{index: k, fc: v})
		}
	}


	pw := &packageWriter{
		outputDir:     dataDir,
		pkgName:       packageName,
		manifest:      manifest,
		created:       make(map[uint32]bool),
		minPkgIndex:   uint32(manifest.Header.PackageCount),
	}
	defer pw.close()

	var framesToProcess []int
	for idx := range affectedFrames {
		framesToProcess = append(framesToProcess, int(idx))
	}
	sort.Ints(framesToProcess)

	lookaheadSize := runtime.NumCPU() * 4
	futureResults := make(chan chan frameResult, lookaheadSize)

	go func() {
		defer close(futureResults)
		for _, idx := range framesToProcess {
			resultChan := make(chan frameResult, 1)
			futureResults <- resultChan

			go func(idx int, ch chan frameResult) {
				v := manifest.Frames[idx]
				res := frameResult{index: idx, isModified: true}

				rawReadBuf := readPool.Get().([]byte)
				if cap(rawReadBuf) < int(v.CompressedSize) {
					rawReadBuf = make([]byte, int(v.CompressedSize))
				} else {
					rawReadBuf = rawReadBuf[:v.CompressedSize]
				}
				res.rawReadBuf = rawReadBuf

				if int(v.PackageIndex) >= len(srcPkg.files) {
					res.err = fmt.Errorf("invalid package index %d", v.PackageIndex)
					ch <- res
					return
				}
				activeFile := srcPkg.files[v.PackageIndex]

				if v.CompressedSize > 0 {
					if _, err := activeFile.ReadAt(rawReadBuf, int64(v.Offset)); err != nil {
						res.err = err
						ch <- res
						return
					}
				}

				decompBuf := decompPool.Get().([]byte)
				decoder := decoderPool.Get().(*zstd.Decoder)
				decompBytes, err := decoder.DecodeAll(rawReadBuf, decompBuf[:0])
				decoderPool.Put(decoder)

				if err != nil {
					res.err = err
					ch <- res
					return
				}
				res.decompBuf = decompBytes

				sorted := make([]fcWrapper, 0)
				if contents, ok := contentsByFrame[uint32(idx)]; ok {
					sorted = append(sorted, contents...)
				}
				sort.Slice(sorted, func(a, b int) bool {
					return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
				})

				bufObj := constructionPool.Get()
				constructionBuf := bufObj.(*bytes.Buffer)
				constructionBuf.Reset()
				defer constructionPool.Put(bufObj)

				currentOffset := uint32(0)
				currentMetadata := make([]FrameMetadataUpdate, 0)

				shipFrame := func() {
					if constructionBuf.Len() == 0 && len(currentMetadata) == 0 {
						return
					}
					compBuf := compPool.Get().([]byte)
					encoder := encoderPool.Get().(*zstd.Encoder)
					encodedData := encoder.EncodeAll(constructionBuf.Bytes(), compBuf[:0])
					encoderPool.Put(encoder)

					res.frames = append(res.frames, compressedFrame{
						data:             encodedData,
						decompressedSize: uint32(constructionBuf.Len()),
						metadataUpdates:  currentMetadata,
					})

					constructionBuf.Reset()
					currentOffset = 0
					currentMetadata = make([]FrameMetadataUpdate, 0)
				}

				for j := 0; j < len(sorted); j++ {
					buf := [128]byte{}
					binary.LittleEndian.PutUint64(buf[0:64], uint64(sorted[j].fc.TypeSymbol))
					binary.LittleEndian.PutUint64(buf[64:128], uint64(sorted[j].fc.FileSymbol))

					align := sorted[j].fc.Alignment
					if align == 0 {
						align = 1
					}
					padding := (align - (currentOffset % align)) % align

					var encodedData []byte
					if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
						fileData, err := os.ReadFile(modFile.Path)
						if err != nil {
							res.err = err
							ch <- res
							return
						}
						encodedData, err = encodeFile(fileData, int64(sorted[j].fc.TypeSymbol))
						if err != nil {
							res.err = err
							ch <- res
							return
						}
					} else {
						start := sorted[j].fc.DataOffset
						end := start + sorted[j].fc.Size
						encodedData = decompBytes[start:end]
					}

					// Split if adding this asset exceeds the 500KB limit
					if constructionBuf.Len() > 0 && uint32(constructionBuf.Len())+padding+uint32(len(encodedData)) > MaxRepackFrameSize {
						shipFrame()
						padding = 0 // Reset padding for new frame
					}

					if padding > 0 {
						constructionBuf.Write(make([]byte, padding))
						currentOffset += padding
					}

					currentMetadata = append(currentMetadata, FrameMetadataUpdate{
						FCIndex:    sorted[j].index,
						DataOffset: currentOffset,
						Size:       uint32(len(encodedData)),
					})

					constructionBuf.Write(encodedData)
					currentOffset += uint32(len(encodedData))
				}
				shipFrame()
				ch <- res
			}(idx, resultChan)


		}
	}()

	fmt.Println("Writing frames...")
	newFrames := make([]Frame, 0, len(manifest.Frames))
	totalFrames := int(manifest.Header.Frames.ElementCount)
	
	// Track results for affected frames
	resByOrigIndex := make(map[int]frameResult)
	for resultCh := range futureResults {
		res := <-resultCh
		if res.err != nil {
			return res.err
		}
		resByOrigIndex[res.index] = res
	}

	for i := 0; i < totalFrames; i++ {
		if res, ok := resByOrigIndex[i]; ok {
			// Process modified/split frames
			for _, frame := range res.frames {
				newFrameIdx := uint32(len(newFrames))
				for _, update := range frame.metadataUpdates {
					fc := &manifest.FrameContents[update.FCIndex]
					fc.FrameIndex = newFrameIdx
					fc.DataOffset = update.DataOffset
					fc.Size = update.Size
				}

				pkgIdx, offset, err := pw.writeRaw(frame.data)
				if err != nil {
					return err
				}
				newFrames = append(newFrames, Frame{
					PackageIndex:   pkgIdx,
					Offset:         offset,
					CompressedSize: uint32(len(frame.data)),
					Length:         frame.decompressedSize,
				})
				compPool.Put(frame.data)
			}
			if res.rawReadBuf != nil {
				readPool.Put(res.rawReadBuf)
			}
			if res.decompBuf != nil {
				decompPool.Put(res.decompBuf)
			}
		} else {
			// For UNMODIFIED frames, we MUST still update their FrameIndex because
			// previous frames might have been split, shifting the indices.
			newFrameIdx := uint32(len(newFrames))
			origFrame := manifest.Frames[i]
			
			// Update assets that originally pointed to this frame index
			if assets, ok := allAssetsByFrame[uint32(i)]; ok {
				for _, k := range assets {
					manifest.FrameContents[k].FrameIndex = newFrameIdx
				}
			}
			newFrames = append(newFrames, origFrame)
		}

	}
	manifest.Frames = newFrames


	// Pack brand-new files (not replacing existing entries) as new frames in the delta package.
	// Each input group becomes its own frame, preserving the group structure from the input directory.
	if len(newFilesByGroup) > 0 {
		fmt.Printf("Adding %d new file group(s) as new frames...\n", len(newFilesByGroup))

		// Sort group indices for deterministic, reproducible output order
		newGroupIndices := make([]int, 0, len(newFilesByGroup))
		for gIdx := range newFilesByGroup {
			newGroupIndices = append(newGroupIndices, gIdx)
		}
		sort.Ints(newGroupIndices)

		for _, gIdx := range newGroupIndices {
			files := newFilesByGroup[gIdx]
			frameBuf := &bytes.Buffer{}
			currentOffset := uint32(0)
			var newFCs []FrameContent
			var newMDs []FileMetadata

			flushFrame := func() error {
				if frameBuf.Len() == 0 {
					return nil
				}
				compBuf := compPool.Get().([]byte)
				encoder := encoderPool.Get().(*zstd.Encoder)
				encoded := encoder.EncodeAll(frameBuf.Bytes(), compBuf[:0])
				encoderPool.Put(encoder)

				pkgIdx, offset, err := pw.writeRaw(encoded)
				if err != nil {
					return fmt.Errorf("write new frame group: %w", err)
				}

				newFrameIdx := uint32(len(manifest.Frames))
				manifest.Frames = append(manifest.Frames, Frame{
					PackageIndex:   pkgIdx,
					Offset:         offset,
					CompressedSize: uint32(len(encoded)),
					Length:         uint32(frameBuf.Len()),
				})

				for i := range newFCs {
					if newFCs[i].FrameIndex == uint32(0xFFFFFFFF) { // Sentinel for current chunk
						newFCs[i].FrameIndex = newFrameIdx
					}
				}
				manifest.FrameContents = append(manifest.FrameContents, newFCs...)
				manifest.Metadata = append(manifest.Metadata, newMDs...)

				frameBuf.Reset()
				newFCs = nil
				newMDs = nil
				currentOffset = 0
				compPool.Put(encoded)
				return nil
			}

			for _, f := range files {
				data, err := os.ReadFile(f.Path)
				if err != nil {
					return fmt.Errorf("read new file %s: %w", f.Path, err)
				}
				
				encodedData, err := encodeFile(data, int64(f.TypeSymbol))
				if err != nil {
					return fmt.Errorf("encode new file %s: %w", f.Path, err)
				}

				align := uint32(16)
				padding := (align - (currentOffset % align)) % align

				if frameBuf.Len() > 0 && uint32(frameBuf.Len())+padding+uint32(len(encodedData)) > MaxRepackFrameSize {
					if err := flushFrame(); err != nil {
						return err
					}
					padding = 0
				}

				if padding > 0 {
					frameBuf.Write(make([]byte, padding))
					currentOffset += padding
				}

				newFCs = append(newFCs, FrameContent{
					TypeSymbol: f.TypeSymbol,
					FileSymbol: f.FileSymbol,
					FrameIndex: 0xFFFFFFFF, // Temporary sentinel
					DataOffset: currentOffset,
					Size:       uint32(len(encodedData)),
					Alignment:  align,
				})
				newMDs = append(newMDs, FileMetadata{
					TypeSymbol: f.TypeSymbol,
					FileSymbol: f.FileSymbol,
				})
				frameBuf.Write(encodedData)
				currentOffset += uint32(len(encodedData))
			}
			if err := flushFrame(); err != nil {
				return err
			}
		}
	}

	pw.close()


	// Determine the highest package index actually used (original + any new ones)
	highestPkg := manifest.Header.PackageCount - 1
	for _, f := range manifest.Frames {
		if f.CompressedSize > 0 && f.PackageIndex > highestPkg {
			highestPkg = f.PackageIndex
		}
	}
	manifest.Header.PackageCount = highestPkg + 1

	// Re-add terminator frames for ALL packages (original + newly created)
	for i := uint32(0); i <= highestPkg; i++ {
		path := fmt.Sprintf("%s/packages/%s_%d", dataDir, packageName, i)
		stats, err := os.Stat(path)
		if err != nil {
			continue
		}
		manifest.Frames = append(manifest.Frames, Frame{
			PackageIndex:   i,
			Offset:         uint32(stats.Size()),
			CompressedSize: 0,
			Length:         0,
		})
	}

	// Final global null terminator
	manifest.Frames = append(manifest.Frames, Frame{})

	// 5. Finalize Manifest Header
	// We MUST sync the manifest counts in the header or the engine will read garbage data.
	manifest.Header.Frames.Count = uint64(len(manifest.Frames))
	manifest.Header.Frames.ElementCount = uint64(len(manifest.Frames))
	manifest.Header.Frames.Length = uint64(len(manifest.Frames)) * 16 // Frame size is 16 bytes

	manifest.Header.FrameContents.Count = uint64(len(manifest.FrameContents))
	manifest.Header.FrameContents.ElementCount = uint64(len(manifest.FrameContents))
	manifest.Header.FrameContents.Length = uint64(len(manifest.FrameContents)) * 32

	manifest.Header.Metadata.Count = uint64(len(manifest.Metadata))
	manifest.Header.Metadata.ElementCount = uint64(len(manifest.Metadata))
	manifest.Header.Metadata.Length = uint64(len(manifest.Metadata)) * 40

	fmt.Printf("Updating manifest: %s\n", manifestPath)
	return WriteFile(manifestPath, manifest)
}
