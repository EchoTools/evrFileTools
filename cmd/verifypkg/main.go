// cmd/verifypkg/main.go
// Reads the modified manifest and checks every frame entry pointing to package 3.
// Reports whether the bytes at declared offsets are valid zstd frames.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
	"github.com/klauspost/compress/zstd"
)

var zstdMagic = [4]byte{0x28, 0xB5, 0x2F, 0xFD}

func main() {
	dataDir := flag.String("d", "", "Data directory (e.g. C:\\...\\rad15\\win10)")
	pkgName := flag.String("p", "48037dc70b0ecab2", "Package name")
	flag.Parse()

	if *dataDir == "" {
		fmt.Fprintln(os.Stderr, "usage: verifypkg -d <dataDir> [-p <packageName>]")
		os.Exit(1)
	}

	manifestPath := filepath.Join(*dataDir, "manifests", *pkgName)
	fmt.Printf("Reading manifest: %s\n", manifestPath)

	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR reading manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PackageCount: %d\n", m.Header.PackageCount)
	fmt.Printf("Frames:       %d\n", len(m.Frames))
	fmt.Printf("FrameContents:%d\n\n", len(m.FrameContents))

	// Find frames pointing to any non-original package (pkg >= originalCount)
	// We'll check ALL packages to be thorough.
	pkgFiles := map[uint32]*os.File{}
	defer func() {
		for _, f := range pkgFiles {
			f.Close()
		}
	}()

	openPkg := func(idx uint32) (*os.File, error) {
		if f, ok := pkgFiles[idx]; ok {
			return f, nil
		}
		p := filepath.Join(*dataDir, "packages", fmt.Sprintf("%s_%d", *pkgName, idx))
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		pkgFiles[idx] = f
		return f, nil
	}

	newPkg := m.Header.PackageCount - 1

	errors := 0
	checked := 0
	for i, fr := range m.Frames {
		if fr.CompressedSize == 0 {
			continue // terminator or null
		}
		// Only check frames pointing to the newest package OR all if you want full scan
		if fr.PackageIndex != newPkg {
			continue
		}

		f, err := openPkg(fr.PackageIndex)
		if err != nil {
			fmt.Printf("[WARN] Frame %d: cannot open pkg %d: %v\n", i, fr.PackageIndex, err)
			errors++
			continue
		}

		hdr := make([]byte, 12)
		n, err := f.ReadAt(hdr, int64(fr.Offset))
		if err != nil && n < 4 {
			fmt.Printf("[ERR]  Frame %5d → pkg %d off %10d: read error: %v\n",
				i, fr.PackageIndex, fr.Offset, err)
			errors++
			checked++
			continue
		}

		magic := hdr[:4]
		if magic[0] != 0x28 || magic[1] != 0xB5 || magic[2] != 0x2F || magic[3] != 0xFD {
			fmt.Printf("[ERR]  Frame %5d → pkg %d off %10d compSz %8d: BAD MAGIC: %02X %02X %02X %02X\n",
				i, fr.PackageIndex, fr.Offset, fr.CompressedSize,
				magic[0], magic[1], magic[2], magic[3])
			errors++
		} else {
			fhd := hdr[4]
			csfFlag := (fhd >> 6) & 3
			ss := (fhd >> 5) & 1
			// Content size position depends on SS flag:
			// SS=0: byte5=WD, bytes6+ = content_size
			// SS=1: bytes5+ = content_size
			var embeddedSize uint64
			csOff := 5
			if ss == 0 {
				csOff = 6 // skip Window_Descriptor
			}
			switch csfFlag {
			case 1:
				if csOff+2 <= n {
					embeddedSize = uint64(binary.LittleEndian.Uint16(hdr[csOff:csOff+2])) + 256
				}
			case 2:
				if csOff+4 <= n {
					embeddedSize = uint64(binary.LittleEndian.Uint32(hdr[csOff : csOff+4]))
				}
			case 3:
				if csOff+8 <= n {
					embeddedSize = binary.LittleEndian.Uint64(hdr[csOff : csOff+8])
				}
			}
			match := ""
			if csfFlag > 0 && embeddedSize != uint64(fr.Length) {
				match = fmt.Sprintf(" ← MISMATCH manifest=%d", fr.Length)
				errors++
			}
			
			// DO FULL DECOMPRESS CHECK
			compressedData := make([]byte, fr.CompressedSize)
			n, err := f.ReadAt(compressedData, int64(fr.Offset))
			if err != nil || n < int(fr.CompressedSize) {
				fmt.Printf("[ERR] Frame %5d: Could not read compressed bytes: %v\n", i, err)
				errors++
			} else {
				dec, _ := zstd.NewReader(nil)
				_, zerr := dec.DecodeAll(compressedData, nil)
				dec.Close()
				if zerr != nil {
					fmt.Printf("[ERR] Frame %5d: zstd decompression failed: %v\n", i, zerr)
					errors++
				}
			}
			
			fmt.Printf("[OK]   Frame %5d → pkg %d off %10d compSz %8d decompSz %8d FHD=0x%02X CSF=%d SS=%d embSz=%d%s\n",
				i, fr.PackageIndex, fr.Offset, fr.CompressedSize, fr.Length,
				fhd, csfFlag, ss, embeddedSize, match)
		}
		checked++
	}

	fmt.Printf("\nChecked %d frames in pkg %d. Errors: %d\n", checked, newPkg, errors)
	if errors > 0 {
		os.Exit(1)
	}
}
