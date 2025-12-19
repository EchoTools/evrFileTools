package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/zstd"
)

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

	ctx := zstd.NewCtx()
	compressed := make([]byte, 32*1024*1024)
	decompressed := make([]byte, 32*1024*1024)
	filesWritten := 0

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

		// Extract files from this frame
		for _, fc := range p.manifest.FrameContents {
			if fc.FrameIndex != uint32(frameIdx) {
				continue
			}

			fileName := fmt.Sprintf("%x", fc.FileSymbol)
			fileType := fmt.Sprintf("%x", fc.TypeSymbol)

			var basePath string
			if cfg.preserveGroups {
				basePath = filepath.Join(outputDir, fmt.Sprintf("%d", fc.FrameIndex), fileType)
			} else {
				basePath = filepath.Join(outputDir, fileType)
			}

			if err := os.MkdirAll(basePath, 0755); err != nil {
				return fmt.Errorf("create dir %s: %w", basePath, err)
			}

			filePath := filepath.Join(basePath, fileName)
			if err := os.WriteFile(filePath, decompressed[fc.DataOffset:fc.DataOffset+fc.Size], 0644); err != nil {
				return fmt.Errorf("write file %s: %w", filePath, err)
			}

			filesWritten++
		}
	}

	return nil
}

// extractConfig holds extraction options.
type extractConfig struct {
	preserveGroups bool
}

// ExtractOption configures extraction behavior.
type ExtractOption func(*extractConfig)

// WithPreserveGroups preserves frame grouping in output directory structure.
func WithPreserveGroups(preserve bool) ExtractOption {
	return func(c *extractConfig) {
		c.preserveGroups = preserve
	}
}
