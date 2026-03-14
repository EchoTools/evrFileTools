package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"text/tabwriter"

	"github.com/EchoTools/evrFileTools/pkg/naming"
)

type typeStats struct {
	typeSymbol naming.TypeSymbol
	typeName   string
	count      int
	totalBytes int64
}

func runInventory() error {
	if inputDir == "" {
		return fmt.Errorf("inventory mode requires -input")
	}

	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}

	var stats []typeStats
	totalFiles := 0
	totalBytes := int64(0)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Type directories are named as unsigned hex type symbols
		val, err := strconv.ParseUint(entry.Name(), 16, 64)
		if err != nil {
			continue // skip non-hex directories
		}
		ts := naming.TypeSymbol(int64(val))

		typeDir := filepath.Join(inputDir, entry.Name())
		count, bytes, err := countFiles(typeDir)
		if err != nil {
			return fmt.Errorf("count files in %s: %w", typeDir, err)
		}

		stats = append(stats, typeStats{
			typeSymbol: ts,
			typeName:   ts.String(),
			count:      count,
			totalBytes: bytes,
		})
		totalFiles += count
		totalBytes += bytes
	}

	// Sort by file count descending
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].count > stats[j].count
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tFILES\tSIZE\tTYPE SYMBOL")
	fmt.Fprintln(w, "----\t-----\t----\t-----------")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%d\t%s\t0x%016x\n",
			s.typeName,
			s.count,
			formatBytes(s.totalBytes),
			uint64(s.typeSymbol),
		)
	}
	fmt.Fprintln(w, "----\t-----\t----\t-----------")
	fmt.Fprintf(w, "TOTAL\t%d\t%s\t\n", totalFiles, formatBytes(totalBytes))
	w.Flush()

	knownCount := 0
	for _, s := range stats {
		if naming.IsKnownType(s.typeSymbol) {
			knownCount += s.count
		}
	}
	unknownCount := totalFiles - knownCount
	fmt.Printf("\n%d known types, %d unknown types (%d files known / %d files unknown)\n",
		countKnownTypes(stats), len(stats)-countKnownTypes(stats), knownCount, unknownCount)

	return nil
}

func countFiles(dir string) (int, int64, error) {
	var count int
	var total int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return 0, 0, err
		}
		count++
		total += info.Size()
	}
	return count, total, nil
}

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/GB)
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/MB)
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/KB)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func countKnownTypes(stats []typeStats) int {
	n := 0
	for _, s := range stats {
		if naming.IsKnownType(s.typeSymbol) {
			n++
		}
	}
	return n
}
