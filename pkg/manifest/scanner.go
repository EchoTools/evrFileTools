package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ScannedFile represents a file scanned from an input directory for building packages.
type ScannedFile struct {
	TypeSymbol int64
	FileSymbol int64
	Path       string
	Size       uint32
}

// ScanFiles walks the input directory and returns files grouped by chunk number.
//
// Two directory layouts are supported:
//
//  1. Hex layout (original): <inputDir>/<chunkNum>/<typeSymbol>/<fileSymbol>
//     Symbols parsed from the path components.
//
//  2. Named layout (from named extraction): any directory structure where each
//     file has a companion .meta sidecar with typeSymbol/fileSymbol JSON.
//     All named files are placed into chunk 0.
//
// The two layouts can coexist: files with .meta sidecars take precedence.
func ScanFiles(inputDir string) ([][]ScannedFile, error) {
	var files [][]ScannedFile

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Skip sidecar files — they're read alongside their data file
		if strings.HasSuffix(path, ".meta") {
			return nil
		}

		// Try sidecar first
		typeSymbol, fileSymbol, err := ReadSidecar(path)
		if err != nil {
			return fmt.Errorf("read sidecar for %s: %w", path, err)
		}

		var chunkNum int64
		if typeSymbol != 0 || fileSymbol != 0 {
			// Named layout: sidecar provided symbols; all go into chunk 0
			chunkNum = 0
		} else {
			// Hex layout: parse symbols from path
			dir := filepath.Dir(path)
			parts := strings.Split(filepath.ToSlash(dir), "/")
			if len(parts) < 3 {
				return fmt.Errorf("invalid path structure (no sidecar, too few parts): %s", path)
			}

			chunkNum, err = strconv.ParseInt(parts[len(parts)-3], 10, 64)
			if err != nil {
				return fmt.Errorf("parse chunk number from %s: %w", path, err)
			}

			typeSymbol, err = strconv.ParseInt(parts[len(parts)-2], 10, 64)
			if err != nil {
				return fmt.Errorf("parse type symbol from %s: %w", path, err)
			}

			fileSymbol, err = strconv.ParseInt(filepath.Base(path), 10, 64)
			if err != nil {
				return fmt.Errorf("parse file symbol from %s: %w", path, err)
			}
		}

		size := info.Size()
		const maxUint32 = int64(^uint32(0))
		if size < 0 || size > maxUint32 {
			return fmt.Errorf("file too large: %s (size %d exceeds %d bytes)", path, size, maxUint32)
		}

		file := ScannedFile{
			TypeSymbol: typeSymbol,
			FileSymbol: fileSymbol,
			Path:       path,
			Size:       uint32(size),
		}

		for int(chunkNum) >= len(files) {
			files = append(files, nil)
		}
		files[chunkNum] = append(files[chunkNum], file)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}
