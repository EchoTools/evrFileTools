package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/EchoTools/evrFileTools/pkg/manifest"
)

func runPatch() error {
	// 1. Read manifest from dataDir/manifests/packageName.
	manifestPath := filepath.Join(dataDir, "manifests", packageName)
	m, err := manifest.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	fmt.Printf("Manifest loaded: %d files in %d packages\n", m.FileCount(), m.PackageCount())

	// 2. Read replacement file from patchInput.
	data, err := os.ReadFile(patchInput)
	if err != nil {
		return fmt.Errorf("read patch input file %s: %w", patchInput, err)
	}
	fmt.Printf("Replacement file read: %d bytes\n", len(data))

	// 3. Parse patchType and patchFile hex strings.
	typeSymbol, err := parseHexSymbol(patchType)
	if err != nil {
		return fmt.Errorf("parse -patch-type %q: %w", patchType, err)
	}
	fileSymbol, err := parseHexSymbol(patchFile)
	if err != nil {
		return fmt.Errorf("parse -patch-file %q: %w", patchFile, err)
	}

	// 4. Copy original package files to outputDir/packages/.
	srcPkgDir := filepath.Join(dataDir, "packages")
	dstPkgDir := filepath.Join(outputDir, "packages")
	if err := os.MkdirAll(dstPkgDir, 0755); err != nil {
		return fmt.Errorf("create output packages dir: %w", err)
	}

	if err := copyPackageFiles(srcPkgDir, dstPkgDir, packageName, m.PackageCount()); err != nil {
		return fmt.Errorf("copy package files: %w", err)
	}
	fmt.Printf("Copied %d package file(s) to %s\n", m.PackageCount(), dstPkgDir)

	// 5. Call manifest.PatchFile with the copied package base path.
	pkgBasePath := filepath.Join(dstPkgDir, packageName)
	updated, err := manifest.PatchFile(m, pkgBasePath, typeSymbol, fileSymbol, data)
	if err != nil {
		return fmt.Errorf("patch file: %w", err)
	}

	// 6. Write updated manifest to outputDir/manifests/packageName.
	manifestsDir := filepath.Join(outputDir, "manifests")
	if err := os.MkdirAll(manifestsDir, 0755); err != nil {
		return fmt.Errorf("create output manifests dir: %w", err)
	}
	outManifestPath := filepath.Join(manifestsDir, packageName)
	if err := manifest.WriteFile(outManifestPath, updated); err != nil {
		return fmt.Errorf("write updated manifest: %w", err)
	}

	fmt.Printf("Patch complete. Updated manifest written to %s\n", outManifestPath)
	return nil
}

// parseHexSymbol parses a hex string (with or without 0x prefix) into int64.
func parseHexSymbol(s string) (int64, error) {
	// Strip optional 0x prefix.
	trimmed := s
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		trimmed = s[2:]
	}
	v, err := strconv.ParseUint(trimmed, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid hex value %q: %w", s, err)
	}
	return int64(v), nil
}

// copyPackageFiles copies all package files (<name>_0, <name>_1, ...) from src to dst dir.
func copyPackageFiles(srcDir, dstDir, name string, count int) error {
	for i := 0; i < count; i++ {
		filename := fmt.Sprintf("%s_%d", name, i)
		src := filepath.Join(srcDir, filename)
		dst := filepath.Join(dstDir, filename)
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy %s: %w", filename, err)
		}
	}
	return nil
}

// copyFile copies a single file from src to dst, creating dst if necessary.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
