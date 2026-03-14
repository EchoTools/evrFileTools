// Package main provides a command-line tool for working with EVR package files.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
)

var (
	mode           string
	packageName    string
	dataDir        string
	inputDir       string
	outputDir      string
	preserveGroups bool
	forceOverwrite bool
	useDecimalName bool
	exportTypes    string
	quickMode      bool
)

func init() {
	flag.StringVar(&mode, "mode", "", "Operation mode: extract, build")
	flag.StringVar(&packageName, "package", "", "Package name (e.g., 48037dc70b0ecab2)")
	flag.StringVar(&dataDir, "data", "", "Path to _data directory containing manifests/packages")
	flag.StringVar(&inputDir, "input", "", "Input directory for build mode")
	flag.StringVar(&outputDir, "output", "", "Output directory")
	flag.BoolVar(&preserveGroups, "preserve-groups", false, "Preserve frame grouping in output")
	flag.BoolVar(&forceOverwrite, "force", false, "Allow non-empty output directory")
	flag.BoolVar(&useDecimalName, "decimal-names", false, "Use decimal format for filenames (default is hex)")
	flag.StringVar(&exportTypes, "export", "", "Comma-separated list of types to export (textures, tints)")
	flag.BoolVar(&quickMode, "quick", false, "Quick swap mode (modifies game files in-place)")
}

func main() {
	flag.Parse()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := validateFlags(); err != nil {
		flag.Usage()
		return err
	}

	if err := prepareOutputDir(); err != nil {
		return err
	}

	switch mode {
	case "extract":
		return runExtract()
	case "build":
		return runBuild()
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

func validateFlags() error {
	if mode == "" {
		return fmt.Errorf("mode is required")
	}
	if outputDir == "" {
		return fmt.Errorf("output directory is required")
	}

	switch mode {
	case "extract":
		if dataDir == "" || packageName == "" {
			return fmt.Errorf("extract mode requires -data and -package")
		}
	case "build":
		if inputDir == "" {
			return fmt.Errorf("build mode requires -input")
		}
		if packageName == "" {
			packageName = "package"
		}
	default:
		return fmt.Errorf("mode must be 'extract' or 'build'")
	}

	return nil
}

func prepareOutputDir() error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if !forceOverwrite {
		empty, err := isDirEmpty(outputDir)
		if err != nil {
			return fmt.Errorf("check output directory: %w", err)
		}
		if !empty {
			return fmt.Errorf("output directory is not empty (use -force to override)")
		}
	}

	return nil
}

func isDirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdir(1)
	return err == io.EOF, nil
}

func runExtract() error {
	manifestPath := filepath.Join(dataDir, "manifests", packageName)
	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	fmt.Printf("Manifest loaded: %d files in %d packages\n", m.FileCount(), m.PackageCount())

	packagePath := filepath.Join(dataDir, "packages", packageName)
	pkg, err := manifest.OpenPackage(m, packagePath)
	if err != nil {
		return fmt.Errorf("open package: %w", err)
	}
	defer pkg.Close()

	var filterTypes []int64
	if exportTypes != "" {
		for _, t := range strings.Split(exportTypes, ",") {
			switch strings.TrimSpace(t) {
			case "textures":
				// Use variables to avoid constant overflow checks for negative int64s
				t1 := uint64(0xBEAC1969CB7B8861)
				t2 := uint64(0x4A4C32C49300B8A0)
				t3 := uint64(0xe2efe7289d5985b8)
				t4 := uint64(0x489bb35d53ca50e9)
				filterTypes = append(filterTypes,
					int64(t1), // -4707359568332879775
					int64(t2), // 5353709876897953952
					int64(t3), // -2094201140079393352
					int64(t4), // 5231972605540061417
				)
			case "tints":
				filterTypes = append(filterTypes,
					int64(uint64(0x24CBFD54E9A7F2EA)), // Folder: 24cbfd54e9a7f2ea
					int64(uint64(0x32f30fe361939dee)), // 3671295590506143214
				)
			}
		}
	}

	fmt.Println("Extracting files...")
	if err := pkg.Extract(
		outputDir,
		manifest.WithPreserveGroups(preserveGroups),
		manifest.WithDecimalNames(useDecimalName),
		manifest.WithTypeFilter(filterTypes),
	); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	fmt.Printf("Extraction complete. Files written to %s\n", outputDir)
	return nil
}

func runBuild() error {
	fmt.Println("Scanning input directory...")
	files, err := manifest.ScanFiles(inputDir)
	if err != nil {
		return fmt.Errorf("scan files: %w", err)
	}

	// If dataDir is provided, we are in "repack" mode where we merge original files
	if dataDir != "" {
		manifestPath := filepath.Join(dataDir, "manifests", packageName)
		if _, err := os.Stat(manifestPath); err == nil {
			if quickMode {
				m, err := manifest.ReadFile(manifestPath)
				if err != nil {
					return fmt.Errorf("read manifest: %w", err)
				}
				return manifest.QuickRepack(m, files, dataDir, packageName)
			}
			return runRepack(files)
		}
	}

	totalFiles := 0
	for _, group := range files {
		totalFiles += len(group)
	}
	fmt.Printf("Found %d files in %d groups\n", totalFiles, len(files))

	fmt.Println("Building package...")
	builder := manifest.NewBuilder(outputDir, packageName)
	m, err := builder.Build(files)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	manifestDir := filepath.Join(outputDir, "manifests")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}

	manifestPath := filepath.Join(manifestDir, packageName)
	if err := manifest.WriteFile(manifestPath, m); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	fmt.Printf("Build complete. Output written to %s\n", outputDir)
	return nil
}

func runRepack(inputFiles [][]manifest.ScannedFile) error {
	fmt.Println("Loading original manifest for repacking...")
	manifestPath := filepath.Join(dataDir, "manifests", packageName)
	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	return manifest.Repack(m, inputFiles, outputDir, packageName, dataDir)
}
