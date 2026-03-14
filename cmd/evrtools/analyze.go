package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/EchoTools/evrFileTools/pkg/naming"
)

// magic signatures for common file formats
var magicSignatures = []struct {
	name   string
	offset int
	magic  []byte
}{
	{"DDS texture", 0, []byte{0x44, 0x44, 0x53, 0x20}},          // "DDS "
	{"OGG audio", 0, []byte{0x4F, 0x67, 0x67, 0x53}},            // "OggS"
	{"WAV audio", 0, []byte{0x52, 0x49, 0x46, 0x46}},            // "RIFF"
	{"PNG image", 0, []byte{0x89, 0x50, 0x4E, 0x47}},            // "\x89PNG"
	{"JPEG image", 0, []byte{0xFF, 0xD8, 0xFF}},                  // JPEG SOI
	{"JSON text", 0, []byte{0x7B}},                               // "{"
	{"JSON array", 0, []byte{0x5B}},                              // "["
	{"ZSTD compressed", 0, []byte{0x28, 0xB5, 0x2F, 0xFD}},      // zstd magic
	{"LZ4 compressed", 0, []byte{0x04, 0x22, 0x4D, 0x18}},       // lz4 magic
	{"Protobuf", 0, []byte{0x0A}},                                // common protobuf field tag
	{"EXE/DLL", 0, []byte{0x4D, 0x5A}},                          // "MZ"
	{"RAD video", 0, []byte{0x42, 0x49, 0x4B, 0x69}},            // "BIKi" (Bink)
	{"RIFF generic", 0, []byte{0x52, 0x49, 0x46, 0x46}},         // "RIFF"
	{"FLAC audio", 0, []byte{0x66, 0x4C, 0x61, 0x43}},           // "fLaC"
	{"MP3 audio", 0, []byte{0xFF, 0xFB}},                         // MP3 sync
	{"Null-padded", 0, []byte{0x00, 0x00, 0x00, 0x00}},          // all zeros
	{"UTF-16 text", 0, []byte{0xFF, 0xFE}},                       // BOM
}

type typeAnalysis struct {
	typeSymbol naming.TypeSymbol
	typeName   string
	count      int
	totalBytes int64
	formats    map[string]int
	entropy    float64 // average entropy of sampled files
	sizeMin    int64
	sizeMax    int64
}

func runAnalyze() error {
	if inputDir == "" {
		return fmt.Errorf("analyze mode requires -input")
	}

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	var analyses []typeAnalysis
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		val, err := strconv.ParseUint(entry.Name(), 16, 64)
		if err != nil {
			continue
		}
		ts := naming.TypeSymbol(int64(val))

		// Skip already-known types unless -force flag added later
		typeDir := filepath.Join(inputDir, entry.Name())
		a, err := analyzeTypeDir(ts, typeDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: analyze %s: %v\n", entry.Name(), err)
			continue
		}
		analyses = append(analyses, a)
	}

	// Sort: unknown types first (most interesting), then by count
	sort.Slice(analyses, func(i, j int) bool {
		iKnown := naming.IsKnownType(analyses[i].typeSymbol)
		jKnown := naming.IsKnownType(analyses[j].typeSymbol)
		if iKnown != jKnown {
			return !iKnown // unknowns first
		}
		return analyses[i].count > analyses[j].count
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE SYMBOL\tTYPE NAME\tFILES\tSIZE\tDETECTED FORMAT\tAVG ENTROPY")
	fmt.Fprintln(w, "-----------\t---------\t-----\t----\t---------------\t-----------")
	for _, a := range analyses {
		topFormat := topFormat(a.formats)
		fmt.Fprintf(w, "0x%016x\t%s\t%d\t%s\t%s\t%.2f\n",
			uint64(a.typeSymbol),
			a.typeName,
			a.count,
			formatBytes(a.totalBytes),
			topFormat,
			a.entropy,
		)
	}
	w.Flush()

	// Detailed breakdown for unknown types
	fmt.Printf("\n=== Unknown Type Details ===\n")
	hasUnknown := false
	for _, a := range analyses {
		if naming.IsKnownType(a.typeSymbol) {
			continue
		}
		hasUnknown = true
		fmt.Printf("\n0x%016x  (%d files, %s, entropy=%.2f)\n",
			uint64(a.typeSymbol), a.count, formatBytes(a.totalBytes), a.entropy)
		fmt.Printf("  Size range: %s – %s\n", formatBytes(a.sizeMin), formatBytes(a.sizeMax))
		if len(a.formats) > 0 {
			// Print all detected formats
			type kv struct {
				k string
				v int
			}
			var pairs []kv
			for k, v := range a.formats {
				pairs = append(pairs, kv{k, v})
			}
			sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
			fmt.Printf("  Detected formats:\n")
			for _, p := range pairs {
				pct := 100.0 * float64(p.v) / float64(a.count)
				fmt.Printf("    %-20s %5d files (%.0f%%)\n", p.k, p.v, pct)
			}
		}
	}
	if !hasUnknown {
		fmt.Println("No unknown types found.")
	}

	return nil
}

const analyzeSampleSize = 32 // files to sample per type dir

func analyzeTypeDir(ts naming.TypeSymbol, dir string) (typeAnalysis, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return typeAnalysis{}, err
	}

	a := typeAnalysis{
		typeSymbol: ts,
		typeName:   ts.String(),
		formats:    make(map[string]int),
		sizeMin:    math.MaxInt64,
	}

	// Sample up to analyzeSampleSize files for format detection
	sampleStep := 1
	if len(entries) > analyzeSampleSize {
		sampleStep = len(entries) / analyzeSampleSize
	}

	var totalEntropy float64
	sampled := 0

	for i, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		size := info.Size()
		a.count++
		a.totalBytes += size
		if size < a.sizeMin {
			a.sizeMin = size
		}
		if size > a.sizeMax {
			a.sizeMax = size
		}

		// Only sample every Nth file for format/entropy analysis
		if i%sampleStep != 0 {
			continue
		}

		path := filepath.Join(dir, e.Name())
		data, err := readSample(path)
		if err != nil {
			continue
		}

		// Detect magic
		sig := detectMagic(data)
		a.formats[sig]++

		// Compute entropy on sample
		totalEntropy += shannonEntropy(data)
		sampled++
	}

	if a.sizeMin == math.MaxInt64 {
		a.sizeMin = 0
	}
	if sampled > 0 {
		a.entropy = totalEntropy / float64(sampled)
	}

	return a, nil
}

func readSample(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, 256)
	n, _ := f.Read(buf)
	return buf[:n], nil
}

func detectMagic(data []byte) string {
	for _, sig := range magicSignatures {
		if sig.offset+len(sig.magic) > len(data) {
			continue
		}
		match := true
		for i, b := range sig.magic {
			if data[sig.offset+i] != b {
				match = false
				break
			}
		}
		if match {
			return sig.name
		}
	}
	return "unknown"
}

func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}
	var freq [256]int
	for _, b := range data {
		freq[b]++
	}
	n := float64(len(data))
	var h float64
	for _, f := range freq {
		if f == 0 {
			continue
		}
		p := float64(f) / n
		h -= p * math.Log2(p)
	}
	return h
}

func topFormat(formats map[string]int) string {
	best := "unknown"
	bestN := 0
	for k, v := range formats {
		if v > bestN {
			bestN = v
			best = k
		}
	}
	return best
}
