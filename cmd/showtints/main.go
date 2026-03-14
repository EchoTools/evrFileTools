// Package main provides a command-line tool to display tint data from extracted files.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/EchoTools/evrFileTools/pkg/tint"
)

var (
	summaryOnly  bool
	knownOnly    bool
	nonZeroOnly  bool
	showRawBytes bool
	cssOutput    bool
)

func init() {
	flag.BoolVar(&summaryOnly, "summary", false, "Only show summary statistics, not individual entries")
	flag.BoolVar(&knownOnly, "known", false, "Only show entries matching known tint hashes")
	flag.BoolVar(&nonZeroOnly, "nonzero", false, "Only show entries with non-zero color data")
	flag.BoolVar(&showRawBytes, "raw", true, "Show raw hex bytes (default: true)")
	flag.BoolVar(&cssOutput, "css", false, "Output tints as CSS custom properties")
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: showtints [options] <extracted_dir>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\nDescription:")
		fmt.Println("  Scans extracted files for 96-byte tint entries and displays their contents.")
		fmt.Println("  Tint entries contain a ResourceID (Symbol64) and 6 RGBA color blocks.")
		fmt.Println("\nExamples:")
		fmt.Println("  showtints ./extracted")
		fmt.Println("  showtints --css --known ./extracted > tints.css")
		fmt.Println("  showtints --summary --nonzero ./extracted")
		os.Exit(1)
	}

	extractedDir := flag.Arg(0)

	var (
		totalFiles   int
		knownTints   int
		nonZeroFiles int
		zeroFiles    int
	)

	// Walk through extracted files looking for 96-byte files (tint entries)
	err := filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Only process regular files that are exactly 96 bytes (tint entry size)
		if !info.IsDir() && info.Size() == 96 {
			totalFiles++

			entry, data, err := readTintFile(path)
			if err != nil {
				if !summaryOnly {
					fmt.Printf("Error processing %s: %v\n", path, err)
				}
				return nil
			}

			isKnown := tint.LookupTintName(entry.ResourceID) != ""
			isNonZero := hasNonZeroColors(entry)

			if isKnown {
				knownTints++
			}
			if isNonZero {
				nonZeroFiles++
			} else {
				zeroFiles++
			}

			// Apply filters
			if knownOnly && !isKnown {
				return nil
			}
			if nonZeroOnly && !isNonZero {
				return nil
			}

			if !summaryOnly {
				if cssOutput {
					displayTintCSS(entry)
				} else {
					displayTintEntry(path, entry, data)
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Show summary unless in CSS mode
	if !cssOutput {
		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Total 96-byte files:    %d\n", totalFiles)
		fmt.Printf("Known tint matches:     %d\n", knownTints)
		fmt.Printf("Files with color data:  %d\n", nonZeroFiles)
		fmt.Printf("Empty/zero files:       %d\n", zeroFiles)
		fmt.Printf("Known tints in lookup:  %d\n", len(tint.KnownTints))
	}
}

func readTintFile(path string) (*tint.TintEntry, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	data := make([]byte, tint.TintEntrySize)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, nil, err
	}

	entry := tint.TintEntryFromBytes(data)
	if entry == nil {
		return nil, nil, fmt.Errorf("failed to parse tint entry")
	}

	return entry, data, nil
}

func hasNonZeroColors(entry *tint.TintEntry) bool {
	for _, c := range entry.Colors {
		if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 0 {
			return true
		}
	}
	return entry.ResourceID != 0
}

func displayTintCSS(entry *tint.TintEntry) {
	tintName := tint.LookupTintName(entry.ResourceID)
	if tintName == "" {
		tintName = fmt.Sprintf("unknown-%016x", entry.ResourceID)
	}

	fmt.Print(entry.ToCSS(tintName))
	fmt.Println()
}

func displayTintEntry(path string, entry *tint.TintEntry, data []byte) {
	relDir := filepath.Dir(path)

	tintName := tint.LookupTintName(entry.ResourceID)
	if tintName == "" {
		tintName = "UNKNOWN"
	}

	fmt.Printf("\n=== Tint Entry ===\n")
	fmt.Printf("File:        %s\n", filepath.Base(path))
	fmt.Printf("Directory:   %s\n", filepath.Base(relDir))
	fmt.Printf("ResourceID:  0x%016x\n", entry.ResourceID)
	fmt.Printf("Tint Name:   %s\n", tintName)
	fmt.Printf("\nColor Blocks (RGBA float32):\n")

	colorNames := []string{
		"Color 0 (Main 1)",
		"Color 1 (Accent 1)",
		"Color 2 (Main 2)",
		"Color 3 (Accent 2)",
		"Color 4 (Body)",
	}

	for i, color := range entry.Colors {
		fmt.Printf("  %s: %s\n", colorNames[i], color.String())
		fmt.Printf("               Hex: %s\n", color.Hex())
	}

	if showRawBytes {
		fmt.Printf("\nRaw Bytes (hex):\n")
		for i := 0; i < len(data); i += 16 {
			end := i + 16
			if end > len(data) {
				end = len(data)
			}
			fmt.Printf("  [%02x-%02x]: ", i, end-1)
			for j := i; j < end; j++ {
				fmt.Printf("%02x ", data[j])
			}
			fmt.Printf("\n")
		}
	}
}

