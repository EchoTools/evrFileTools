package manifest

import (
	"fmt"
	"io/fs"
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

	// Source for repacking (optional)
	SrcPackage   *Package
	SrcContent   *FrameContent
	SkipManifest bool
}

// ScanFiles walks the input directory and returns files grouped by chunk number.
// The directory structure is expected to be: <inputDir>/<chunkNum>/<typeSymbol>/<fileSymbol>
func ScanFiles(inputDir string) ([][]ScannedFile, error) {
	var files [][]ScannedFile

	err := filepath.WalkDir(inputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(inputDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Normalize separators
		relPath = filepath.ToSlash(relPath)
		parts := strings.Split(relPath, "/")

		var chunkNum int64 = 0
		var typeStr, fileStr string

		if len(parts) == 3 {
			if c, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				chunkNum = c
				typeStr = parts[1]
				fileStr = parts[2]
			} else {
				typeStr = parts[1]
				fileStr = parts[2]
			}
		} else if len(parts) == 2 {
			typeStr = parts[0]
			fileStr = parts[1]
		} else {
			return nil // Skip
		}

		parseSymbol := func(s string) (int64, error) {
			s = strings.TrimSuffix(s, filepath.Ext(s))
			if u, err := strconv.ParseUint(s, 16, 64); err == nil {
				return int64(u), nil
			}
			return strconv.ParseInt(s, 10, 64)
		}

		typeSymbol, err := parseSymbol(typeStr)
		if err != nil {
			return nil
		}

		fileSymbol, err := parseSymbol(fileStr)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("get file info %s: %w", path, err)
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

		// Grow slice if needed
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
