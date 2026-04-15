// cmd/findfile/main.go
// Finds a specific file symbol in the manifest and shows which frame/package it's in.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
)

func main() {
	dataDir := flag.String("d", "", "Data directory")
	pkgName := flag.String("p", "48037dc70b0ecab2", "Package name")
	symbol := flag.String("sym", "", "File symbol hex (e.g. e8cc523d0fc9e5fe)")
	flag.Parse()

	if *dataDir == "" || *symbol == "" {
		fmt.Fprintln(os.Stderr, "usage: findfile -d <dataDir> -sym <hex> [-p <pkg>]")
		os.Exit(1)
	}

	symStr := strings.TrimPrefix(*symbol, "0x")
	symVal, err := strconv.ParseUint(symStr, 16, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad symbol: %v\n", err)
		os.Exit(1)
	}
	symSigned := int64(symVal)

	manifestPath := filepath.Join(*dataDir, "manifests", *pkgName)
	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Searching for file symbol 0x%016X in %d FrameContents...\n", symVal, len(m.FrameContents))

	found := false
	for i, fc := range m.FrameContents {
		if fc.FileSymbol == symSigned || fc.TypeSymbol == symSigned {
			found = true
			fmt.Printf("\nFound in FrameContents[%d]:\n", i)
			fmt.Printf("  FileSymbol:  0x%016X\n", uint64(fc.FileSymbol))
			fmt.Printf("  TypeSymbol:  0x%016X\n", uint64(fc.TypeSymbol))
			fmt.Printf("  FrameIndex:  %d\n", fc.FrameIndex)
			fmt.Printf("  DataOffset:  %d\n", fc.DataOffset)
			fmt.Printf("  Size:        %d\n", fc.Size)

			if int(fc.FrameIndex) < len(m.Frames) {
				fr := m.Frames[fc.FrameIndex]
				fmt.Printf("  Frame[%d]: PackageIndex: %d, Offset: %d, CompSz: %d, DecompSz: %d\n",
					fc.FrameIndex, fr.PackageIndex, fr.Offset, fr.CompressedSize, fr.Length)
			}
		}
	}

	for i, md := range m.Metadata {
		if md.FileSymbol == symSigned || md.TypeSymbol == symSigned ||
			md.AssetType == symSigned || md.Unk1 == symSigned || md.Unk2 == symSigned {
			found = true
			fmt.Printf("\nFound in Metadata[%d]:\n", i)
			fmt.Printf("  FileSymbol:               0x%016X\n", uint64(md.FileSymbol))
			fmt.Printf("  TypeSymbol:               0x%016X\n", uint64(md.TypeSymbol))
			fmt.Printf("  AssetType :               0x%016X\n", uint64(md.AssetType))
		}
	}

	if !found {
		fmt.Printf("Symbol 0x%016X not found in Manifest FrameContents or Metadata.\n", symVal)
	}
}
