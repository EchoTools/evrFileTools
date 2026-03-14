package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/zstd"
	"github.com/EchoTools/evrFileTools/pkg/naming"
)

// typeExtension returns the file extension for a given type symbol.
func typeExtension(typeSymbol int64) string {
	return naming.FileExtension(naming.TypeSymbol(typeSymbol))
}

// Package represents a multi-part package file set.
type Package struct {
	manifest *Manifest
	files    []packageFile
}

type packageFile interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

// OpenPackage opens a multi-part package from the given base path.
// The path should be the package name without the _N suffix.
func OpenPackage(manifest *Manifest, basePath string) (*Package, error) {
	dir := filepath.Dir(basePath)
	stem := filepath.Base(basePath)
	count := manifest.PackageCount()

	pkg := &Package{
		manifest: manifest,
		files:    make([]packageFile, count),
	}

	for i := range count {
		path := filepath.Join(dir, fmt.Sprintf("%s_%d", stem, i))
		f, err := os.Open(path)
		if err != nil {
			pkg.Close()
			return nil, fmt.Errorf("open package %d: %w", i, err)
		}
		pkg.files[i] = f
	}

	return pkg, nil
}

// Close closes all package files.
func (p *Package) Close() error {
	var lastErr error
	for _, f := range p.files {
		if f != nil {
			if err := f.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// Manifest returns the associated manifest.
func (p *Package) Manifest() *Manifest {
	return p.manifest
}

// Extract extracts all files from the package to the output directory.
func (p *Package) Extract(outputDir string, opts ...ExtractOption) error {
	cfg := &extractConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build frame index for O(1) lookup instead of O(n) scan per frame
	frameIndex := make(map[uint32][]FrameContent)
	for _, fc := range p.manifest.FrameContents {
		frameIndex[fc.FrameIndex] = append(frameIndex[fc.FrameIndex], fc)
	}

	ctx := zstd.NewCtx()
	compressed := make([]byte, 32*1024*1024)
	decompressed := make([]byte, 32*1024*1024)

	// Pre-create directory cache to avoid repeated MkdirAll calls
	createdDirs := make(map[string]struct{})

	for frameIdx, frame := range p.manifest.Frames {
		if frame.Length == 0 || frame.CompressedSize == 0 {
			continue
		}

		// Ensure buffers are large enough
		if int(frame.CompressedSize) > len(compressed) {
			compressed = make([]byte, frame.CompressedSize)
		}
		if int(frame.Length) > len(decompressed) {
			decompressed = make([]byte, frame.Length)
		}

		// Read compressed data
		file := p.files[frame.PackageIndex]
		if _, err := file.Seek(int64(frame.Offset), io.SeekStart); err != nil {
			return fmt.Errorf("seek frame %d: %w", frameIdx, err)
		}

		if _, err := io.ReadFull(file, compressed[:frame.CompressedSize]); err != nil {
			return fmt.Errorf("read frame %d: %w", frameIdx, err)
		}

		// Decompress
		if _, err := ctx.Decompress(decompressed[:frame.Length], compressed[:frame.CompressedSize]); err != nil {
			return fmt.Errorf("decompress frame %d: %w", frameIdx, err)
		}

		// Extract files from this frame using pre-built index
		contents := frameIndex[uint32(frameIdx)]
		for _, fc := range contents {
			// Determine directory name
			var typeDirName string
			if cfg.typeNames != nil {
				if name, ok := cfg.typeNames[fc.TypeSymbol]; ok {
					typeDirName = name
				}
			}
			if typeDirName == "" {
				typeDirName = strconv.FormatUint(uint64(fc.TypeSymbol), 16)
			}

			// Determine file name
			var fileName string
			var writeSidecar bool
			if cfg.nameTable != nil {
				if name, ok := cfg.nameTable[fc.FileSymbol]; ok {
					ext := typeExtension(fc.TypeSymbol)
					fileName = name + ext
					writeSidecar = true
				}
			}
			if fileName == "" {
				if cfg.decimalNames {
					fileName = strconv.FormatInt(fc.FileSymbol, 10)
				} else {
					fileName = strconv.FormatUint(uint64(fc.FileSymbol), 16)
				}
				// Still write sidecar for unnamed files when name table is active,
				// so the scanner can recover symbols without needing hex paths.
				if cfg.nameTable != nil {
					writeSidecar = true
				}
			}

			var basePath string
			if cfg.preserveGroups {
				basePath = filepath.Join(outputDir, strconv.FormatUint(uint64(fc.FrameIndex), 10), typeDirName)
			} else {
				basePath = filepath.Join(outputDir, typeDirName)
			}

			// Only create directory if not already created
			if _, exists := createdDirs[basePath]; !exists {
				if err := os.MkdirAll(basePath, 0755); err != nil {
					return fmt.Errorf("create dir %s: %w", basePath, err)
				}
				createdDirs[basePath] = struct{}{}
			}

			filePath := filepath.Join(basePath, fileName)
			if err := os.WriteFile(filePath, decompressed[fc.DataOffset:fc.DataOffset+fc.Size], 0644); err != nil {
				return fmt.Errorf("write file %s: %w", filePath, err)
			}

			if writeSidecar {
				if err := WriteSidecar(filePath, fc.TypeSymbol, fc.FileSymbol); err != nil {
					return fmt.Errorf("write sidecar %s: %w", filePath, err)
				}
			}
		}
	}

	return nil
}

// extractConfig holds extraction options.
type extractConfig struct {
	preserveGroups bool
	decimalNames   bool
	nameTable      map[int64]string // fileSymbol → friendly name (nil = disabled)
	typeNames      map[int64]string // typeSymbol → friendly dir name (nil = use hex)
}

// ExtractOption configures extraction behavior.
type ExtractOption func(*extractConfig)

// WithPreserveGroups preserves frame grouping in output directory structure.
func WithPreserveGroups(preserve bool) ExtractOption {
	return func(c *extractConfig) {
		c.preserveGroups = preserve
	}
}

// WithDecimalNames uses decimal format for filenames instead of hex.
func WithDecimalNames(decimal bool) ExtractOption {
	return func(c *extractConfig) {
		c.decimalNames = decimal
	}
}

// WithNameTable enables named extraction: files are written using friendly names
// from the provided fileSymbol→name map, with a .meta sidecar for round-trip rebuilding.
// Files without a known name fall back to the hex file symbol.
func WithNameTable(table map[int64]string) ExtractOption {
	return func(c *extractConfig) {
		c.nameTable = table
	}
}

// WithTypeNames overrides the type directory names during named extraction.
// Keys are type symbols (int64), values are directory names.
func WithTypeNames(table map[int64]string) ExtractOption {
	return func(c *extractConfig) {
		c.typeNames = table
	}
}
