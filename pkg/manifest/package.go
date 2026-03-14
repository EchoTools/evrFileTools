package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/DataDog/zstd"
)

// Package represents a multi-part package file set.
type Package struct {
	manifest *Manifest
	files    []packageFile

	// Decompression cache
	lastFrameIdx  uint32
	lastFrameData []byte
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
		manifest:     manifest,
		files:        make([]packageFile, count),
		lastFrameIdx: ^uint32(0), // Invalid index
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
	p.lastFrameData = nil
	return lastErr
}

// Manifest returns the associated manifest.
func (p *Package) Manifest() *Manifest {
	return p.manifest
}

// ReadContent reads the data for a specific file content.
func (p *Package) ReadContent(fc *FrameContent) ([]byte, error) {
	// Check cache
	if p.lastFrameData != nil && p.lastFrameIdx == fc.FrameIndex {
		if uint32(len(p.lastFrameData)) < fc.DataOffset+fc.Size {
			return nil, fmt.Errorf("frame data too short for content")
		}
		return p.lastFrameData[fc.DataOffset : fc.DataOffset+fc.Size], nil
	}

	// Load frame
	if int(fc.FrameIndex) >= len(p.manifest.Frames) {
		return nil, fmt.Errorf("invalid frame index %d", fc.FrameIndex)
	}
	frame := p.manifest.Frames[fc.FrameIndex]

	if frame.Length == 0 {
		return nil, nil
	}

	// Read compressed data
	if int(frame.PackageIndex) >= len(p.files) {
		return nil, fmt.Errorf("invalid package index %d", frame.PackageIndex)
	}
	file := p.files[frame.PackageIndex]
	if _, err := file.Seek(int64(frame.Offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek frame %d: %w", fc.FrameIndex, err)
	}

	compressed := make([]byte, frame.CompressedSize)
	if _, err := io.ReadFull(file, compressed); err != nil {
		return nil, fmt.Errorf("read frame %d: %w", fc.FrameIndex, err)
	}

	// Decompress
	decompressed, err := zstd.Decompress(nil, compressed)
	if err != nil {
		return nil, fmt.Errorf("decompress frame %d: %w", fc.FrameIndex, err)
	}

	// Update cache
	p.lastFrameIdx = fc.FrameIndex
	p.lastFrameData = decompressed

	if uint32(len(decompressed)) < fc.DataOffset+fc.Size {
		return nil, fmt.Errorf("decompressed frame too short")
	}

	return decompressed[fc.DataOffset : fc.DataOffset+fc.Size], nil
}

// ReadRawFrame reads the raw compressed data for a specific frame.
func (p *Package) ReadRawFrame(frameIndex uint32) ([]byte, error) {
	if int(frameIndex) >= len(p.manifest.Frames) {
		return nil, fmt.Errorf("invalid frame index %d", frameIndex)
	}
	frame := p.manifest.Frames[frameIndex]

	if frame.Length == 0 {
		return nil, nil
	}

	if int(frame.PackageIndex) >= len(p.files) {
		return nil, fmt.Errorf("invalid package index %d", frame.PackageIndex)
	}
	file := p.files[frame.PackageIndex]
	if _, err := file.Seek(int64(frame.Offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek frame %d: %w", frameIndex, err)
	}

	compressed := make([]byte, frame.CompressedSize)
	if _, err := io.ReadFull(file, compressed); err != nil {
		return nil, fmt.Errorf("read frame %d: %w", frameIndex, err)
	}

	return compressed, nil
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

	// Pre-create directory cache to avoid repeated MkdirAll calls
	var dirMu sync.Mutex
	createdDirs := make(map[string]struct{})

	// Worker pool for parallel extraction
	numWorkers := runtime.NumCPU()
	type job struct {
		index int
		frame Frame
	}
	jobs := make(chan job, numWorkers)
	errs := make(chan error, 1)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Thread-local buffers and context
			ctx := zstd.NewCtx()
			var compressed []byte
			var decompressed []byte

			for j := range jobs {
				frame := j.frame
				frameIdx := j.index

				// Ensure buffers are large enough
				if int(frame.CompressedSize) > cap(compressed) {
					compressed = make([]byte, frame.CompressedSize)
				}
				compressed = compressed[:frame.CompressedSize]

				if int(frame.Length) > cap(decompressed) {
					decompressed = make([]byte, frame.Length)
				}
				decompressed = decompressed[:frame.Length]

				// Read compressed data using ReadAt (thread-safe)
				file := p.files[frame.PackageIndex]
				if _, err := file.ReadAt(compressed, int64(frame.Offset)); err != nil {
					select {
					case errs <- fmt.Errorf("read frame %d: %w", frameIdx, err):
					default:
					}
					return
				}

				// Decompress
				var err error
				decompressed, err = ctx.Decompress(decompressed[:0], compressed)
				if err != nil {
					select {
					case errs <- fmt.Errorf("decompress frame %d: %w", frameIdx, err):
					default:
					}
					return
				}

				// Extract files from this frame
				contents := frameIndex[uint32(frameIdx)]
				for _, fc := range contents {
					if len(cfg.allowedTypes) > 0 && !cfg.allowedTypes[fc.TypeSymbol] {
						continue
					}

					var fileName string
					if cfg.decimalNames {
						fileName = strconv.FormatInt(fc.FileSymbol, 10)
					} else {
						fileName = strconv.FormatUint(uint64(fc.FileSymbol), 16)
					}
					fileType := strconv.FormatUint(uint64(fc.TypeSymbol), 16)

					var basePath string
					if cfg.preserveGroups {
						basePath = filepath.Join(outputDir, strconv.FormatUint(uint64(fc.FrameIndex), 10), fileType)
					} else {
						basePath = filepath.Join(outputDir, fileType)
					}

					// Thread-safe directory creation
					dirMu.Lock()
					if _, exists := createdDirs[basePath]; !exists {
						if err := os.MkdirAll(basePath, 0755); err != nil {
							dirMu.Unlock()
							select {
							case errs <- fmt.Errorf("create dir %s: %w", basePath, err):
							default:
							}
							return
						}
						createdDirs[basePath] = struct{}{}
					}
					dirMu.Unlock()

					filePath := filepath.Join(basePath, fileName)
					if err := os.WriteFile(filePath, decompressed[fc.DataOffset:fc.DataOffset+fc.Size], 0644); err != nil {
						select {
						case errs <- fmt.Errorf("write file %s: %w", filePath, err):
						default:
						}
						return
					}
				}
			}
		}()
	}

	// Feed jobs
	go func() {
		for frameIdx, frame := range p.manifest.Frames {
			if frame.Length == 0 || frame.CompressedSize == 0 {
				continue
			}
			select {
			case jobs <- job{frameIdx, frame}:
			case <-errs:
				close(jobs)
				return
			}
		}
		close(jobs)
	}()

	wg.Wait()

	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

// extractConfig holds extraction options.
type extractConfig struct {
	preserveGroups bool
	decimalNames   bool
	allowedTypes   map[int64]bool
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

// WithTypeFilter configures extraction to only include specific file types.
func WithTypeFilter(types []int64) ExtractOption {
	return func(c *extractConfig) {
		if len(types) > 0 {
			c.allowedTypes = make(map[int64]bool, len(types))
			for _, t := range types {
				c.allowedTypes[t] = true
			}
		}
	}
}
