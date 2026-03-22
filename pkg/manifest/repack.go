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

	"github.com/DataDog/zstd"
)

// Pools to eliminate GC overhead
var (
	readPool         = sync.Pool{New: func() interface{} { return make([]byte, 0, 1024*1024) }}
	decompPool       = sync.Pool{New: func() interface{} { return make([]byte, 0, 4*1024*1024) }}
	compPool         = sync.Pool{New: func() interface{} { return make([]byte, 0, 1024*1024) }}
	constructionPool = sync.Pool{New: func() interface{} { return bytes.NewBuffer(make([]byte, 0, 4*1024*1024)) }}
)

const MaxRepackFrameSize = 1 * 1024 * 1024

type frameResult struct {
	index            int
	data             []byte
	err              error
	decompressedSize uint32
	isModified       bool
	shouldSkip       bool
	rawReadBuf       []byte
	decompBuf        []byte
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

			// Add 1-byte alignment padding (effectively no padding)
			if size > 0 && size%1 != 0 {
				padding := 1 - (size % 1)
				if _, err := f.Write(make([]byte, padding)); err != nil {
					return fmt.Errorf("pad package for alignment: %w", err)
				}
				size += padding
			}

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

	for _, v := range manifest.FrameContents {
		buf := [128]byte{}
		binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
		binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))
		frameContentsLookupTable[buf] = v
	}

	for _, fileGroup := range fileMap {
		for _, v := range fileGroup {
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
				res := frameResult{index: idx, isModified: isMod, decompressedSize: v.Length}

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
							res.data = []byte{} // Pass empty data
							ch <- res
							return
						}
						res.err = err
						ch <- res
						return
					}
				}

				if !isMod {
					res.data = rawReadBuf
					ch <- res
					return
				}

				decompBuf := decompPool.Get().([]byte)
				decompBytes, err := zstd.Decompress(decompBuf[:0], rawReadBuf)
				if err != nil {
					res.err = err
					ch <- res
					return
				}
				res.decompBuf = decompBytes

				bufObj := constructionPool.Get()
				constructionBuf := bufObj.(*bytes.Buffer)
				constructionBuf.Reset()
				defer constructionPool.Put(bufObj)

				sorted := make([]fcWrapper, 0)
				if contents, ok := contentsByFrame[uint32(idx)]; ok {
					sorted = append(sorted, contents...)
				}
				sort.Slice(sorted, func(a, b int) bool {
					return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
				})

				currentOffset := uint32(0)
				for j := 0; j < len(sorted); j++ {
					align := sorted[j].fc.Alignment
					if align == 0 {
						align = 1
					}
					padding := (align - (currentOffset % align)) % align
					if padding > 0 {
						constructionBuf.Write(make([]byte, padding))
						currentOffset += padding
					}

					buf := [128]byte{}
					binary.LittleEndian.PutUint64(buf[0:64], uint64(sorted[j].fc.TypeSymbol))
					binary.LittleEndian.PutUint64(buf[64:128], uint64(sorted[j].fc.FileSymbol))

					if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
						modData, err := os.ReadFile(modFile.Path)
						if err != nil {
							res.err = err
							ch <- res
							return
						}
						constructionBuf.Write(modData)
						currentOffset += uint32(len(modData))
					} else {
						start := sorted[j].fc.DataOffset
						end := start + sorted[j].fc.Size
						constructionBuf.Write(decompBytes[start:end])
						currentOffset += sorted[j].fc.Size
					}
				}

				compBuf := compPool.Get().([]byte)
				encodedData, err := zstd.CompressLevel(compBuf[:0], constructionBuf.Bytes(), zstd.BestSpeed)
				if err != nil {
					res.err = fmt.Errorf("compress frame: %w", err)
					ch <- res
					return
				}
				res.data = encodedData
				res.decompressedSize = uint32(constructionBuf.Len())

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
			if res.isModified && res.data != nil {
				compPool.Put(res.data)
			}
			continue
		}

		newFrameIdx := uint32(len(newManifest.Frames))

		// Update all contents belonging to this frame (modified or not) to account for shifts
		if contents, ok := contentsByFrame[uint32(res.index)]; ok {
			sorted := make([]fcWrapper, len(contents))
			copy(sorted, contents)
			sort.Slice(sorted, func(a, b int) bool {
				return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
			})

			currentOffset := uint32(0)
			for j := 0; j < len(sorted); j++ {
				fc := &newManifest.FrameContents[sorted[j].index]

				align := fc.Alignment
				if align == 0 {
					align = 1
				}
				padding := (align - (currentOffset % align)) % align
				currentOffset += padding

				size := fc.Size
				if res.isModified {
					buf := [128]byte{}
					binary.LittleEndian.PutUint64(buf[0:64], uint64(fc.TypeSymbol))
					binary.LittleEndian.PutUint64(buf[64:128], uint64(fc.FileSymbol))
					if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
						size = modFile.Size
					}
				}

				fc.FrameIndex = newFrameIdx
				fc.DataOffset = currentOffset
				fc.Size = size
				fc.Alignment = align

				currentOffset += size
			}
		}

		if err := writer.write(&newManifest, res.data, res.decompressedSize); err != nil {
			return err
		}

		if res.isModified {
			if res.rawReadBuf != nil {
				readPool.Put(res.rawReadBuf)
			}
			if res.decompBuf != nil {
				decompPool.Put(res.decompBuf)
			}
			if res.data != nil {
				compPool.Put(res.data)
			}
		} else {
			if res.data != nil {
				readPool.Put(res.data)
			}
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
			encodedData, err := zstd.CompressLevel(compBuf[:0], currentFrame.Bytes(), zstd.BestSpeed)
			if err != nil {
				return err
			}

			if err := writer.write(&newManifest, encodedData, uint32(currentFrame.Len())); err != nil {
				return err
			}

			frameIdx := uint32(len(newManifest.Frames) - 1)
			currentOffset := uint32(0)

			for _, file := range currentFrameFiles {
				align := uint32(1)
				padding := (align - (currentOffset % align)) % align
				currentOffset += padding

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

			// Align file data within the frame
			align := uint32(8)
			padding := (align - (uint32(currentFrame.Len()) % align)) % align
			if padding > 0 {
				currentFrame.Write(make([]byte, padding))
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

			align := uint32(8)
			padding := (align - (uint32(currentFrame.Len()) % align)) % align

			if currentFrame.Len() > 0 && currentFrame.Len()+int(padding)+len(data) > MaxRepackFrameSize {
				if err := flushFrame(); err != nil {
					return err
				}
				padding = 0
			}

			if padding > 0 {
				currentFrame.Write(make([]byte, padding))
			}

			currentFrame.Write(data)
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

	if _, err := os.Stat(originalManifestPath); err == nil {
		// Backup exists, load it as the source of truth
		fmt.Println("Loading original manifest from backup...")
		origM, err := ReadFile(originalManifestPath)
		if err != nil {
			return fmt.Errorf("failed to read backup manifest: %w", err)
		}
		*manifest = *origM
	} else {
		// No backup, create one from current (assumed original)
		fmt.Println("Creating backup of original manifest...")
		input, err := os.ReadFile(manifestPath)
		if err == nil {
			os.WriteFile(originalManifestPath, input, 0644)
		}
	}

	minSafePackageIndex := manifest.Header.PackageCount

	// 2. Open Package
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

	for _, v := range manifest.FrameContents {
		buf := [128]byte{}
		binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
		binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))
		frameContentsLookupTable[buf] = v
	}

	for _, fileGroup := range fileMap {
		for _, v := range fileGroup {
			buf := [128]byte{}
			binary.LittleEndian.PutUint64(buf[0:64], uint64(v.TypeSymbol))
			binary.LittleEndian.PutUint64(buf[64:128], uint64(v.FileSymbol))

			if _, ok := frameContentsLookupTable[buf]; ok {
				modifiedFilesLookupTable[buf] = v
			}
		}
	}

	fmt.Println("Checking for identical files...")
	type checkItem struct {
		key [128]byte
		fc  FrameContent
		mod ScannedFile
	}
	var checks []checkItem
	for key, modFile := range modifiedFilesLookupTable {
		if fc, ok := frameContentsLookupTable[key]; ok {
			checks = append(checks, checkItem{key, fc, modFile})
		}
	}

	sort.Slice(checks, func(i, j int) bool {
		if checks[i].fc.FrameIndex != checks[j].fc.FrameIndex {
			return checks[i].fc.FrameIndex < checks[j].fc.FrameIndex
		}
		return checks[i].fc.DataOffset < checks[j].fc.DataOffset
	})

	skippedCount := 0
	for _, item := range checks {
		newData, err := os.ReadFile(item.mod.Path)
		if err != nil {
			return fmt.Errorf("read input %s: %w", item.mod.Path, err)
		}

		if uint32(len(newData)) == item.fc.Size {
			oldData, err := srcPkg.ReadContent(&item.fc)
			if err == nil && bytes.Equal(newData, oldData) {
				delete(modifiedFilesLookupTable, item.key)
				skippedCount++
			}
		}
	}

	if skippedCount > 0 {
		fmt.Printf("Skipped %d identical files.\n", skippedCount)
	}

	if len(modifiedFilesLookupTable) == 0 {
		fmt.Println("No files changed. Nothing to repack.")
		return nil
	}

	affectedFrames := make(map[uint32]bool)
	for key := range modifiedFilesLookupTable {
		if fc, ok := frameContentsLookupTable[key]; ok {
			affectedFrames[fc.FrameIndex] = true
		}
	}
	fmt.Printf("Mapped %d files to modify across %d frames.\n", len(modifiedFilesLookupTable), len(affectedFrames))

	// Truncate Frames to remove old terminators and null frames from the end of the package before appending new ones
	for len(manifest.Frames) > 0 {
		lastIdx := len(manifest.Frames) - 1
		f := manifest.Frames[lastIdx]
		if f.CompressedSize == 0 && f.Length == 0 {
			manifest.Frames = manifest.Frames[:lastIdx]
		} else {
			break
		}
	}

	contentsByFrame := make(map[uint32][]fcWrapper)
	for k, v := range manifest.FrameContents {
		if affectedFrames[v.FrameIndex] {
			contentsByFrame[v.FrameIndex] = append(contentsByFrame[v.FrameIndex], fcWrapper{index: k, fc: v})
		}
	}

	createdMap := make(map[uint32]bool)
	for i := uint32(0); i < manifest.Header.PackageCount; i++ {
		createdMap[i] = true
	}

	writer := &packageWriter{
		outputDir:   dataDir,
		pkgName:     packageName,
		created:     createdMap,
		minPkgIndex: minSafePackageIndex,
	}
	defer writer.close()

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
				res := frameResult{index: idx, isModified: true, decompressedSize: v.Length}

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
				decompBytes, err := zstd.Decompress(decompBuf[:0], rawReadBuf)
				if err != nil {
					res.err = err
					ch <- res
					return
				}
				res.decompBuf = decompBytes

				bufObj := constructionPool.Get()
				constructionBuf := bufObj.(*bytes.Buffer)
				constructionBuf.Reset()
				defer constructionPool.Put(bufObj)

				sorted := make([]fcWrapper, 0)
				if contents, ok := contentsByFrame[uint32(idx)]; ok {
					sorted = append(sorted, contents...)
				}
				sort.Slice(sorted, func(a, b int) bool {
					return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
				})

				for j := 0; j < len(sorted); j++ {
					align := sorted[j].fc.Alignment
					if align == 0 {
						align = 1
					}
					padding := (align - (uint32(constructionBuf.Len()) % align)) % align
					if padding > 0 {
						constructionBuf.Write(make([]byte, padding))
					}

					buf := [128]byte{}
					binary.LittleEndian.PutUint64(buf[0:64], uint64(sorted[j].fc.TypeSymbol))
					binary.LittleEndian.PutUint64(buf[64:128], uint64(sorted[j].fc.FileSymbol))

					if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
						modData, err := os.ReadFile(modFile.Path)
						if err != nil {
							res.err = err
							ch <- res
							return
						}
						constructionBuf.Write(modData)
					} else {
						start := sorted[j].fc.DataOffset
						end := start + sorted[j].fc.Size
						if end > uint32(len(decompBytes)) {
							res.err = fmt.Errorf("frame content out of bounds")
							ch <- res
							return
						}
						constructionBuf.Write(decompBytes[start:end])
					}
				}

				compBuf := compPool.Get().([]byte)
				encodedData, err := zstd.CompressLevel(compBuf[:0], constructionBuf.Bytes(), zstd.BestSpeed)
				if err != nil {
					res.err = err
					ch <- res
					return
				}
				res.data = encodedData
				res.decompressedSize = uint32(constructionBuf.Len())

				ch <- res
			}(idx, resultChan)
		}
	}()

	fmt.Println("Writing modified frames...")
	for resultCh := range futureResults {
		res := <-resultCh
		if res.err != nil {
			return res.err
		}

		newFrameIndex := len(manifest.Frames)

		sorted := make([]fcWrapper, 0)
		if contents, ok := contentsByFrame[uint32(res.index)]; ok {
			sorted = append(sorted, contents...)
		}
		sort.Slice(sorted, func(a, b int) bool {
			return sorted[a].fc.DataOffset < sorted[b].fc.DataOffset
		})

		currentOffset := uint32(0)
		for j := 0; j < len(sorted); j++ {
			fc := &manifest.FrameContents[sorted[j].index]

			align := fc.Alignment
			if align == 0 {
				align = 1
			}
			padding := (align - (currentOffset % align)) % align
			currentOffset += padding

			buf := [128]byte{}
			binary.LittleEndian.PutUint64(buf[0:64], uint64(fc.TypeSymbol))
			binary.LittleEndian.PutUint64(buf[64:128], uint64(fc.FileSymbol))

			size := fc.Size
			if modFile, exists := modifiedFilesLookupTable[buf]; exists && modFile.FileSymbol != 0 {
				size = modFile.Size
			}

			fc.FrameIndex = uint32(newFrameIndex)
			fc.DataOffset = currentOffset
			fc.Size = size
			fc.Alignment = align

			currentOffset += size
		}

		if err := writer.write(manifest, res.data, res.decompressedSize); err != nil {
			return err
		}

		if res.rawReadBuf != nil {
			readPool.Put(res.rawReadBuf)
		}
		if res.decompBuf != nil {
			decompPool.Put(res.decompBuf)
		}
		if res.data != nil {
			compPool.Put(res.data)
		}
	}

	writer.close()

	// Re-add terminators for all packages at their current positions
	for i := uint32(0); i < manifest.Header.PackageCount; i++ {
		path := filepath.Join(dataDir, "packages", fmt.Sprintf("%s_%d", packageName, i))
		stats, err := os.Stat(path)
		if err != nil {
			continue
		}
		// The RAD engine expects the terminator frame Offset to be exactly at the file end.
		// Align the file end to 1 byte first for consistency if we modified this package.
		if stats.Size()%1 != 0 {
			f, err := os.OpenFile(path, os.O_RDWR, 0644)
			if err == nil {
				size, _ := f.Seek(0, io.SeekEnd)
				if size%1 != 0 {
					padding := 1 - (size % 1)
					f.Write(make([]byte, int(padding)))
				}
				f.Close()
				stats, _ = os.Stat(path) // Re-stat for new size
			}
		}

		manifest.Frames = append(manifest.Frames, Frame{
			PackageIndex: i,
			Offset:       uint32(stats.Size()),
		})
		incrementSection(&manifest.Header.Frames, 1)
	}

	// Final null frame
	manifest.Frames = append(manifest.Frames, Frame{})
	incrementSection(&manifest.Header.Frames, 1)

	fmt.Printf("Updating manifest: %s\n", manifestPath)
	return WriteFile(manifestPath, manifest)
}
