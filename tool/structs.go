package tool

import "encoding/binary"

type ManifestHeader struct {
	PackageCount  uint32
	Unk1          uint32 // ? - 524288 on latest builds
	Unk2          uint64 // ? - 0 on latest builds
	FrameContents ManifestSection
	_             [16]byte // padding
	SomeStructure ManifestSection
	_             [16]byte // padding
	Frames        ManifestSection
}

func (m *ManifestHeader) Len() int {
	return int(binary.Size(m))
}

type ManifestSection struct {
	Length        uint64 // total byte length of entire section
	Unk1          uint64 // ? 0 on latest builds
	Unk2          uint64 // ? 4294967296 on latest builds
	ElementLength uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
	Count         uint64 // number of elements, can differ from ElementCount?
	ElementCount  uint64 // number of elements
}

type FrameContent struct {
	TypeSymbol    int64
	FileSymbol    int64
	FileIndex     uint32
	DataOffset    uint32
	Length        uint32
	SomeAlignment uint32
}

type SomeStructureEntry struct {
	TypeSymbol int64
	FileSymbol int64
	Unk1       int64
	Unk2       int64
	Unk3       int64
}

type FrameEntry struct {
	CurrentPackageIndex uint32
	CurrentOffset       uint32
	CompressedSize      uint32
	DecompressedSize    uint32
}

type FrameContents struct { // 32 bytes
	T             int64  // Probably filetype
	FileSymbol    int64  // Symbol for file
	FileIndex     uint32 // Frame[FileIndex] = file containing this entry
	DataOffset    uint32 // Byte offset for beginning of wanted data in given file
	Size          uint32 // Size of file
	SomeAlignment uint32 // file divisible by this (can this just be set to 1??) - yes
}

type SomeStructure struct { // 40 bytes
	T          int64 // seems to be the same as AssetType
	FileSymbol int64 // filename symbol
	Unk1       int64 // ? - game still launches when set to 0
	Unk2       int64 // ? - game still launches when set to 0
	AssetType  int64 // ? - game still launches when set to 0
}

type Frame struct { // 16 bytes
	Index          uint32 // the package index
	Offset         uint32 // the package byte offset
	CompressedSize uint32 // compressed size of file
	Length         uint32 // decompressed size of file
}
