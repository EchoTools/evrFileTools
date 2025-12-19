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
go install github.com/EchoTools/evrFileTools/cmd/evrtools@latest
```

Or build from source:

```bash
git clone https://github.com/EchoTools/evrFileTools.git
cd evrFileTools
make build
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
    "github.com/EchoTools/evrFileTools/pkg/manifest"
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
├── pkg/
│   ├── archive/            # ZSTD archive format
│   │   ├── header.go       # Archive header (24 bytes)
│   │   ├── reader.go       # Streaming decompression
│   │   └── writer.go       # Streaming compression
│   └── manifest/           # EVR manifest/package handling
│       ├── manifest.go     # Manifest types and binary encoding
│       ├── package.go      # Multi-part package extraction
│       ├── builder.go      # Package building from files
│       └── scanner.go      # Input directory scanning
├── Makefile
└── go.mod
```

## Development

```bash
# Build
make build

# Run tests
make test

# Run benchmarks
make bench

# Format and lint
make check
```

## Performance

The library uses several optimizations:

- **Direct binary encoding** instead of reflection-based `binary.Read/Write`
- **Pre-allocated buffers** for zero-allocation encoding paths
- **ZSTD context reuse** for ~4x faster decompression with zero allocations
- **Frame index maps** for O(1) file lookups during extraction
- **Directory caching** to minimize syscalls

Run benchmarks to see current performance:

```bash
go test -bench=. -benchmem ./pkg/...
```

## License

MIT License - see LICENSE file
