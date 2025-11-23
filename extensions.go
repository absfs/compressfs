package compressfs

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
)

// Extension mapping
var extensionMap = map[Algorithm]string{
	AlgorithmGzip:   ".gz",
	AlgorithmZstd:   ".zst",
	AlgorithmLZ4:    ".lz4",
	AlgorithmBrotli: ".br",
	AlgorithmSnappy: ".sz",
}

// Reverse extension mapping (extension -> algorithm)
var reverseExtensionMap = map[string]Algorithm{
	".gz":     AlgorithmGzip,
	".gzip":   AlgorithmGzip,
	".zst":    AlgorithmZstd,
	".zstd":   AlgorithmZstd,
	".lz4":    AlgorithmLZ4,
	".br":     AlgorithmBrotli,
	".sz":     AlgorithmSnappy,
	".snappy": AlgorithmSnappy,
}

// Magic bytes for compression format detection
var magicBytes = map[Algorithm][]byte{
	AlgorithmGzip:   {0x1f, 0x8b},                                         // gzip
	AlgorithmZstd:   {0x28, 0xb5, 0x2f, 0xfd},                             // zstd
	AlgorithmLZ4:    {0x04, 0x22, 0x4d, 0x18},                             // lz4
	AlgorithmBrotli: {0xce, 0xb2, 0xcf, 0x81},                             // brotli (partial, first frame)
	AlgorithmSnappy: {0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50}, // snappy framed
}

// GetExtension returns the file extension for an algorithm
func GetExtension(algo Algorithm) string {
	if ext, ok := extensionMap[algo]; ok {
		return ext
	}
	return ""
}

// DetectAlgorithmFromExtension detects the algorithm from file extension
func DetectAlgorithmFromExtension(name string) (Algorithm, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	if algo, ok := reverseExtensionMap[ext]; ok {
		return algo, true
	}
	return "", false
}

// DetectAlgorithm detects compression algorithm from magic bytes
func DetectAlgorithm(r io.Reader) (Algorithm, error) {
	// Read enough bytes to detect any format
	buf := make([]byte, 10)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", err
	}
	buf = buf[:n]

	// Check each algorithm's magic bytes
	for algo, magic := range magicBytes {
		if len(buf) >= len(magic) && bytes.Equal(buf[:len(magic)], magic) {
			return algo, nil
		}
	}

	return "", nil // No compression detected
}

// AddExtension adds the compression extension to a filename
func AddExtension(name string, algo Algorithm, preserveOriginal bool) string {
	ext := GetExtension(algo)
	if ext == "" {
		return name
	}

	if preserveOriginal {
		return name + ext
	}

	// Replace original extension
	base := strings.TrimSuffix(name, filepath.Ext(name))
	return base + ext
}

// StripExtension removes compression extension from filename
func StripExtension(name string) (string, Algorithm, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	if algo, ok := reverseExtensionMap[ext]; ok {
		// Remove the compression extension
		stripped := strings.TrimSuffix(name, ext)
		return stripped, algo, true
	}
	return name, "", false
}

// HasCompressionExtension checks if filename has a compression extension
func HasCompressionExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := reverseExtensionMap[ext]
	return ok
}

// IsCompressed checks if data appears to be compressed based on magic bytes
func IsCompressed(data []byte) (Algorithm, bool) {
	for algo, magic := range magicBytes {
		if len(data) >= len(magic) && bytes.Equal(data[:len(magic)], magic) {
			return algo, true
		}
	}
	return "", false
}
