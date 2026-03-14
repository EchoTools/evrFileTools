package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
)

func runSearch() error {
	var matchHash *int64
	var matchType *int64

	if searchHash != "" {
		v, err := parseHex(searchHash)
		if err != nil {
			return fmt.Errorf("invalid -search-hash %q: %w", searchHash, err)
		}
		iv := int64(v)
		matchHash = &iv
	}

	if searchType != "" {
		v, err := parseHex(searchType)
		if err != nil {
			return fmt.Errorf("invalid -search-type %q: %w", searchType, err)
		}
		iv := int64(v)
		matchType = &iv
	}

	matches := 0

	err := filepath.WalkDir(inputDir, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Skip sidecar files
		if strings.HasSuffix(filePath, ".meta") {
			return nil
		}

		typeSymbol, fileSymbol, err := manifest.ReadSidecar(filePath)
		if err != nil {
			return fmt.Errorf("read sidecar for %s: %w", filePath, err)
		}

		// Fall back to filename/dirname parsing if no sidecar
		if typeSymbol == 0 && fileSymbol == 0 {
			base := filepath.Base(filePath)
			// Strip extension(s) to get the stem
			stem := base
			for {
				ext := filepath.Ext(stem)
				if ext == "" {
					break
				}
				stem = strings.TrimSuffix(stem, ext)
			}
			if v, err := strconv.ParseUint(stem, 16, 64); err == nil {
				fileSymbol = int64(v)
			}

			parentDir := filepath.Base(filepath.Dir(filePath))
			if v, err := strconv.ParseUint(parentDir, 16, 64); err == nil {
				typeSymbol = int64(v)
			}
		}

		// Apply filters
		if matchHash != nil && fileSymbol != *matchHash {
			return nil
		}
		if matchType != nil && typeSymbol != *matchType {
			return nil
		}
		if searchName != "" {
			matched, err := path.Match(searchName, filepath.Base(filePath))
			if err != nil {
				return fmt.Errorf("invalid -search-name pattern %q: %w", searchName, err)
			}
			if !matched {
				return nil
			}
		}

		matches++
		if verbose {
			fmt.Printf("%s  type=%016x file=%016x\n", filePath, uint64(typeSymbol), uint64(fileSymbol))
		} else {
			fmt.Println(filePath)
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Found %d matches\n", matches)
	return nil
}

// parseHex parses a hex string (with or without leading "0x"/"0X") as uint64.
func parseHex(s string) (uint64, error) {
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return strconv.ParseUint(s, 16, 64)
}
