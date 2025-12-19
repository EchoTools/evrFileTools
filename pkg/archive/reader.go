package archive

import (
	"fmt"
	"io"

	"github.com/DataDog/zstd"
)

const (
	// DefaultCompressionLevel is the default compression level for encoding.
	DefaultCompressionLevel = zstd.BestSpeed
)

// Reader wraps an io.ReadSeeker to provide decompression of archive data.
type Reader struct {
	header    *Header
	zReader   io.ReadCloser
	headerBuf [HeaderSize]byte // Reusable buffer for header decoding
}

// NewReader creates a new archive reader from the given source.
// It reads and validates the header, then returns a reader for the decompressed content.
func NewReader(r io.ReadSeeker) (*Reader, error) {
	reader := &Reader{
		header: &Header{},
	}

	if _, err := r.Read(reader.headerBuf[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	if err := reader.header.UnmarshalBinary(reader.headerBuf[:]); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	reader.zReader = zstd.NewReader(r)
	return reader, nil
}

// Header returns the archive header.
func (r *Reader) Header() *Header {
	return r.header
}

// Read reads decompressed data into p.
func (r *Reader) Read(p []byte) (n int, err error) {
	return r.zReader.Read(p)
}

// Close closes the reader.
func (r *Reader) Close() error {
	return r.zReader.Close()
}

// Length returns the uncompressed data length.
func (r *Reader) Length() int {
	return int(r.header.Length)
}

// CompressedLength returns the compressed data length.
func (r *Reader) CompressedLength() int {
	return int(r.header.CompressedLength)
}

// ReadAll reads the entire decompressed content from an archive.
func ReadAll(r io.ReadSeeker) ([]byte, error) {
	reader, err := NewReader(r)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data := make([]byte, reader.Length())
	n, err := io.ReadFull(reader, data)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}
	if n != reader.Length() {
		return nil, fmt.Errorf("incomplete read: expected %d, got %d", reader.Length(), n)
	}

	return data, nil
}
