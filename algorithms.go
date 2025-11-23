package compressfs

import (
	"compress/gzip"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// createCompressor creates a compressor for the specified algorithm
func createCompressor(algo Algorithm, w io.Writer, level int) (io.WriteCloser, error) {
	switch algo {
	case AlgorithmGzip:
		return createGzipCompressor(w, level)
	case AlgorithmZstd:
		return createZstdCompressor(w, level)
	case AlgorithmLZ4:
		return createLZ4Compressor(w, level)
	case AlgorithmBrotli:
		return createBrotliCompressor(w, level)
	case AlgorithmSnappy:
		return createSnappyCompressor(w, level)
	default:
		return nil, ErrUnsupportedAlgorithm
	}
}

// createDecompressor creates a decompressor for the specified algorithm
func createDecompressor(algo Algorithm, r io.Reader, level int) (io.ReadCloser, error) {
	switch algo {
	case AlgorithmGzip:
		return createGzipDecompressor(r)
	case AlgorithmZstd:
		return createZstdDecompressor(r)
	case AlgorithmLZ4:
		return createLZ4Decompressor(r)
	case AlgorithmBrotli:
		return createBrotliDecompressor(r)
	case AlgorithmSnappy:
		return createSnappyDecompressor(r)
	default:
		return nil, ErrUnsupportedAlgorithm
	}
}

// Gzip implementation using standard library
func createGzipCompressor(w io.Writer, level int) (io.WriteCloser, error) {
	if level == 0 {
		level = gzip.DefaultCompression
	}
	return gzip.NewWriterLevel(w, level)
}

func createGzipDecompressor(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}

// Zstd implementation using github.com/klauspost/compress/zstd
func createZstdCompressor(w io.Writer, level int) (io.WriteCloser, error) {
	// Default to level 3 if not specified
	if level == 0 {
		level = 3
	}

	// Map level to zstd encoder level
	var encoderLevel zstd.EncoderLevel
	switch {
	case level <= 0:
		encoderLevel = zstd.SpeedFastest
	case level <= 3:
		encoderLevel = zstd.SpeedDefault
	case level <= 6:
		encoderLevel = zstd.SpeedBetterCompression
	default:
		encoderLevel = zstd.SpeedBestCompression
	}

	return zstd.NewWriter(w, zstd.WithEncoderLevel(encoderLevel))
}

func createZstdDecompressor(r io.Reader) (io.ReadCloser, error) {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &zstdReadCloser{Decoder: decoder}, nil
}

// zstdReadCloser wraps zstd.Decoder to implement io.ReadCloser
type zstdReadCloser struct {
	*zstd.Decoder
}

func (r *zstdReadCloser) Close() error {
	r.Decoder.Close()
	return nil
}

// LZ4 implementation using github.com/pierrec/lz4
func createLZ4Compressor(w io.Writer, level int) (io.WriteCloser, error) {
	// Create LZ4 writer
	// LZ4 in this library uses default compression settings
	// The pierrec/lz4/v4 library doesn't support traditional compression levels
	return lz4.NewWriter(w), nil
}

func createLZ4Decompressor(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(lz4.NewReader(r)), nil
}

// Brotli implementation using github.com/andybalholm/brotli
func createBrotliCompressor(w io.Writer, level int) (io.WriteCloser, error) {
	// Default to level 6 if not specified
	if level == 0 {
		level = 6
	}

	// Brotli supports levels 0-11
	if level < 0 {
		level = 0
	} else if level > 11 {
		level = 11
	}

	return &brotliWriteCloser{
		Writer: brotli.NewWriterLevel(w, level),
	}, nil
}

func createBrotliDecompressor(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(brotli.NewReader(r)), nil
}

// brotliWriteCloser wraps brotli.Writer to implement io.WriteCloser
type brotliWriteCloser struct {
	*brotli.Writer
}

func (w *brotliWriteCloser) Close() error {
	return w.Writer.Close()
}

// Snappy implementation using github.com/golang/snappy
// Note: Snappy does not support compression levels
func createSnappyCompressor(w io.Writer, level int) (io.WriteCloser, error) {
	// Snappy uses framed format for streaming
	return snappy.NewBufferedWriter(w), nil
}

func createSnappyDecompressor(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(snappy.NewReader(r)), nil
}
