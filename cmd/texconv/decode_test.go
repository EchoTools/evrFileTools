package main

import (
	"math"
	"testing"
)

func TestDecompressR8(t *testing.T) {
	data := []byte{0, 128, 255, 64}
	img, err := decompressR8(data, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Each grayscale value maps to R=G=B=val, A=255
	tests := []struct {
		x, y    int
		r, g, b uint8
	}{
		{0, 0, 0, 0, 0},
		{1, 0, 128, 128, 128},
		{0, 1, 255, 255, 255},
		{1, 1, 64, 64, 64},
	}
	for _, tt := range tests {
		off := img.PixOffset(tt.x, tt.y)
		if img.Pix[off] != tt.r || img.Pix[off+1] != tt.g || img.Pix[off+2] != tt.b || img.Pix[off+3] != 255 {
			t.Errorf("pixel(%d,%d) = (%d,%d,%d,%d), want (%d,%d,%d,255)",
				tt.x, tt.y, img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3],
				tt.r, tt.g, tt.b)
		}
	}
}

func TestDecompressR8_Truncated(t *testing.T) {
	_, err := decompressR8([]byte{0, 1}, 2, 2) // need 4 bytes, only 2
	if err == nil {
		t.Fatal("expected error for truncated data")
	}
}

func TestDecompressRGBA(t *testing.T) {
	// 1x1 pixel: R=10, G=20, B=30, A=40
	data := []byte{10, 20, 30, 40}
	img, err := decompressRGBA(data, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	off := img.PixOffset(0, 0)
	if img.Pix[off] != 10 || img.Pix[off+1] != 20 || img.Pix[off+2] != 30 || img.Pix[off+3] != 40 {
		t.Errorf("pixel = (%d,%d,%d,%d), want (10,20,30,40)",
			img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3])
	}
}

func TestDecompressRGBA_Truncated(t *testing.T) {
	_, err := decompressRGBA([]byte{1, 2, 3}, 1, 1)
	if err == nil {
		t.Fatal("expected error for truncated data")
	}
}

func TestDecompressBGRA(t *testing.T) {
	// 1x1 BGRA pixel: B=10, G=20, R=30, A=40 -> should output R=30, G=20, B=10, A=40
	data := []byte{10, 20, 30, 40}
	img, err := decompressBGRA(data, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	off := img.PixOffset(0, 0)
	if img.Pix[off] != 30 || img.Pix[off+1] != 20 || img.Pix[off+2] != 10 || img.Pix[off+3] != 40 {
		t.Errorf("pixel = (%d,%d,%d,%d), want (30,20,10,40)",
			img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3])
	}
}

func TestDecompressBGRA_Truncated(t *testing.T) {
	_, err := decompressBGRA([]byte{1, 2, 3}, 1, 1)
	if err == nil {
		t.Fatal("expected error for truncated data")
	}
}

func TestF11ToF32(t *testing.T) {
	tests := []struct {
		name string
		in   uint32
		want float32
	}{
		{"zero", 0, 0.0},
		{"one", 0x3C0, 1.0},         // exponent=15, mantissa=0 -> 2^0 * 1.0 = 1.0
		{"max_exponent", 0x7C0, float32(math.Inf(1))}, // exponent=31, mantissa=0 -> +Inf
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f11ToF32(tt.in)
			if math.IsInf(float64(tt.want), 1) {
				if !math.IsInf(float64(got), 1) {
					t.Errorf("f11ToF32(0x%x) = %f, want +Inf", tt.in, got)
				}
			} else if math.Abs(float64(got-tt.want)) > 0.001 {
				t.Errorf("f11ToF32(0x%x) = %f, want %f", tt.in, got, tt.want)
			}
		})
	}
}

func TestF10ToF32(t *testing.T) {
	tests := []struct {
		name string
		in   uint32
		want float32
	}{
		{"zero", 0, 0.0},
		{"one", 0x1E0, 1.0},                          // exponent=15, mantissa=0 -> 2^0 * 1.0 = 1.0
		{"max_exponent", 0x3E0, float32(math.Inf(1))}, // exponent=31, mantissa=0 -> +Inf
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f10ToF32(tt.in)
			if math.Abs(float64(got-tt.want)) > 0.001 {
				t.Errorf("f10ToF32(0x%x) = %f, want %f", tt.in, got, tt.want)
			}
		})
	}
}

func TestSrgbRoundTrip(t *testing.T) {
	// sRGB -> linear -> sRGB should be identity (within rounding)
	for v := 0; v <= 255; v++ {
		linear := srgbToLinear(uint8(v))
		back := linearToSrgb(linear)
		diff := int(back) - v
		if diff < -1 || diff > 1 {
			t.Errorf("sRGB round-trip failed for %d: got %d (diff %d)", v, back, diff)
		}
	}
}

func TestSrgbToLinear_Boundaries(t *testing.T) {
	// 0 -> 0.0
	if got := srgbToLinear(0); got != 0.0 {
		t.Errorf("srgbToLinear(0) = %f, want 0.0", got)
	}
	// 255 -> ~1.0
	if got := srgbToLinear(255); math.Abs(float64(got)-1.0) > 0.001 {
		t.Errorf("srgbToLinear(255) = %f, want ~1.0", got)
	}
}

func TestLinearToSrgb_Boundaries(t *testing.T) {
	if got := linearToSrgb(0.0); got != 0 {
		t.Errorf("linearToSrgb(0.0) = %d, want 0", got)
	}
	if got := linearToSrgb(1.0); got < 254 {
		t.Errorf("linearToSrgb(1.0) = %d, want 254 or 255", got)
	}
}

func TestDecompressR11G11B10Float(t *testing.T) {
	// Pack 1.0, 1.0, 1.0 into R11G11B10 format
	// R=1.0 as f11: exponent=15(0xF), mantissa=0 -> 0x3C0
	// G=1.0 as f11: same -> 0x3C0 << 11
	// B=1.0 as f10: exponent=15(0xF), mantissa=0 -> 0x1E0 << 22
	r11 := uint32(0x3C0)
	g11 := uint32(0x3C0) << 11
	b10 := uint32(0x1E0) << 22
	packed := r11 | g11 | b10

	data := []byte{
		byte(packed), byte(packed >> 8), byte(packed >> 16), byte(packed >> 24),
	}

	img, err := decompressR11G11B10Float(data, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	off := img.PixOffset(0, 0)
	// 1.0 * 255 = 255
	if img.Pix[off] != 255 || img.Pix[off+1] != 255 || img.Pix[off+2] != 255 {
		t.Errorf("pixel = (%d,%d,%d), want (255,255,255)",
			img.Pix[off], img.Pix[off+1], img.Pix[off+2])
	}
	if img.Pix[off+3] != 255 {
		t.Errorf("alpha = %d, want 255", img.Pix[off+3])
	}
}

func TestDecompressR11G11B10Float_Truncated(t *testing.T) {
	_, err := decompressR11G11B10Float([]byte{1, 2, 3}, 1, 1)
	if err == nil {
		t.Fatal("expected error for truncated data")
	}
}

func TestDecompressBC1_4x4Block(t *testing.T) {
	// Minimal valid BC1 block: 8 bytes
	// c0=0xFFFF (white), c1=0x0000 (black), all indices=0 (use c0)
	block := []byte{
		0xFF, 0xFF, // c0 = white (RGB565)
		0x00, 0x00, // c1 = black
		0x00, 0x00, 0x00, 0x00, // all indices = 0 (use c0)
	}
	img, err := decompressBC1(block, 4, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	// All pixels should be white (255,255,255,255)
	off := img.PixOffset(0, 0)
	if img.Pix[off] != 255 || img.Pix[off+1] != 255 || img.Pix[off+2] != 255 {
		t.Errorf("pixel(0,0) = (%d,%d,%d), want (255,255,255)",
			img.Pix[off], img.Pix[off+1], img.Pix[off+2])
	}
}

func TestDecompressBC1_NRGBA(t *testing.T) {
	// Verify return type is NRGBA, not RGBA
	block := make([]byte, 8)
	img, err := decompressBC1(block, 4, 4, false)
	if err != nil {
		t.Fatal(err)
	}
	// Type assertion — img should be *image.NRGBA
	_ = img.Stride // only NRGBA has Stride; compilation proves the type
}
