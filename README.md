# evrFileTools

A Go library and CLI tool for working with EVR (Echo VR) package and manifest files.

> Thanks to [Exhibitmark](https://github.com/Exhibitmark) for [carnation](https://github.com/Exhibitmark/carnation) which helped with reversing the manifest format!

## Features

- Extract files from EVR packages
- Build new packages from extracted files
- Read and write EVR manifest files
- ZSTD compression/decompression with optimized context reuse

## Installation

```bash
go install github.com/goopsie/evrFileTools/cmd/evrtools@latest
```

Or build from source:

```bash
git clone https://github.com/goopsie/evrFileTools.git
cd evrFileTools
go build -o evrtools ./cmd/evrtools
```

## Usage

### Extract files from a package

```bash
evrtools -mode extract \
    -data ./ready-at-dawn-echo-arena/_data/5932408047/rad15/win10 \
    -package 48037dc70b0ecab2 \
    -output ./extracted
```

This extracts all files from the package. Output structure:
- `./output/<typeSymbol>/<fileSymbol>`

With `-preserve-groups`, frames are preserved:
- `./output/<frameIndex>/<typeSymbol>/<fileSymbol>`

### Build a package from files

```bash
evrtools -mode build \
    -input ./files \
    -output ./output \
    -package mypackage
```

Expected input structure: `./input/<frameIndex>/<typeSymbol>/<fileSymbol>`

### CLI Options

| Flag | Description |
|------|-------------|
| `-mode` | Operation mode: `extract` or `build` |
| `-data` | Path to _data directory containing manifests/packages |
| `-package` | Package name (e.g., `48037dc70b0ecab2`) |
| `-input` | Input directory for build mode |
| `-output` | Output directory |
| `-preserve-groups` | Preserve frame grouping in extract output |
| `-force` | Allow non-empty output directory |

## Library Usage

```go
package main

import (
    "log"
    "github.com/goopsie/evrFileTools/pkg/manifest"
)

func main() {
    // Read a manifest
    m, err := manifest.ReadFile("/path/to/manifests/packagename")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Manifest: %d files in %d packages", m.FileCount(), m.PackageCount())

    // Open the package files
    pkg, err := manifest.OpenPackage(m, "/path/to/packages/packagename")
    if err != nil {
        log.Fatal(err)
    }
    defer pkg.Close()

    // Extract all files
    if err := pkg.Extract("./output"); err != nil {
        log.Fatal(err)
    }
}
```

## Project Structure

```
evrFileTools/
├── cmd/
│   └── evrtools/           # CLI application
│       └── main.go
├── pkg/
│   ├── archive/            # ZSTD archive format
│   │   ├── header.go       # Archive header types
│   │   ├── reader.go       # Decompression
│   │   └── writer.go       # Compression
│   └── manifest/           # EVR manifest/package handling
│       ├── manifest.go     # Manifest types and parsing
│       ├── package.go      # Package file handling
│       ├── builder.go      # Package building
│       └── scanner.go      # Input file scanning
├── evrManifests/           # Legacy manifest types (deprecated)
├── tool/                   # Legacy package (deprecated)
└── go.mod
```

## Benchmarks

Run benchmarks:

```bash
go test -bench=. -benchmem ./pkg/...
```

Key findings:
- Context reuse for ZSTD decompression is ~5x faster with zero allocations
- Struct keys for lookups outperform byte array keys

## Legacy CLI

The original `main.go` CLI is still available but deprecated. Use `cmd/evrtools` for new projects.

## License

MIT License - see LICENSE file
