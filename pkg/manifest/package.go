package manifest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/DataDog/zstd"
	"github.com/EchoTools/evrFileTools/pkg/naming"
	"github.com/EchoTools/evrFileTools/pkg/texture"
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
//
// Codec pipeline (lossless round-trip):
//   - TypeDDSTexture       → <name>.dds  (verbatim)
//   - TypeRawBCTexture     → <name>.dds  (DDS header prepended; stripped on repack)
//   - TypeTextureMetadata  → <name>.tmeta (verbatim; avoids collision with .meta sidecars)
//   - Audio (OGG/WAV/…)    → <name>.ogg/.wav/… (detected by magic bytes)
//   - Unknown              → <name>.<sniffed ext> (magic byte detection, fallback .bin)
//
// Sidecars (.meta) are always written so the builder can recover typeSymbol/fileSymbol
// for deterministic repackaging regardless of whether a name table is in use.
func (p *Package) Extract(outputDir string, opts ...ExtractOption) error {
	cfg := &extractConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Pass 1: collect TypeTextureMetadata into a map keyed by fileSymbol.
	// These are small (256 bytes each) and needed to reconstruct TypeRawBCTexture
	// files as proper DDS files. We decompress their frames here; the main pass
	// will decompress all frames including these again (acceptable overhead).
	texMeta := p.buildTextureMeta()

	// Build frame→contents index for O(1) lookup.
	frameIndex := make(map[uint32][]FrameContent)
	for _, fc := range p.manifest.FrameContents {
		frameIndex[fc.FrameIndex] = append(frameIndex[fc.FrameIndex], fc)
	}

	ctx := zstd.NewCtx()
	compressed := make([]byte, 32*1024*1024)
	decompressed := make([]byte, 32*1024*1024)
	createdDirs := make(map[string]struct{})

	for frameIdx, frame := range p.manifest.Frames {
		if frame.Length == 0 || frame.CompressedSize == 0 {
			continue
		}

		if int(frame.CompressedSize) > len(compressed) {
			compressed = make([]byte, frame.CompressedSize)
		}
		if int(frame.Length) > len(decompressed) {
			decompressed = make([]byte, frame.Length)
		}

		file := p.files[frame.PackageIndex]
		if _, err := file.Seek(int64(frame.Offset), io.SeekStart); err != nil {
			return fmt.Errorf("seek frame %d: %w", frameIdx, err)
		}
		if _, err := io.ReadFull(file, compressed[:frame.CompressedSize]); err != nil {
			return fmt.Errorf("read frame %d: %w", frameIdx, err)
		}
		if _, err := ctx.Decompress(decompressed[:frame.Length], compressed[:frame.CompressedSize]); err != nil {
			return fmt.Errorf("decompress frame %d: %w", frameIdx, err)
		}

		contents := frameIndex[uint32(frameIdx)]
		for _, fc := range contents {
			raw := decompressed[fc.DataOffset : fc.DataOffset+fc.Size]

			// --- Codec: decode payload to source-file bytes + extension ---
			fileData, ext := p.decode(fc, raw, texMeta)

			// --- Naming ---
			typeDirName := p.typeDirName(cfg, fc.TypeSymbol)
			fileName := p.fileName(cfg, fc, ext)

			// --- Write ---
			basePath := filepath.Join(outputDir, typeDirName)
			if cfg.preserveGroups {
				basePath = filepath.Join(outputDir, strconv.FormatUint(uint64(fc.FrameIndex), 10), typeDirName)
			}
			if _, exists := createdDirs[basePath]; !exists {
				if err := os.MkdirAll(basePath, 0755); err != nil {
					return fmt.Errorf("create dir %s: %w", basePath, err)
				}
				createdDirs[basePath] = struct{}{}
			}

			filePath := filepath.Join(basePath, fileName)
			if err := os.WriteFile(filePath, fileData, 0644); err != nil {
				return fmt.Errorf("write file %s: %w", filePath, err)
			}

			// Always write sidecar — stores original typeSymbol/fileSymbol for
			// lossless repackaging regardless of codec transformations.
			if err := WriteSidecar(filePath, fc.TypeSymbol, fc.FileSymbol); err != nil {
				return fmt.Errorf("write sidecar %s: %w", filePath, err)
			}
		}
	}

	return nil
}

// decode converts raw package bytes to source-file bytes and returns the
// appropriate file extension. This is where the decode half of the codec lives.
func (p *Package) decode(fc FrameContent, raw []byte, texMeta map[int64]*texture.TextureMetadata) ([]byte, string) {
	ts := naming.TypeSymbol(fc.TypeSymbol)

	switch ts {
	case naming.TypeRawBCTexture:
		// Reconstruct a proper DDS file by prepending the DDS header derived
		// from the paired TextureMetadata (same fileSymbol, different typeSymbol).
		if meta, ok := texMeta[fc.FileSymbol]; ok {
			dds, err := decodeRawBCTexture(raw, meta)
			if err == nil {
				return dds, ".dds"
			}
			// Fall through to raw on error.
		}
		// No metadata available: write raw with .bc extension as fallback.
		return raw, ".bc"

	case naming.TypeDDSTexture:
		return raw, ".dds"

	case naming.TypeTextureMetadata:
		return raw, ".tmeta"

	case naming.TypeAudioReference:
		return raw, ".aref"

	case naming.TypeAssetReference:
		return raw, ".aref"

	default:
		// Unknown type: sniff content to determine extension.
		return raw, sniffExtension(raw)
	}
}

// buildTextureMeta does a pre-pass over all frames to collect TypeTextureMetadata
// entries into a map[fileSymbol → *TextureMetadata]. Used by decode() to reconstruct
// TypeRawBCTexture files as proper DDS files.
func (p *Package) buildTextureMeta() map[int64]*texture.TextureMetadata {
	result := make(map[int64]*texture.TextureMetadata)

	// Quickly check if there are any TypeTextureMetadata entries at all.
	hasMeta := false
	for _, fc := range p.manifest.FrameContents {
		if naming.TypeSymbol(fc.TypeSymbol) == naming.TypeTextureMetadata {
			hasMeta = true
			break
		}
	}
	if !hasMeta {
		return result
	}

	// Identify which frames contain TypeTextureMetadata entries.
	metaFrames := make(map[uint32][]FrameContent)
	for _, fc := range p.manifest.FrameContents {
		if naming.TypeSymbol(fc.TypeSymbol) == naming.TypeTextureMetadata {
			metaFrames[fc.FrameIndex] = append(metaFrames[fc.FrameIndex], fc)
		}
	}

	ctx := zstd.NewCtx()
	compressed := make([]byte, 4*1024*1024)
	decompressed := make([]byte, 4*1024*1024)

	for frameIdx, frame := range p.manifest.Frames {
		if frame.Length == 0 || frame.CompressedSize == 0 {
			continue
		}
		contents, ok := metaFrames[uint32(frameIdx)]
		if !ok {
			continue
		}

		if int(frame.CompressedSize) > len(compressed) {
			compressed = make([]byte, frame.CompressedSize)
		}
		if int(frame.Length) > len(decompressed) {
			decompressed = make([]byte, frame.Length)
		}

		file := p.files[frame.PackageIndex]
		if _, err := file.Seek(int64(frame.Offset), io.SeekStart); err != nil {
			continue
		}
		if _, err := io.ReadFull(file, compressed[:frame.CompressedSize]); err != nil {
			continue
		}
		if _, err := ctx.Decompress(decompressed[:frame.Length], compressed[:frame.CompressedSize]); err != nil {
			continue
		}

		for _, fc := range contents {
			raw := decompressed[fc.DataOffset : fc.DataOffset+fc.Size]
			meta, err := parseTextureMetadata(raw)
			if err == nil {
				result[fc.FileSymbol] = meta
			}
		}
	}

	return result
}

// typeDirName returns the directory name to use for a given type symbol.
func (p *Package) typeDirName(cfg *extractConfig, typeSymbol int64) string {
	if cfg.typeNames != nil {
		if name, ok := cfg.typeNames[typeSymbol]; ok {
			return name
		}
	}
	return naming.TypeName(naming.TypeSymbol(typeSymbol))
}

// fileName returns the filename (including extension) for a FrameContent entry.
func (p *Package) fileName(cfg *extractConfig, fc FrameContent, ext string) string {
	if cfg.nameTable != nil {
		if name, ok := cfg.nameTable[fc.FileSymbol]; ok {
			return name + ext
		}
	}
	if cfg.decimalNames {
		return strconv.FormatInt(fc.FileSymbol, 10) + ext
	}
	return strconv.FormatUint(uint64(fc.FileSymbol), 16) + ext
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

// WithNameTable enables named extraction using a fileSymbol→name map.
// Files without a known name fall back to the hex file symbol.
func WithNameTable(table map[int64]string) ExtractOption {
	return func(c *extractConfig) {
		c.nameTable = table
	}
}

// WithTypeNames overrides the type directory names.
// Keys are type symbols (int64), values are directory names.
func WithTypeNames(table map[int64]string) ExtractOption {
	return func(c *extractConfig) {
		c.typeNames = table
	}
}
