// Package compressfs provides a transparent compression/decompression wrapper
// for any absfs.FileSystem implementation.
//
// It automatically compresses data when writing files and decompresses when
// reading, supporting multiple compression algorithms with configurable levels
// and smart content detection.
//
// # Features
//
//   - Transparent compression/decompression
//   - 5 compression algorithms: gzip, zstd, lz4, brotli, snappy
//   - Configurable compression levels
//   - Skip patterns for selective compression
//   - Automatic format detection
//   - Statistics tracking
//   - Empty file handling
//   - Large file support
//
// # Quick Start
//
//	import (
//	    "github.com/absfs/compressfs"
//	    "github.com/absfs/osfs"
//	)
//
//	// Create base filesystem
//	base := osfs.New("/data")
//
//	// Wrap with zstd compression (recommended)
//	fs, _ := compressfs.New(base, &compressfs.Config{
//	    Algorithm: compressfs.AlgorithmZstd,
//	    Level:     3,
//	})
//
//	// Write file - automatically compressed as data.txt.zst
//	f, _ := fs.Create("data.txt")
//	f.Write([]byte("Hello, compressed world!"))
//	f.Close()
//
//	// Read file - automatically decompressed
//	f, _ = fs.Open("data.txt")
//	data, _ := io.ReadAll(f)
//	f.Close()
//
// # Algorithm Selection Guide
//
// Choose based on your requirements:
//
//   - General Purpose: Zstd (level 3) - Best balance of speed and compression
//   - Maximum Speed: LZ4 or Snappy - Ultra-fast, moderate compression
//   - Maximum Compression: Brotli (level 9-11) - Best for static content
//   - Maximum Compatibility: Gzip - Universally supported
//   - CPU-Constrained: Snappy - Lowest CPU usage
//
// # Performance Characteristics
//
// Compression speeds (4KB files):
//   - LZ4:    642 MB/s  (fastest)
//   - Snappy:  77 MB/s  (very fast, low CPU)
//   - Gzip:    12 MB/s  (compatible)
//   - Brotli:   6 MB/s  (best compression)
//   - Zstd:     4 MB/s  (recommended - best ratio/speed balance)
//
// # Configuration Options
//
// Extension Handling:
//   - PreserveExtension: true  → file.txt becomes file.txt.zst
//   - StripExtension: true     → access via "file.txt" (transparent)
//
// Selective Compression:
//   - SkipPatterns: Skip files matching regex patterns
//   - MinSize: Only compress files above threshold
//   - AutoDetect: Detect and handle pre-compressed files
//
// See examples in the examples directory for more usage patterns.
package compressfs
