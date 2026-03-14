// encoder.go - BC texture compression using libsquish via CGo

package main

/*
#cgo LDFLAGS: -lsquish -lstdc++
#cgo CXXFLAGS: -std=c++11
#include "squish_wrapper.h"
*/
import "C"
import (
	"fmt"
	"image"
	"image/color"
	"unsafe"
)

// BCFormat represents a block compression format
type BCFormat int

const (
	BC1 BCFormat = iota // DXT1 - RGB + 1-bit alpha, 8 bytes/block
	BC3                 // DXT5 - RGBA, 16 bytes/block
	BC5                 // Two-channel, 16 bytes/block
)

// CompressBC compresses RGBA image data to BC format using libsquish
func CompressBC(img image.Image, format BCFormat) ([]byte, error) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Convert image to RGBA format that libsquish expects
	rgba := imageToRGBA(img)

	// Determine squish flags
	var flags C.int

	switch format {
	case BC1:
		flags = C.SQUISH_DXT1 | C.SQUISH_COLOUR_CLUSTER_FIT // DXT1, high quality
	case BC3:
		flags = C.SQUISH_DXT5 | C.SQUISH_COLOUR_CLUSTER_FIT // DXT5, high quality
	case BC5:
		flags = C.SQUISH_BC5
	default:
		return nil, fmt.Errorf("unsupported BC format: %d", format)
	}

	// Calculate storage requirements
	storageSize := C.squish_get_storage_requirements(C.int(width), C.int(height), flags)
	if storageSize <= 0 {
		return nil, fmt.Errorf("invalid storage size: %d", storageSize)
	}

	// Allocate output buffer
	compressed := make([]byte, storageSize)

	// Compress using libsquish
	C.squish_compress_image(
		(*C.uchar)(unsafe.Pointer(&rgba[0])),
		C.int(width),
		C.int(height),
		unsafe.Pointer(&compressed[0]),
		flags,
	)

	return compressed, nil
}

// DecompressBC decompresses BC-encoded blocks to an RGBA image.
// blocks: raw BC data (no DDS header), width/height: texture dimensions, format: BC format
func DecompressBC(blocks []byte, width, height int, format BCFormat) (*image.NRGBA, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("empty block data")
	}

	rgba := make([]byte, width*height*4)

	var flags C.int
	switch format {
	case BC1:
		flags = C.SQUISH_DXT1
	case BC3:
		flags = C.SQUISH_DXT5
	case BC5:
		flags = C.SQUISH_BC5
	default:
		return nil, fmt.Errorf("unsupported BC format for decompression: %d", format)
	}

	C.squish_decompress_image(
		(*C.uchar)(unsafe.Pointer(&rgba[0])),
		C.int(width),
		C.int(height),
		unsafe.Pointer(&blocks[0]),
		flags,
	)

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	copy(img.Pix, rgba)
	return img, nil
}

// imageToRGBA converts an image.Image to RGBA byte array in the format libsquish expects
// Format: r1,g1,b1,a1, r2,g2,b2,a2, ..., rn,gn,bn,an
func imageToRGBA(img image.Image) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	rgba := make([]byte, width*height*4)
	idx := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// Convert from 16-bit to 8-bit
			rgba[idx+0] = uint8(r >> 8)
			rgba[idx+1] = uint8(g >> 8)
			rgba[idx+2] = uint8(b >> 8)
			rgba[idx+3] = uint8(a >> 8)
			idx += 4
		}
	}

	return rgba
}

// DetectBCFormat analyzes an image and determines the best BC format to use
func DetectBCFormat(img image.Image) BCFormat {
	bounds := img.Bounds()
	hasPartialAlpha := false

	// Sample pixels to check alpha usage
	sampleCount := 0
	maxSamples := 1000 // Sample at most 1000 pixels for performance

	for y := bounds.Min.Y; y < bounds.Max.Y; y += (bounds.Dy() / 10) + 1 {
		for x := bounds.Min.X; x < bounds.Max.X; x += (bounds.Dx() / 10) + 1 {
			if sampleCount >= maxSamples {
				break
			}

			_, _, _, a := img.At(x, y).RGBA()
			alpha8 := uint8(a >> 8)

			if alpha8 < 255 && alpha8 > 0 {
				hasPartialAlpha = true
				break
			}
			sampleCount++
		}
		if hasPartialAlpha {
			break
		}
	}

	// Decision logic:
	// - BC3 (DXT5) if there's partial alpha (smooth gradients)
	// - BC1 (DXT1) if binary alpha or no alpha
	if hasPartialAlpha {
		return BC3
	}
	return BC1
}

// GenerateMipmaps creates a mipmap chain for the given image
func GenerateMipmaps(img image.Image) []image.Image {
	mipmaps := []image.Image{img}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Generate mips until we reach 1x1
	for width > 1 || height > 1 {
		if width > 1 {
			width = width / 2
		}
		if height > 1 {
			height = height / 2
		}

		mip := resizeImage(mipmaps[len(mipmaps)-1], width, height)
		mipmaps = append(mipmaps, mip)
	}

	return mipmaps
}

// resizeImage downsamples an image to the target dimensions using box filtering
func resizeImage(img image.Image, targetWidth, targetHeight int) image.Image {
	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	result := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))

	scaleX := float64(srcWidth) / float64(targetWidth)
	scaleY := float64(srcHeight) / float64(targetHeight)

	for dy := 0; dy < targetHeight; dy++ {
		for dx := 0; dx < targetWidth; dx++ {
			// Box filter: average 2x2 block of source pixels
			sx := int(float64(dx) * scaleX)
			sy := int(float64(dy) * scaleY)

			var rSum, gSum, bSum, aSum uint32
			sampleCount := 0

			for ssy := sy; ssy < sy+int(scaleY)+1 && ssy < srcHeight; ssy++ {
				for ssx := sx; ssx < sx+int(scaleX)+1 && ssx < srcWidth; ssx++ {
					r, g, b, a := img.At(bounds.Min.X+ssx, bounds.Min.Y+ssy).RGBA()
					rSum += r
					gSum += g
					bSum += b
					aSum += a
					sampleCount++
				}
			}

			if sampleCount > 0 {
				result.SetRGBA(dx, dy, color.RGBA{
					R: uint8((rSum / uint32(sampleCount)) >> 8),
					G: uint8((gSum / uint32(sampleCount)) >> 8),
					B: uint8((bSum / uint32(sampleCount)) >> 8),
					A: uint8((aSum / uint32(sampleCount)) >> 8),
				})
			}
		}
	}

	return result
}
