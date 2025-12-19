package tool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/zstd"
)

type FileMetadata struct { // Build manifest/package from this
	TypeSymbol       int64
	FileSymbol       int64
	ModifiedFilePath string
	FileSize         uint32
}

func ScanPackageFiles(inputDir string) ([][]FileMetadata, error) {
	files := make([][]FileMetadata, 0)

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Extract directory names
		dir := filepath.Dir(path)

		// The directory structure is expected to be:
		// <inputDir>/<chunkNum>/<typeSymbol>/<fileSymbol>
		// Example: /path/to/inputDir/0/123456/789012
		parts := strings.Split(filepath.ToSlash(dir), "/")
		if len(parts) < 3 {
			return fmt.Errorf("invalid file path structure: %s", path)
		}

		chunkNum, err := strconv.ParseInt(parts[len(parts)-3], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse chunk number: %w", err)
		}
		typeSymbol, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse type symbol: %w", err)
		}
		fileSymbol, err := strconv.ParseInt(filepath.Base(path), 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse file symbol: %w", err)
		}

		// Create FileMetadata
		newFile := FileMetadata{
			TypeSymbol:       typeSymbol,
			FileSymbol:       fileSymbol,
			ModifiedFilePath: path,
			FileSize:         uint32(info.Size()),
		}

		// Ensure files slice has enough capacity
		if int(chunkNum) >= len(files) {
			newFiles := make([][]FileMetadata, chunkNum+1)
			copy(newFiles, files)
			files = newFiles
		}

		files[chunkNum] = append(files[chunkNum], newFile)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

type PackageFile interface {
	io.Reader
	io.ReaderAt
	io.Closer
	io.Seeker
}

type Package struct {
	Manifest *ManifestBase
	Files    []PackageFile
}

func PackageOpenMultiPart(manifest *ManifestBase, path string) (*Package, error) {

	var (
		err      error
		stem     = filepath.Base(path)
		dirPath  = filepath.Dir(path)
		resource = &Package{
			Manifest: manifest,
			Files:    make([]PackageFile, manifest.PackageCount()),
		}
	)

	for i := range manifest.PackageCount() {
		path := filepath.Join(dirPath, fmt.Sprintf("%s_%d", stem, i))
		resource.Files[i], err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open package file %s: %w", path, err)
		}
	}

	return resource, nil
}

func PackageExtract(p *Package, outputDir string, preserveGroups bool) error {

	var (
		totalFilesWritten = 0
		zstdCtx           = zstd.NewCtx()
		compressed        = make([]byte, 32*1024*1024)
		decompressed      = make([]byte, 32*1024*1024)
	)
	for k, v := range p.Manifest.Frames {
		activeFile := p.Files[v.Index]

		if v.Length == 0 {
			continue
		}
		if v.CompressedSize == 0 {
			return fmt.Errorf("compressed size is 0 for file index %d", k)
		}

		if _, err := activeFile.Seek(int64(v.Offset), 0); err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", v.Offset, err)
		}

		if len(compressed) < int(v.CompressedSize) {
			compressed = make([]byte, v.CompressedSize)
		}

		if len(decompressed) < int(v.Length) {
			decompressed = make([]byte, v.Length)
		}

		if _, err := activeFile.Read(compressed[:v.Length]); err != nil {
			return fmt.Errorf("failed to read file, check input: %w", err)
		}

		fmt.Printf("Decompressing and extracting files contained in file index %d, %d/%d\n", k, totalFilesWritten, p.Manifest.Header.FrameContents.Count)
		if _, err := zstdCtx.Decompress(decompressed[:v.Length], compressed[:v.CompressedSize]); err != nil {
			fmt.Println("failed to decompress file, check input")
		}

		for _, v2 := range p.Manifest.FrameContents {
			if v2.FileIndex != uint32(k) {
				continue
			}
			fileName := fmt.Sprintf("%x", v2.FileSymbol)
			fileType := fmt.Sprintf("%x", v2.T)
			basePath := fmt.Sprintf("%s/%s", outputDir, fileType)
			if preserveGroups {
				basePath = fmt.Sprintf("%s/%d/%s", outputDir, v2.FileIndex, fileType)
			}
			os.MkdirAll(basePath, 0777)
			file, err := os.OpenFile(fmt.Sprintf("%s/%s", basePath, fileName), os.O_RDWR|os.O_CREATE, 0777)
			if err != nil {
				fmt.Println(err)
				continue
			}

			file.Write(decompressed[v2.DataOffset : v2.DataOffset+v2.Size])
			file.Close()
			totalFilesWritten++
		}
	}
	return nil
}

func Int64Hex(v int64) string {
	return fmt.Sprintf("%x", v)
}
