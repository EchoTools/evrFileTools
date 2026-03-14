// Package main provides a command-line tool for working with EVR package files.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/EchoTools/evrFileTools/pkg/hash"
	"github.com/EchoTools/evrFileTools/pkg/manifest"
	"github.com/EchoTools/evrFileTools/pkg/naming"
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
	verbose        bool
	diffManifestA  string
	diffManifestB  string
	wordlistFile   string
	searchHash     string
	searchType     string
	searchName     string
	patchType      string
	patchFile      string
	patchInput     string
)

func init() {
	flag.StringVar(&mode, "mode", "", "Operation mode: extract, build, inventory, analyze, diff, search, patch, audio")
	flag.StringVar(&packageName, "package", "", "Package name (e.g., 48037dc70b0ecab2)")
	flag.StringVar(&dataDir, "data", "", "Path to _data directory containing manifests/packages")
	flag.StringVar(&inputDir, "input", "", "Input directory (inventory/analyze/build/audio mode)")
	flag.StringVar(&outputDir, "output", "", "Output directory (extract/build mode)")
	flag.BoolVar(&preserveGroups, "preserve-groups", false, "Preserve frame grouping in output")
	flag.BoolVar(&forceOverwrite, "force", false, "Allow non-empty output directory")
	flag.BoolVar(&useDecimalName, "decimal-names", false, "Use decimal format for filenames (default is hex)")
	flag.BoolVar(&verbose, "verbose", false, "Print detailed file list (diff/audio mode)")
	flag.StringVar(&diffManifestA, "manifest-a", "", "First manifest path (diff mode)")
	flag.StringVar(&diffManifestB, "manifest-b", "", "Second manifest path (diff mode)")
	flag.StringVar(&wordlistFile, "wordlist", "", "Path to name wordlist for friendly-named extraction (extract mode)")
	flag.StringVar(&searchHash, "search-hash", "", "File symbol hash to search for (search mode, e.g. 0x74d228d09dc5dd8f)")
	flag.StringVar(&searchType, "search-type", "", "Type symbol hash to filter by (search mode, optional)")
	flag.StringVar(&searchName, "search-name", "", "Filename glob pattern to match (search mode, e.g. \"rwd_tint_*\")")
	flag.StringVar(&patchType, "patch-type", "", "Type symbol of file to replace, hex (e.g. beac1969cb7b8861) (patch mode)")
	flag.StringVar(&patchFile, "patch-file", "", "File symbol of file to replace, hex (patch mode)")
	flag.StringVar(&patchInput, "patch-input", "", "Path to replacement file (patch mode)")
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

	needsOutput := mode == "extract" || mode == "build" || mode == "patch"
	if needsOutput {
		if outputDir == "" {
			return fmt.Errorf("output directory is required")
		}
		if err := prepareOutputDir(); err != nil {
			return err
		}
	}

	switch mode {
	case "extract":
		return runExtract()
	case "build":
		return runBuild()
	case "inventory":
		return runInventory()
	case "analyze":
		return runAnalyze()
	case "diff":
		return runDiff()
	case "search":
		return runSearch()
	case "patch":
		return runPatch()
	case "audio":
		return runAudio()
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

func validateFlags() error {
	if mode == "" {
		return fmt.Errorf("mode is required")
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
	case "inventory", "analyze", "audio":
		if inputDir == "" {
			return fmt.Errorf("%s mode requires -input", mode)
		}
	case "diff":
		if diffManifestA == "" || diffManifestB == "" {
			return fmt.Errorf("diff mode requires -manifest-a and -manifest-b")
		}
	case "search":
		if inputDir == "" {
			return fmt.Errorf("search mode requires -input")
		}
		if searchHash == "" && searchType == "" && searchName == "" {
			return fmt.Errorf("search mode requires at least one of -search-hash, -search-type, -search-name")
		}
	case "patch":
		if dataDir == "" || packageName == "" {
			return fmt.Errorf("patch mode requires -data and -package")
		}
		if patchType == "" || patchFile == "" || patchInput == "" {
			return fmt.Errorf("patch mode requires -patch-type, -patch-file, and -patch-input")
		}
		if outputDir == "" {
			return fmt.Errorf("patch mode requires -output")
		}
	default:
		return fmt.Errorf("mode must be one of: extract, build, inventory, analyze, diff, search, patch, audio")
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

	opts := []manifest.ExtractOption{
		manifest.WithPreserveGroups(preserveGroups),
		manifest.WithDecimalNames(useDecimalName),
	}

	if wordlistFile != "" {
		nameTable, err := loadNameTable(wordlistFile)
		if err != nil {
			return fmt.Errorf("load wordlist: %w", err)
		}
		fmt.Printf("Loaded %d names from wordlist\n", len(nameTable))
		opts = append(opts, manifest.WithNameTable(nameTable))
		opts = append(opts, manifest.WithTypeNames(buildTypeNames()))
	}

	fmt.Println("Extracting files...")
	if err := pkg.Extract(outputDir, opts...); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	fmt.Printf("Extraction complete. Files written to %s\n", outputDir)
	return nil
}

// loadNameTable reads a wordlist file (one name per line) and returns a
// fileSymbol→name map using CSymbol64Hash.
func loadNameTable(path string) (map[int64]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	table := make(map[int64]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name == "" || strings.HasPrefix(name, "#") {
			continue
		}
		sym := int64(hash.CSymbol64Hash(name))
		table[sym] = name
	}
	return table, scanner.Err()
}

// buildTypeNames returns a map of typeSymbol→directory name for all known types.
func buildTypeNames() map[int64]string {
	m := make(map[int64]string)
	for _, ts := range naming.AllKnownTypes() {
		m[int64(ts)] = naming.TypeName(ts)
	}
	return m
}

func runBuild() error {
	fmt.Println("Scanning input directory...")
	files, err := manifest.ScanFiles(inputDir)
	if err != nil {
		return fmt.Errorf("scan files: %w", err)
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
