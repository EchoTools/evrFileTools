// Command batchextract extracts all packages from an EVR data directory in parallel.
//
// Usage:
//
//	batchextract -data ./ready-at-dawn/_data -output ./extracted
//	batchextract -data ./ready-at-dawn/_data -output ./extracted -workers 8
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
)

var (
	dataDir   string
	outputDir string
	workers   int
	verbose   bool
)

func init() {
	flag.StringVar(&dataDir, "data", "", "Path to _data directory containing manifests/ and packages/")
	flag.StringVar(&outputDir, "output", "", "Output directory for extracted files")
	flag.IntVar(&workers, "workers", runtime.NumCPU(), "Number of parallel extraction workers")
	flag.BoolVar(&verbose, "verbose", false, "Print each package as it is extracted")
}

func main() {
	flag.Parse()

	if dataDir == "" || outputDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: batchextract -data <_data dir> -output <output dir>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type result struct {
	name  string
	files int
	err   error
}

func run() error {
	manifestDir := filepath.Join(dataDir, "manifests")
	entries, err := os.ReadDir(manifestDir)
	if err != nil {
		return fmt.Errorf("read manifests directory: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("no manifests found in %s", manifestDir)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	// Collect manifest names (skip directories)
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}

	if len(names) == 0 {
		return fmt.Errorf("no manifest files found in %s (directory contains only subdirectories)", manifestDir)
	}

	fmt.Printf("Found %d manifests, extracting with %d workers...\n", len(names), workers)
	start := time.Now()

	// Feed work through a channel
	work := make(chan string, len(names))
	for _, n := range names {
		work <- n
	}
	close(work)

	results := make(chan result, len(names))
	var wg sync.WaitGroup

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range work {
				files, err := extractOne(name)
				results <- result{name: name, files: files, err: err}
			}
		}()
	}

	// Close results when all workers done
	go func() {
		wg.Wait()
		close(results)
	}()

	var (
		done       atomic.Int64
		totalFiles atomic.Int64
		errors     []string
	)

	total := int64(len(names))
	for r := range results {
		done.Add(1)
		if r.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", r.name, r.err))
			if verbose {
				fmt.Printf("[%d/%d] FAILED %s: %v\n", done.Load(), total, r.name, r.err)
			} else {
				fmt.Printf("\r[%d/%d] extracting...  ", done.Load(), total)
			}
		} else {
			totalFiles.Add(int64(r.files))
			if verbose {
				fmt.Printf("[%d/%d] %s (%d files)\n", done.Load(), total, r.name, r.files)
			} else {
				fmt.Printf("\r[%d/%d] extracting...  ", done.Load(), total)
			}
		}
	}

	elapsed := time.Since(start)
	fmt.Printf("\r%-40s\n", "") // clear progress line
	fmt.Printf("Extracted %d files from %d packages in %s\n", totalFiles.Load(), len(names)-len(errors), elapsed.Round(time.Millisecond))

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d packages failed:\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return fmt.Errorf("%d packages failed", len(errors))
	}

	return nil
}

func extractOne(name string) (int, error) {
	manifestPath := filepath.Join(dataDir, "manifests", name)
	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		return 0, fmt.Errorf("read manifest: %w", err)
	}

	packagePath := filepath.Join(dataDir, "packages", name)
	pkg, err := manifest.OpenPackage(m, packagePath)
	if err != nil {
		return 0, fmt.Errorf("open package: %w", err)
	}
	defer pkg.Close()

	dest := filepath.Join(outputDir, name)
	if err := os.MkdirAll(dest, 0755); err != nil {
		return 0, fmt.Errorf("create output dir: %w", err)
	}

	if err := pkg.Extract(dest); err != nil {
		return 0, fmt.Errorf("extract: %w", err)
	}

	return m.FileCount(), nil
}
