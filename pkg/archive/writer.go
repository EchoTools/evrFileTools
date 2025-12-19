package archive

import (
	"fmt"
	"io"

	"github.com/DataDog/zstd"
)

// Writer wraps an io.WriteSeeker to provide compression of archive data.
type Writer struct {
	dst     io.WriteSeeker
	zWriter *zstd.Writer
	header  *Header
	level   int
}

// WriterOption configures a Writer.
type WriterOption func(*Writer)

// WithCompressionLevel sets the compression level for the writer.
func WithCompressionLevel(level int) WriterOption {
	return func(w *Writer) {
		w.level = level
	}
}

// NewWriter creates a new archive writer that writes to dst.
// The uncompressedSize is the expected size of the uncompressed data.
func NewWriter(dst io.WriteSeeker, uncompressedSize uint64, opts ...WriterOption) (*Writer, error) {
	w := &Writer{
		dst:   dst,
		level: DefaultCompressionLevel,
		header: &Header{
			Magic:            Magic,
			HeaderLength:     16,
			Length:           uncompressedSize,
			CompressedLength: 0, // Will be updated after writing
		},
	}

	for _, opt := range opts {
		opt(w)
	}

	// Write placeholder header
	headerBytes, err := w.header.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("marshal header: %w", err)
	}
	if _, err := dst.Write(headerBytes); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}

	w.zWriter = zstd.NewWriterLevel(dst, w.level)
	return w, nil
}

// Write writes compressed data.
func (w *Writer) Write(p []byte) (n int, err error) {
	return w.zWriter.Write(p)
}

// Close finalizes the archive by updating the header with the compressed size.
func (w *Writer) Close() error {
	if err := w.zWriter.Close(); err != nil {
		return fmt.Errorf("close compressor: %w", err)
	}

	// Get current position to determine compressed size
	pos, err := w.dst.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("get position: %w", err)
	}

	// Update header with actual compressed size
	w.header.CompressedLength = uint64(pos) - uint64(w.header.Size())

	// Seek to beginning and rewrite header
	if _, err := w.dst.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek to start: %w", err)
	}

	headerBytes, err := w.header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal header: %w", err)
	}

	if _, err := w.dst.Write(headerBytes); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// Seek back to end
	if _, err := w.dst.Seek(pos, io.SeekStart); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}

	return nil
}

// Encode compresses data and writes it as an archive to dst.
func Encode(dst io.WriteSeeker, data []byte, opts ...WriterOption) error {
	w, err := NewWriter(dst, uint64(len(data)), opts...)
	if err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return w.Close()
}
