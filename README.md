# evrFileTools

A Go toolkit for working with EVR (Echo VR) package and manifest files.

> Thanks to [Exhibitmark](https://github.com/Exhibitmark) for [carnation](https://github.com/Exhibitmark/carnation) which helped with reversing the manifest format!

## Features

- Extract files from EVR packages
- Build new packages from extracted files
- Analyze and diff manifests
- Inventory extracted assets by type
- Compute and reverse-lookup EVR symbol hashes
- Full texture conversion pipeline: DDS <-> PNG with BC1/BC3 compression
- Display and export tint color data as CSS
- ZSTD compression/decompression with optimized context reuse

## Installation

### Requirements

- **Go 1.24 or later**
- **libsquish** (optional, for PNG -> DDS encoding in `texconv`)
  - Ubuntu/Debian: `sudo apt-get install libsquish-dev`
  - Arch Linux: `sudo pacman -S libsquish`
  - macOS: `brew install squish`

### Install via Go

```bash
go install github.com/EchoTools/evrFileTools/cmd/evrtools@latest
go install github.com/EchoTools/evrFileTools/cmd/showtints@latest
go install github.com/EchoTools/evrFileTools/cmd/texconv@latest
go install github.com/EchoTools/evrFileTools/cmd/symhash@latest
```

### Build from Source

```bash
git clone https://github.com/EchoTools/evrFileTools.git
cd evrFileTools
make build
```

Binaries are output to the `bin/` directory.

## Usage

### evrtools - Package Management

#### Extract files from a package

```bash
evrtools -mode extract \
    -data ./ready-at-dawn-echo-arena/_data/5932408047/rad15/win10 \
    -package 48037dc70b0ecab2 \
    -output ./extracted
```

Output structure: `./output/<typeSymbol>/<fileSymbol>`

With `-preserve-groups`: `./output/<frameIndex>/<typeSymbol>/<fileSymbol>`

#### Build a package from files

```bash
evrtools -mode build \
    -input ./files \
    -output ./output \
    -package mypackage
```

Expected input structure: `./input/<frameIndex>/<typeSymbol>/<fileSymbol>`

#### Analyze extracted assets

```bash
# Inventory: count files and sizes by type
evrtools -mode inventory -input ./extracted

# Analyze: detect file formats via magic bytes + entropy
evrtools -mode analyze -input ./extracted

# Diff: compare two manifests
evrtools -mode diff \
    -data ./path/to/_data \
    -manifest-a package_a \
    -manifest-b package_b
```

### symhash - Symbol Hash Tool

Compute and reverse-lookup EVR symbol hashes:

```bash
# Hash a symbol name
symhash mySymbolName

# Reverse-lookup from a hash
symhash -reverse 0xbeac1969cb7b8861

# Use a wordlist for reverse lookups
symhash -reverse 0xbeac1969cb7b8861 -wordlist symbols.txt

# Use SNS message hash algorithm
symhash -algo sns SNSLobbySmiteEntrant
```

### showtints - Tint Color Display

```bash
# Show all tints with details
showtints ./extracted

# Export tints as CSS custom properties
showtints --css --known ./extracted > tints.css

# Show summary only
showtints --summary ./extracted
```

### texconv - Texture Converter

```bash
# Decode DDS to PNG for editing
texconv decode texture.dds texture.png

# Encode PNG back to DDS with automatic format detection
texconv encode texture.png texture.dds

# Show texture information
texconv info texture.dds

# Batch convert directory
texconv batch decode _extracted/ png_output/
texconv batch encode png_input/ dds_output/
```

Supported formats: BC1 (DXT1), BC3 (DXT5), BC5 (partial), BC6H/BC7 (decode only).

## CLI Reference

### evrtools flags

| Flag | Description |
|------|-------------|
| `-mode` | Operation mode: `extract`, `build`, `inventory`, `analyze`, `diff` |
| `-data` | Path to `_data` directory containing manifests and packages |
| `-package` | Package name (e.g., `48037dc70b0ecab2`) |
| `-input` | Input directory (for `build`, `inventory`, `analyze`) |
| `-output` | Output directory (for `extract`, `build`) |
| `-preserve-groups` | Preserve frame grouping in extract output |
| `-force` | Allow non-empty output directory |
| `-verbose` | Show detailed output |
| `-manifest-a` | First manifest for diff mode |
| `-manifest-b` | Second manifest for diff mode |

### showtints flags

| Flag | Description |
|------|-------------|
| `-css` | Output tints as CSS custom properties |
| `-known` | Only show entries matching known tint hashes |
| `-nonzero` | Only show entries with non-zero color data |
| `-summary` | Only show summary statistics |
| `-raw` | Show raw hex bytes (default: true) |

### texconv commands

| Command | Description |
|---------|-------------|
| `decode <in.dds> <out.png>` | Decompress DDS to PNG |
| `encode <in.png> <out.dds>` | Compress PNG to DDS |
| `info <file.dds>` | Display texture info |
| `batch <mode> <indir> <outdir>` | Batch convert directory |

## Project Structure

```
evrFileTools/
├── cmd/
│   ├── evrtools/        # Package extract/build/analyze CLI
│   ├── showtints/       # Tint display and CSS export CLI
│   ├── symhash/         # Symbol hash computation CLI
│   └── texconv/         # DDS <-> PNG texture converter
├── pkg/
│   ├── archive/         # ZSTD archive format (header, reader, writer)
│   ├── manifest/        # EVR manifest parsing, package extraction, building
│   ├── hash/            # EVR symbol hash algorithms (CSymbol64, SNSMessage)
│   ├── naming/          # Type symbol mappings and asset name resolution
│   ├── texture/         # Texture metadata and DDS conversion
│   ├── audio/           # Audio format detection
│   ├── tint/            # Tint color parsing and CSS export
│   └── asset/           # Asset reference structures
├── docs/                # Format specifications and research
├── CONTRIBUTING.md      # Development guidelines, commit conventions
├── LICENSE              # MIT License
├── Makefile
└── go.mod
```

## Development

```bash
make build          # Build all CLI tools
make test           # Run all tests
make check          # Format, vet, and test
make bench          # Run benchmarks
make clean          # Remove build artifacts
make install        # Install all CLI tools via go install
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for commit conventions, versioning policy,
and code style guidelines.

## Documentation

- [ASSET_FORMATS.md](docs/ASSET_FORMATS.md) - Binary format specifications
- [IMPLEMENTATION_SUMMARY.md](docs/IMPLEMENTATION_SUMMARY.md) - Design overview
- [TEXTURE_FORMAT_VERIFIED.md](docs/TEXTURE_FORMAT_VERIFIED.md) - Texture format analysis
- [LEVEL_FORMAT_INVESTIGATION.md](docs/LEVEL_FORMAT_INVESTIGATION.md) - Level format research

## License

MIT License - see [LICENSE](LICENSE)
