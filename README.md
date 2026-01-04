# evrFileTools

A Go library and CLI tool for working with EVR (Echo VR) package and manifest files.

> Thanks to [Exhibitmark](https://github.com/Exhibitmark) for [carnation](https://github.com/Exhibitmark/carnation) which helped with reversing the manifest format!

## Features

- Extract files from EVR packages
- Build new packages from extracted files
- Read and write EVR manifest files
- ZSTD compression/decompression with optimized context reuse
- **Tint asset discovery and parsing** - Find and display tint color data
- **Asset type analysis** - Analyze manifest contents by type and asset

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

## Tools

### evrtools - Package extraction and building

```bash
# Extract files from a package
evrtools -mode extract \
    -data ./ready-at-dawn-echo-arena/_data/5932408047/rad15/win10 \
    -package 48037dc70b0ecab2 \
    -output ./extracted

# Build a package from files
evrtools -mode build \
    -input ./files \
    -output ./output \
    -package mypackage
```

### listtints - Manifest analysis for tint discovery

Scans manifests to find tint assets and provides statistics on asset types:

```bash
./listtints /path/to/manifests/48037dc70b0ecab2
```

Output includes:
- Top type symbols by file count
- Top asset types by file count  
- Known tint matches (if any FileSymbols match known tint hashes)

### showtints - Display tint color data

Parses extracted 96-byte tint entry files and displays their color values:

```bash
./showtints /path/to/extracted/files
```

Output shows:
- ResourceID (Symbol64 hash)
- Tint name (if known)
- 6 color blocks as RGBA float32 values with hex equivalents
- Raw byte dump for analysis

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
│   ├── evrtools/           # CLI application - extract/build packages
│   ├── listtints/          # Manifest scanner for tint discovery
│   └── showtints/          # Tint entry parser and displayer
├── pkg/
│   ├── archive/            # ZSTD archive format
│   │   ├── header.go       # Archive header (24 bytes)
│   │   ├── reader.go       # Streaming decompression
│   │   └── writer.go       # Streaming compression
│   ├── manifest/           # EVR manifest/package handling
│   │   ├── manifest.go     # Manifest types and binary encoding
│   │   ├── package.go      # Multi-part package extraction
│   │   ├── builder.go      # Package building from files
│   │   └── scanner.go      # Input directory scanning
│   └── tint/               # Tint asset handling
│       └── tint.go         # Tint structures and known tints
├── Makefile
└── go.mod
```

## Tint Format Documentation

Tints in Echo VR are cosmetic color schemes applied to player chassis. Based on Ghidra reverse engineering of `echovr.exe`, tints are stored as color data within `CR15NetRewardItemCS` component entries.

### Tint Entry Structure (96 bytes / 0x60)

| Offset | Size | Type | Description |
|--------|------|------|-------------|
| 0x00 | 8 | uint64 | ResourceID (Symbol64 hash) |
| 0x08 | 16 | Color | Color 0 - Main/Primary color |
| 0x18 | 16 | Color | Color 1 - Accent color |
| 0x28 | 16 | Color | Color 2 - Secondary main |
| 0x38 | 16 | Color | Color 3 - Secondary accent |
| 0x48 | 16 | Color | Color 4 - Tertiary main |
| 0x50 | 16 | Color | Color 5 - Tertiary accent |

### Color Structure (16 bytes)

Each color is 4 float32 values in RGBA order (little-endian):

| Offset | Size | Type | Description |
|--------|------|------|-------------|
| 0x00 | 4 | float32 | Red (0.0-1.0) |
| 0x04 | 4 | float32 | Green (0.0-1.0) |
| 0x08 | 4 | float32 | Blue (0.0-1.0) |
| 0x0C | 4 | float32 | Alpha (0.0-1.0) |

### Known Tint Hashes

There are 48 known tints in Echo VR. Sample hashes:

| Hash | Name |
|------|------|
| `0x74d228d09dc5dc86` | rwd_tint_0000 |
| `0x74d228d09dc5dc87` | rwd_tint_0001 |
| `0x3e474b60a9416aca` | rwd_tint_s1_a_default |
| `0x43ac219540f9df74` | rwd_tint_s1_b_default |
| `0x0bf4c0e4d2eaa06c` | rwd_tint_s2_a_default |
| `0xa11587a1254c9507` | rwd_tint_s3_tint_a |

See `pkg/tint/tint.go` for the complete list of 48 known tint hashes.

### Binary Functions (from Ghidra analysis)

| Address | Name | Description |
|---------|------|-------------|
| `0x140cf23c0` | `CR15NetRewardItemCS_RegisterTint` | Registers tints from reward item data |
| `0x140d4ed80` | `CR15NetRewardItemCS_ClearTintTables` | Removes tints when items unloaded |
| `0x140d20710` | `R15NetCustomization_OverrideTint` | Applies tint colors to chassis |
| `0x140c682f0` | `CSymbolTable_BinarySearch` | Binary search for tint lookup |

### Global Tint Tables

| Address | Name | Entry Size | Description |
|---------|------|------------|-------------|
| `0x1420d3ac0` | `g_TintTable_ItemIDs` | 0x18 | Primary lookup by ResourceID |
| `0x1420d3ac8` | `g_TintTable_Secondary` | 0x20 | Secondary tint values |
| `0x1420d3ad0` | `g_TintTable_Tertiary` | 0x20 | Tertiary tint values |
| `0x1420d3ad8` | `g_TintTable_Flags` | - | Flags/alignment lookup |
| `0x1420d3ae0` | `g_TintTableRefCount` | 4 | Reference counter |

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
