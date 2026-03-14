// texconv - Lossless DDS texture converter for EchoVR assets
//
// Converts between DDS (BC-compressed) and PNG (lossless) formats.
// BC formats are lossy, so this tool decompresses to RGBA and saves as PNG
// for lossless editing, then recompresses to BC format for game use.
//
// Supported formats:
//   - BC1 (DXT1): RGB, 1-bit alpha, 4bpp
//   - BC3 (DXT5): RGBA, 8bpp
//   - BC5: Two-channel (normal maps), 8bpp
//   - BC6H: HDR float, 8bpp
//   - BC7: High-quality RGBA, 8bpp
//
// Usage:
//   texconv decode input.dds output.png    # DDS → PNG (lossless storage)
//   texconv encode input.png output.dds    # PNG → DDS (BC compression)
//   texconv info input.dds                 # Show texture info
//   texconv batch decode dir/ out/         # Batch convert directory

package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DDS magic number "DDS "
	DDSMagic = 0x20534444

	// DDS header flags
	DDSFlagCaps        = 0x00000001
	DDSFlagHeight      = 0x00000002
	DDSFlagWidth       = 0x00000004
	DDSFlagPitch       = 0x00000008
	DDSFlagPixelFormat = 0x00001000
	DDSFlagMipMapCount = 0x00020000
	DDSFlagLinearSize  = 0x00080000
	DDSFlagDepth       = 0x00800000

	// DDS pixel format flags
	DDPFAlphaPixels = 0x00000001
	DDPFAlpha       = 0x00000002
	DDPFFourCC      = 0x00000004
	DDPFRGB         = 0x00000040
	DDPFYUV         = 0x00000200
	DDPFLuminance   = 0x00020000

	// DXGI formats (DX10 extension)
	DXGIFormatUnknown           = 0
	DXGIFormatBC1Unorm          = 71 // BC1/DXT1
	DXGIFormatBC1UnormSRGB      = 72
	DXGIFormatBC3Unorm          = 77 // BC3/DXT5
	DXGIFormatBC3UnormSRGB      = 78
	DXGIFormatBC5Unorm          = 83 // Normal maps
	DXGIFormatBC5SNorm          = 84
	DXGIFormatBC6HUF16          = 95 // HDR
	DXGIFormatBC6HSF16          = 96
	DXGIFormatBC7Unorm          = 98 // High quality
	DXGIFormatBC7UnormSRGB      = 99
	DXGIFormatR8Unorm           = 61 // Grayscale
	DXGIFormatR11G11B10Float    = 26 // Packed Float
	DXGIFormatR8G8B8A8Unorm     = 28 // Uncompressed RGBA
	DXGIFormatR8G8B8A8UnormSRGB = 29
	DXGIFormatB8G8R8A8UnormSRGB = 91 // BGRA sRGB
	DXGIFormatB8G8R8A8Typeless  = 87 // BGRA Typeless
)

// DDSHeader represents the main DDS file header (124 bytes)
type DDSHeader struct {
	Magic             uint32 // Must be "DDS "
	Size              uint32 // Size of structure (124)
	Flags             uint32 // Flags to indicate valid fields
	Height            uint32 // Height of surface
	Width             uint32 // Width of surface
	PitchOrLinearSize uint32 // Bytes per scan line or total bytes
	Depth             uint32 // Depth of volume texture
	MipMapCount       uint32 // Number of mipmap levels
	Reserved1         [11]uint32
	PixelFormat       DDSPixelFormat
	Caps              uint32
	Caps2             uint32
	Caps3             uint32
	Caps4             uint32
	Reserved2         uint32
}

// DDSPixelFormat describes the pixel format (32 bytes)
type DDSPixelFormat struct {
	Size        uint32  // Size of structure (32)
	Flags       uint32  // Pixel format flags
	FourCC      [4]byte // FourCC code (e.g., "DXT1")
	RGBBitCount uint32
	RBitMask    uint32
	GBitMask    uint32
	BBitMask    uint32
	ABitMask    uint32
}

// DDSDX10Header is the extended header for DX10+ formats (20 bytes)
type DDSDX10Header struct {
	DXGIFormat        uint32
	ResourceDimension uint32
	MiscFlag          uint32
	ArraySize         uint32
	MiscFlags2        uint32
}

// TextureInfo contains decoded texture information
type TextureInfo struct {
	Width         uint32
	Height        uint32
	MipLevels     uint32
	Format        uint32 // DXGI format
	FormatName    string
	Compression   string
	DataOffset    uint32 // Offset to pixel data
	DataSize      uint32 // Size of pixel data
	BytesPerPixel int
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "decode":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Usage: texconv decode input.dds output.png\n")
			os.Exit(1)
		}
		if err := decodeDDS(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Decoded %s → %s\n", os.Args[2], os.Args[3])

	case "encode":
		if len(os.Args) != 4 {
			fmt.Fprintf(os.Stderr, "Usage: texconv encode input.png output.dds\n")
			os.Exit(1)
		}
		if err := encodeDDS(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Encoded %s → %s\n", os.Args[2], os.Args[3])

	case "info":
		if len(os.Args) != 3 {
			fmt.Fprintf(os.Stderr, "Usage: texconv info input.dds\n")
			os.Exit(1)
		}
		if err := showInfo(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "batch":
		if len(os.Args) != 5 {
			fmt.Fprintf(os.Stderr, "Usage: texconv batch decode|encode input_dir output_dir\n")
			os.Exit(1)
		}
		if err := batchConvert(os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("texconv - Lossless DDS texture converter for EchoVR")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  texconv decode <input.dds> <output.png>    # DDS → PNG")
	fmt.Println("  texconv encode <input.png> <output.dds>    # PNG → DDS")
	fmt.Println("  texconv info <input.dds>                   # Show info")
	fmt.Println("  texconv batch <decode|encode> <dir> <out>  # Batch convert")
	fmt.Println()
	fmt.Println("Supported formats:")
	fmt.Println("  BC1 (DXT1)  - RGB + 1-bit alpha")
	fmt.Println("  BC3 (DXT5)  - RGBA")
	fmt.Println("  BC5         - Normal maps")
	fmt.Println("  BC6H        - HDR")
	fmt.Println("  BC7         - High quality RGBA")
}

// decodeDDS reads a DDS file and converts it to PNG
func decodeDDS(inputPath, outputPath string) error {
	// Read DDS file
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Parse header
	info, err := parseDDSHeader(f)
	if err != nil {
		return fmt.Errorf("parse header: %w", err)
	}

	// Read compressed data
	f.Seek(int64(info.DataOffset), io.SeekStart)
	compressedData := make([]byte, info.DataSize)
	if _, err := io.ReadFull(f, compressedData); err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	// Decompress to RGBA
	img, err := decompressBC(compressedData, info)
	if err != nil {
		return fmt.Errorf("decompress: %w", err)
	}

	// Write PNG
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	if err := png.Encode(outFile, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}

	return nil
}

// encodeDDS reads a PNG and converts it to DDS
func encodeDDS(inputPath, outputPath string) error {
	// Read PNG file
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	// Auto-detect best BC format
	format := DetectBCFormat(img)
	var dxgiFormat uint32
	switch format {
	case BC1:
		dxgiFormat = DXGIFormatBC1Unorm
	case BC3:
		dxgiFormat = DXGIFormatBC3Unorm
	default:
		return fmt.Errorf("unsupported format: %d", format)
	}

	// Generate mipmaps
	mipmaps := GenerateMipmaps(img)

	// Compress all mip levels
	var compressedData []byte
	for _, mip := range mipmaps {
		compressed, err := CompressBC(mip, format)
		if err != nil {
			return fmt.Errorf("compress mip: %w", err)
		}
		compressedData = append(compressedData, compressed...)
	}

	// Write DDS file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outFile.Close()

	bounds := img.Bounds()
	width := uint32(bounds.Dx())
	height := uint32(bounds.Dy())
	mipCount := uint32(len(mipmaps))

	if err := writeDDSFile(outFile, width, height, mipCount, dxgiFormat, compressedData); err != nil {
		return fmt.Errorf("write dds: %w", err)
	}

	return nil
}

// showInfo displays information about a DDS file
func showInfo(inputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	info, err := parseDDSHeader(f)
	if err != nil {
		return fmt.Errorf("parse header: %w", err)
	}

	fmt.Printf("File: %s\n", inputPath)
	fmt.Printf("Dimensions: %dx%d\n", info.Width, info.Height)
	fmt.Printf("Mip levels: %d\n", info.MipLevels)
	fmt.Printf("Format: %s (DXGI %d)\n", info.FormatName, info.Format)
	fmt.Printf("Compression: %s\n", info.Compression)
	fmt.Printf("Data offset: 0x%x\n", info.DataOffset)
	fmt.Printf("Data size: %d bytes (%.2f KB)\n", info.DataSize, float64(info.DataSize)/1024)
	fmt.Printf("Bytes per pixel: %d\n", info.BytesPerPixel)

	return nil
}

// batchConvert processes a directory of files
func batchConvert(mode, inputDir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	var ext string
	if mode == "decode" {
		ext = ".dds"
	} else {
		ext = ".png"
	}

	count := 0
	errors := 0

	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ext) {
			return nil
		}

		relPath, _ := filepath.Rel(inputDir, path)
		outPath := filepath.Join(outputDir, relPath)

		if mode == "decode" {
			outPath = strings.TrimSuffix(outPath, ext) + ".png"
		} else {
			outPath = strings.TrimSuffix(outPath, ext) + ".dds"
		}

		// Create output directory
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", filepath.Dir(outPath), err)
			errors++
			return nil
		}

		// Convert
		var convErr error
		if mode == "decode" {
			convErr = decodeDDS(path, outPath)
		} else {
			convErr = encodeDDS(path, outPath)
		}

		if convErr != nil {
			fmt.Fprintf(os.Stderr, "convert %s: %v\n", path, convErr)
			errors++
		} else {
			count++
			if count%100 == 0 {
				fmt.Printf("Processed %d files...\n", count)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	fmt.Printf("\nCompleted: %d files converted, %d errors\n", count, errors)
	return nil
}

// parseDDSHeader reads and parses the DDS header
func parseDDSHeader(r io.ReadSeeker) (*TextureInfo, error) {
	var header DDSHeader
	if err := binary.Read(r, binary.LittleEndian, &header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	if header.Magic != DDSMagic {
		return nil, fmt.Errorf("invalid DDS magic: 0x%08x", header.Magic)
	}

	info := &TextureInfo{
		Width:     header.Width,
		Height:    header.Height,
		MipLevels: header.MipMapCount,
	}

	if info.MipLevels == 0 {
		info.MipLevels = 1
	}

	// Check for DX10 extended header
	fourCC := string(header.PixelFormat.FourCC[:])
	if fourCC == "DX10" {
		var dx10 DDSDX10Header
		if err := binary.Read(r, binary.LittleEndian, &dx10); err != nil {
			return nil, fmt.Errorf("read DX10 header: %w", err)
		}
		info.Format = dx10.DXGIFormat
		info.DataOffset = 128 + 20 // DDS header + DX10 header
	} else {
		// Legacy format
		info.DataOffset = 128
		// Map FourCC to DXGI format
		switch fourCC {
		case "DXT1":
			info.Format = DXGIFormatBC1Unorm
		case "DXT3":
			info.Format = 74 // BC2
		case "DXT5":
			info.Format = DXGIFormatBC3Unorm
		case "ATI1", "BC4U":
			info.Format = 80 // BC4
		case "ATI2", "BC5U":
			info.Format = DXGIFormatBC5Unorm
		default:
			return nil, fmt.Errorf("unsupported fourCC: %s", fourCC)
		}
	}

	// Set format name and compression
	switch info.Format {
	case DXGIFormatBC1Unorm, DXGIFormatBC1UnormSRGB:
		info.FormatName = "BC1 (DXT1)"
		info.Compression = "BC1"
		info.BytesPerPixel = 0 // 4 bits per pixel (0.5 bytes)
	case DXGIFormatBC3Unorm, DXGIFormatBC3UnormSRGB:
		info.FormatName = "BC3 (DXT5)"
		info.Compression = "BC3"
		info.BytesPerPixel = 1
	case DXGIFormatBC5Unorm, DXGIFormatBC5SNorm:
		info.FormatName = "BC5"
		info.Compression = "BC5"
		info.BytesPerPixel = 1
	case DXGIFormatBC6HUF16, DXGIFormatBC6HSF16:
		info.FormatName = "BC6H (HDR)"
		info.Compression = "BC6H"
		info.BytesPerPixel = 1
	case DXGIFormatBC7Unorm, DXGIFormatBC7UnormSRGB:
		info.FormatName = "BC7"
		info.Compression = "BC7"
		info.BytesPerPixel = 1
	case DXGIFormatR8Unorm:
		info.FormatName = "R8_UNORM"
		info.Compression = "None"
		info.BytesPerPixel = 1
	case DXGIFormatR11G11B10Float:
		info.FormatName = "R11G11B10_FLOAT"
		info.Compression = "None"
		info.BytesPerPixel = 4
	case DXGIFormatR8G8B8A8Unorm, DXGIFormatR8G8B8A8UnormSRGB:
		info.FormatName = "RGBA8"
		info.Compression = "None"
		info.BytesPerPixel = 4
	case DXGIFormatB8G8R8A8UnormSRGB:
		info.FormatName = "BGRA8"
		info.Compression = "None"
		info.BytesPerPixel = 4
	case DXGIFormatB8G8R8A8Typeless:
		info.FormatName = "BGRA8_TYPELESS"
		info.Compression = "None"
		info.BytesPerPixel = 4
	default:
		return nil, fmt.Errorf("unsupported DXGI format: %d", info.Format)
	}

	// Calculate data size
	info.DataSize = calculateTextureSize(info.Width, info.Height, info.MipLevels, info.Format)

	return info, nil
}

// calculateTextureSize computes total texture data size including mipmaps
func calculateTextureSize(width, height, mipLevels, format uint32) uint32 {
	var totalSize uint32
	for i := uint32(0); i < mipLevels; i++ {
		w := max(1, width>>i)
		h := max(1, height>>i)
		totalSize += calculateMipSize(w, h, format)
	}
	return totalSize
}

func calculateMipSize(width, height, format uint32) uint32 {
	blockW := (width + 3) / 4
	blockH := (height + 3) / 4

	switch format {
	case DXGIFormatBC1Unorm, DXGIFormatBC1UnormSRGB:
		return blockW * blockH * 8 // 8 bytes per block
	case DXGIFormatBC3Unorm, DXGIFormatBC3UnormSRGB,
		DXGIFormatBC5Unorm, DXGIFormatBC5SNorm,
		DXGIFormatBC6HUF16, DXGIFormatBC6HSF16,
		DXGIFormatBC7Unorm, DXGIFormatBC7UnormSRGB:
		return blockW * blockH * 16 // 16 bytes per block
	case DXGIFormatR8Unorm:
		return width * height
	case DXGIFormatR11G11B10Float:
		return width * height * 4
	case DXGIFormatR8G8B8A8Unorm, DXGIFormatR8G8B8A8UnormSRGB:
		return width * height * 4
	case DXGIFormatB8G8R8A8UnormSRGB:
		return width * height * 4
	case DXGIFormatB8G8R8A8Typeless:
		return width * height * 4
	default:
		return width * height * 4 // Fallback: uncompressed RGBA
	}
}

// decompressBC decompresses BC-compressed data to RGBA
func decompressBC(data []byte, info *TextureInfo) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, int(info.Width), int(info.Height)))

	isSRGB := info.Format == DXGIFormatBC1UnormSRGB ||
		info.Format == DXGIFormatBC3UnormSRGB ||
		info.Format == DXGIFormatBC7UnormSRGB

	switch info.Format {
	case DXGIFormatBC1Unorm, DXGIFormatBC1UnormSRGB:
		return decompressBC1(data, int(info.Width), int(info.Height), isSRGB)
	case DXGIFormatBC3Unorm, DXGIFormatBC3UnormSRGB:
		return decompressBC3(data, int(info.Width), int(info.Height), isSRGB)
	case DXGIFormatBC5Unorm, DXGIFormatBC5SNorm:
		return decompressBC5(data, int(info.Width), int(info.Height))
	case DXGIFormatR8Unorm:
		return decompressR8(data, int(info.Width), int(info.Height))
	case DXGIFormatR11G11B10Float:
		return decompressR11G11B10Float(data, int(info.Width), int(info.Height))
	case DXGIFormatR8G8B8A8Unorm, DXGIFormatR8G8B8A8UnormSRGB:
		return decompressRGBA(data, int(info.Width), int(info.Height))
	case DXGIFormatB8G8R8A8UnormSRGB:
		return decompressBGRA(data, int(info.Width), int(info.Height))
	case DXGIFormatB8G8R8A8Typeless:
		return decompressBGRA(data, int(info.Width), int(info.Height))
	default:
		return nil, fmt.Errorf("decompression not implemented for format: %s", info.FormatName)
	}

	return nrgba, nil
}

// decompressBC1 decompresses BC1/DXT1 to RGBA
func decompressBC1(data []byte, width, height int, isSRGB bool) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))

	blockW := (width + 3) / 4
	blockH := (height + 3) / 4

	offset := 0
	for by := 0; by < blockH; by++ {
		for bx := 0; bx < blockW; bx++ {
			if offset+8 > len(data) {
				return nil, fmt.Errorf("data truncated")
			}

			// Read color endpoints
			c0 := uint16(data[offset]) | uint16(data[offset+1])<<8
			c1 := uint16(data[offset+2]) | uint16(data[offset+3])<<8
			offset += 4

			// Decode RGB565
			r0_5 := (c0 >> 11) & 0x1F
			g0_6 := (c0 >> 5) & 0x3F
			b0_5 := c0 & 0x1F
			r0_8 := uint8((r0_5 << 3) | (r0_5 >> 2))
			g0_8 := uint8((g0_6 << 2) | (g0_6 >> 4))
			b0_8 := uint8((b0_5 << 3) | (b0_5 >> 2))

			r1_5 := (c1 >> 11) & 0x1F
			g1_6 := (c1 >> 5) & 0x3F
			b1_5 := c1 & 0x1F
			r1_8 := uint8((r1_5 << 3) | (r1_5 >> 2))
			g1_8 := uint8((g1_6 << 2) | (g1_6 >> 4))
			b1_8 := uint8((b1_5 << 3) | (b1_5 >> 2))

			// Color palette
			var colors [4][4]uint8

			if isSRGB {
				lr0 := srgbToLinear(r0_8)
				lg0 := srgbToLinear(g0_8)
				lb0 := srgbToLinear(b0_8)
				lr1 := srgbToLinear(r1_8)
				lg1 := srgbToLinear(g1_8)
				lb1 := srgbToLinear(b1_8)

				var linearColors [4][3]float32
				linearColors[0] = [3]float32{lr0, lg0, lb0}
				linearColors[1] = [3]float32{lr1, lg1, lb1}

				if c0 > c1 {
					linearColors[2] = [3]float32{(2*lr0 + lr1) / 3, (2*lg0 + lg1) / 3, (2*lb0 + lb1) / 3}
					linearColors[3] = [3]float32{(lr0 + 2*lr1) / 3, (lg0 + 2*lg1) / 3, (lb0 + 2*lb1) / 3}
				} else {
					linearColors[2] = [3]float32{(lr0 + lr1) / 2, (lg0 + lg1) / 2, (lb0 + lb1) / 2}
					linearColors[3] = [3]float32{0, 0, 0}
				}

				for i := 0; i < 4; i++ {
					colors[i][0] = linearToSrgb(linearColors[i][0])
					colors[i][1] = linearToSrgb(linearColors[i][1])
					colors[i][2] = linearToSrgb(linearColors[i][2])
					colors[i][3] = 255
				}
				if c0 <= c1 {
					colors[3][3] = 0
				}
			} else {
				colors[0] = [4]uint8{r0_8, g0_8, b0_8, 255}
				colors[1] = [4]uint8{r1_8, g1_8, b1_8, 255}

				if c0 > c1 {
					colors[2] = [4]uint8{
						(2*r0_8 + r1_8) / 3,
						(2*g0_8 + g1_8) / 3,
						(2*b0_8 + b1_8) / 3,
						255,
					}
					colors[3] = [4]uint8{
						(r0_8 + 2*r1_8) / 3,
						(g0_8 + 2*g1_8) / 3,
						(b0_8 + 2*b1_8) / 3,
						255,
					}
				} else {
					colors[2] = [4]uint8{(r0_8 + r1_8) / 2, (g0_8 + g1_8) / 2, (b0_8 + b1_8) / 2, 255}
					colors[3] = [4]uint8{0, 0, 0, 0} // Transparent
				}
			}

			// Read index bits
			indices := uint32(data[offset]) | uint32(data[offset+1])<<8 |
				uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
			offset += 4

			// Decode 4x4 block
			for py := 0; py < 4; py++ {
				for px := 0; px < 4; px++ {
					x := bx*4 + px
					y := by*4 + py
					if x >= width || y >= height {
						continue
					}

					idx := (indices >> (2 * (py*4 + px))) & 3
					color := colors[idx]

					offset := nrgba.PixOffset(x, y)
					nrgba.Pix[offset+0] = color[0]
					nrgba.Pix[offset+1] = color[1]
					nrgba.Pix[offset+2] = color[2]
					nrgba.Pix[offset+3] = color[3]
				}
			}
		}
	}

	return nrgba, nil
}

// decompressBC3 decompresses BC3/DXT5 to RGBA
func decompressBC3(data []byte, width, height int, isSRGB bool) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))

	blockW := (width + 3) / 4
	blockH := (height + 3) / 4

	offset := 0
	for by := 0; by < blockH; by++ {
		for bx := 0; bx < blockW; bx++ {
			if offset+16 > len(data) {
				return nil, fmt.Errorf("data truncated")
			}

			// Decode alpha block (8 bytes)
			alpha0 := data[offset]
			alpha1 := data[offset+1]
			alphaIndices := uint64(0)
			for i := 0; i < 6; i++ {
				alphaIndices |= uint64(data[offset+2+i]) << (i * 8)
			}

			// Alpha palette
			var alphas [8]uint8
			alphas[0] = alpha0
			alphas[1] = alpha1
			if alpha0 > alpha1 {
				for i := 2; i < 8; i++ {
					alphas[i] = uint8((int(alpha0)*(8-i) + int(alpha1)*(i-1)) / 7)
				}
			} else {
				for i := 2; i < 6; i++ {
					alphas[i] = uint8((int(alpha0)*(6-i) + int(alpha1)*(i-1)) / 5)
				}
				alphas[6] = 0
				alphas[7] = 255
			}
			offset += 8

			// Decode color block (8 bytes) - same as BC1
			c0 := uint16(data[offset]) | uint16(data[offset+1])<<8
			c1 := uint16(data[offset+2]) | uint16(data[offset+3])<<8
			offset += 4

			r0_5 := (c0 >> 11) & 0x1F
			g0_6 := (c0 >> 5) & 0x3F
			b0_5 := c0 & 0x1F
			r0_8 := uint8((r0_5 << 3) | (r0_5 >> 2))
			g0_8 := uint8((g0_6 << 2) | (g0_6 >> 4))
			b0_8 := uint8((b0_5 << 3) | (b0_5 >> 2))

			r1_5 := (c1 >> 11) & 0x1F
			g1_6 := (c1 >> 5) & 0x3F
			b1_5 := c1 & 0x1F
			r1_8 := uint8((r1_5 << 3) | (r1_5 >> 2))
			g1_8 := uint8((g1_6 << 2) | (g1_6 >> 4))
			b1_8 := uint8((b1_5 << 3) | (b1_5 >> 2))

			var colors [4][3]uint8
			if isSRGB {
				lr0 := srgbToLinear(r0_8)
				lg0 := srgbToLinear(g0_8)
				lb0 := srgbToLinear(b0_8)
				lr1 := srgbToLinear(r1_8)
				lg1 := srgbToLinear(g1_8)
				lb1 := srgbToLinear(b1_8)

				var linearColors [4][3]float32
				linearColors[0] = [3]float32{lr0, lg0, lb0}
				linearColors[1] = [3]float32{lr1, lg1, lb1}
				linearColors[2] = [3]float32{(2*lr0 + lr1) / 3, (2*lg0 + lg1) / 3, (2*lb0 + lb1) / 3}
				linearColors[3] = [3]float32{(lr0 + 2*lr1) / 3, (lg0 + 2*lg1) / 3, (lb0 + 2*lb1) / 3}

				for i := 0; i < 4; i++ {
					colors[i][0] = linearToSrgb(linearColors[i][0])
					colors[i][1] = linearToSrgb(linearColors[i][1])
					colors[i][2] = linearToSrgb(linearColors[i][2])
				}
			} else {
				colors[0] = [3]uint8{r0_8, g0_8, b0_8}
				colors[1] = [3]uint8{r1_8, g1_8, b1_8}
				colors[2] = [3]uint8{(2*r0_8 + r1_8) / 3, (2*g0_8 + g1_8) / 3, (2*b0_8 + b1_8) / 3}
				colors[3] = [3]uint8{(r0_8 + 2*r1_8) / 3, (g0_8 + 2*g1_8) / 3, (b0_8 + 2*b1_8) / 3}
			}

			colorIndices := uint32(data[offset]) | uint32(data[offset+1])<<8 |
				uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
			offset += 4

			// Decode 4x4 block
			for py := 0; py < 4; py++ {
				for px := 0; px < 4; px++ {
					x := bx*4 + px
					y := by*4 + py
					if x >= width || y >= height {
						continue
					}

					pidx := py*4 + px
					colorIdx := (colorIndices >> (2 * pidx)) & 3
					alphaIdx := (alphaIndices >> (3 * pidx)) & 7

					color := colors[colorIdx]
					alpha := alphas[alphaIdx]

					pixOffset := nrgba.PixOffset(x, y)
					nrgba.Pix[pixOffset+0] = color[0]
					nrgba.Pix[pixOffset+1] = color[1]
					nrgba.Pix[pixOffset+2] = color[2]
					nrgba.Pix[pixOffset+3] = alpha
				}
			}
		}
	}

	return nrgba, nil
}

// decompressBC5 decompresses BC5 (normal maps) to RGBA
func decompressBC5(data []byte, width, height int) (*image.NRGBA, error) {
	// BC5 stores two channels (RG for normal maps)
	// We'll decode them and reconstruct Z = sqrt(1 - X^2 - Y^2)
	return nil, fmt.Errorf("BC5 decompression not yet implemented")
}

// decompressR8 decompresses R8_UNORM (grayscale) to RGBA
func decompressR8(data []byte, width, height int) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(data) < width*height {
		return nil, fmt.Errorf("data truncated")
	}

	offset := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			v := data[offset]
			offset++
			pixOffset := nrgba.PixOffset(x, y)
			nrgba.Pix[pixOffset+0] = v
			nrgba.Pix[pixOffset+1] = v
			nrgba.Pix[pixOffset+2] = v
			nrgba.Pix[pixOffset+3] = 255
		}
	}
	return nrgba, nil
}

// decompressRGBA decompresses uncompressed RGBA to RGBA
func decompressRGBA(data []byte, width, height int) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(data) < width*height*4 {
		return nil, fmt.Errorf("data truncated")
	}
	copy(nrgba.Pix, data[:width*height*4])
	return nrgba, nil
}

// decompressBGRA decompresses uncompressed BGRA to RGBA
func decompressBGRA(data []byte, width, height int) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(data) < width*height*4 {
		return nil, fmt.Errorf("data truncated")
	}

	count := width * height
	for i := 0; i < count; i++ {
		offset := i * 4
		b := data[offset]
		g := data[offset+1]
		r := data[offset+2]
		a := data[offset+3]

		nrgba.Pix[offset] = r
		nrgba.Pix[offset+1] = g
		nrgba.Pix[offset+2] = b
		nrgba.Pix[offset+3] = a
	}
	return nrgba, nil
}

// decompressR11G11B10Float decompresses packed float format to RGBA
func decompressR11G11B10Float(data []byte, width, height int) (*image.NRGBA, error) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
	if len(data) < width*height*4 {
		return nil, fmt.Errorf("data truncated")
	}

	offset := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			packed := uint32(data[offset]) | uint32(data[offset+1])<<8 | uint32(data[offset+2])<<16 | uint32(data[offset+3])<<24
			offset += 4

			r := f11ToF32(packed & 0x7FF)
			g := f11ToF32((packed >> 11) & 0x7FF)
			b := f10ToF32((packed >> 22) & 0x3FF)

			// Clamp to 0-255
			r8 := uint8(math.Min(255, math.Max(0, float64(r)*255)))
			g8 := uint8(math.Min(255, math.Max(0, float64(g)*255)))
			b8 := uint8(math.Min(255, math.Max(0, float64(b)*255)))

			pixOffset := nrgba.PixOffset(x, y)
			nrgba.Pix[pixOffset+0] = r8
			nrgba.Pix[pixOffset+1] = g8
			nrgba.Pix[pixOffset+2] = b8
			nrgba.Pix[pixOffset+3] = 255
		}
	}
	return nrgba, nil
}

func f11ToF32(u uint32) float32 {
	exponent := (u >> 6) & 0x1F
	mantissa := u & 0x3F
	if exponent == 0 {
		if mantissa == 0 {
			return 0.0
		}
		return float32(mantissa) / 64.0 * (1.0 / 16384.0)
	} else if exponent == 31 {
		return 65504.0
	}
	return float32(math.Pow(2, float64(exponent)-15)) * (1.0 + float32(mantissa)/64.0)
}

func f10ToF32(u uint32) float32 {
	exponent := (u >> 5) & 0x1F
	mantissa := u & 0x1F
	if exponent == 0 {
		if mantissa == 0 {
			return 0.0
		}
		return float32(mantissa) / 32.0 * (1.0 / 16384.0)
	} else if exponent == 31 {
		return 65504.0
	}
	return float32(math.Pow(2, float64(exponent)-15)) * (1.0 + float32(mantissa)/32.0)
}

// srgbToLinear converts an sRGB byte value to a linear float32 value.
func srgbToLinear(c uint8) float32 {
	v := float32(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return float32(math.Pow(float64((v+0.055)/1.055), 2.4))
}

// linearToSrgb converts a linear float32 value to an sRGB byte value.
func linearToSrgb(v float32) uint8 {
	if v <= 0.0031308 {
		return uint8(math.Min(255, math.Max(0, float64(v)*12.92*255.0)))
	}
	srgb := 1.055*math.Pow(float64(v), 1.0/2.4) - 0.055
	return uint8(math.Min(255, math.Max(0, srgb*255.0)))
}

// writeDDSFile writes a complete DDS file with DX10 header
func writeDDSFile(w io.Writer, width, height, mipCount, dxgiFormat uint32, compressedData []byte) error {
	// Calculate pitch/linear size
	var bytesPerBlock uint32
	switch dxgiFormat {
	case DXGIFormatBC1Unorm, DXGIFormatBC1UnormSRGB:
		bytesPerBlock = 8
	case DXGIFormatBC3Unorm, DXGIFormatBC3UnormSRGB, DXGIFormatBC5Unorm, DXGIFormatBC5SNorm:
		bytesPerBlock = 16
	default:
		return fmt.Errorf("unsupported DXGI format: %d", dxgiFormat)
	}

	blocksWide := (width + 3) / 4
	blocksHigh := (height + 3) / 4
	linearSize := blocksWide * blocksHigh * bytesPerBlock

	// Build DDS header
	header := DDSHeader{
		Magic:             DDSMagic,
		Size:              124,
		Flags:             DDSFlagCaps | DDSFlagHeight | DDSFlagWidth | DDSFlagPixelFormat | DDSFlagMipMapCount | DDSFlagLinearSize,
		Height:            height,
		Width:             width,
		PitchOrLinearSize: linearSize,
		Depth:             0,
		MipMapCount:       mipCount,
		PixelFormat: DDSPixelFormat{
			Size:   32,
			Flags:  DDPFFourCC,
			FourCC: [4]byte{'D', 'X', '1', '0'}, // Use DX10 extension
		},
		Caps:  0x1000 | 0x400000, // DDSCAPS_TEXTURE | DDSCAPS_MIPMAP
		Caps2: 0,
	}

	// Build DX10 header
	dx10 := DDSDX10Header{
		DXGIFormat:        dxgiFormat,
		ResourceDimension: 3, // D3D10_RESOURCE_DIMENSION_TEXTURE2D
		MiscFlag:          0,
		ArraySize:         1,
		MiscFlags2:        0,
	}

	// Write headers
	if err := binary.Write(w, binary.LittleEndian, &header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if err := binary.Write(w, binary.LittleEndian, &dx10); err != nil {
		return fmt.Errorf("write dx10 header: %w", err)
	}

	// Write compressed data
	if _, err := w.Write(compressedData); err != nil {
		return fmt.Errorf("write compressed data: %w", err)
	}

	return nil
}

func max(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}
