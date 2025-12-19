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
// The directory structure is expected to be: <inputDir>/<chunkNum>/<typeSymbol>/<fileSymbol>
func ScanFiles(inputDir string) ([][]ScannedFile, error) {
	var files [][]ScannedFile

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Parse directory structure
		dir := filepath.Dir(path)
		parts := strings.Split(filepath.ToSlash(dir), "/")
		if len(parts) < 3 {
			return fmt.Errorf("invalid path structure: %s", path)
		}

		chunkNum, err := strconv.ParseInt(parts[len(parts)-3], 10, 64)
		if err != nil {
			return fmt.Errorf("parse chunk number: %w", err)
		}

		typeSymbol, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if err != nil {
			return fmt.Errorf("parse type symbol: %w", err)
		}

		fileSymbol, err := strconv.ParseInt(filepath.Base(path), 10, 64)
		if err != nil {
			return fmt.Errorf("parse file symbol: %w", err)
		}

		file := ScannedFile{
			TypeSymbol: typeSymbol,
			FileSymbol: fileSymbol,
			Path:       path,
			Size:       uint32(info.Size()),
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
