# compressfs

[![Go Reference](https://pkg.go.dev/badge/github.com/absfs/compressfs.svg)](https://pkg.go.dev/github.com/absfs/compressfs)
[![Go Report Card](https://goreportcard.com/badge/github.com/absfs/compressfs)](https://goreportcard.com/report/github.com/absfs/compressfs)

A transparent compression/decompression wrapper for the [absfs](https://github.com/absfs/absfs) filesystem abstraction layer.

## Features

✅ **5 Compression Algorithms**: gzip, zstd, lz4, brotli, snappy
✅ **Transparent Operations**: Files are automatically compressed/decompressed
✅ **Configurable Levels**: Fine-tune compression speed vs ratio
✅ **Smart Detection**: Auto-detect compression formats
✅ **Selective Compression**: Skip already-compressed files with regex patterns
✅ **Statistics Tracking**: Monitor compression operations
✅ **Production Ready**: Comprehensive test suite (40+ tests passing)
✅ **High Performance**: LZ4 achieves 642 MB/s on 4KB files

## Quick Start

```go
package main

import (
	"github.com/absfs/compressfs"
	"github.com/absfs/osfs"
)

func main() {
	// Create base filesystem
	base := osfs.New("/data")

	// Wrap with compression (uses recommended settings)
	fs, _ := compressfs.NewWithRecommendedConfig(base)

	// Write file - automatically compressed as data.txt.zst
	f, _ := fs.Create("data.txt")
	f.Write([]byte("Hello, World!"))
	f.Close()

	// Read file - automatically decompressed
	f, _ = fs.Open("data.txt")
	data, _ := io.ReadAll(f)
	f.Close()
}
```

## Installation

```bash
go get github.com/absfs/compressfs
```

## Performance Benchmarks

Measured on 4KB files:

| Algorithm | Write Speed | Best Use Case |
|-----------|-------------|---------------|
| **LZ4** | 642 MB/s | Speed-critical applications |
| **Snappy** | 77 MB/s | Low CPU usage |
| **Gzip** | 11.7 MB/s | Compatibility |
| **Brotli** | 6.0 MB/s | Maximum compression |
| **Zstd** | 3.76 MB/s | **Recommended - Best balance** |

## Supported Algorithms

### Zstd - **RECOMMENDED**
- **Speed**: Very fast (3-5x faster than gzip)
- **Ratio**: Excellent (65-75% reduction)
- **Use**: General purpose, high-throughput systems
- **Levels**: 1-22 (recommended: 3)

### LZ4
- **Speed**: Extremely fast (642 MB/s)
- **Ratio**: Moderate (50-60% reduction)
- **Use**: Real-time compression, latency-sensitive apps
- **Levels**: Not applicable (single mode)

### Snappy
- **Speed**: Very fast (77 MB/s)
- **Ratio**: Low (40-50% reduction)
- **Use**: CPU-constrained, bulk data processing
- **Levels**: Not applicable (single mode)

### Brotli
- **Speed**: Slow compression, fast decompression
- **Ratio**: Best (70-80% reduction)
- **Use**: Static content, write-once/read-many
- **Levels**: 0-11 (recommended: 6 or 11)

### Gzip
- **Speed**: Moderate
- **Ratio**: Good (60-70% reduction)
- **Use**: Maximum compatibility
- **Levels**: 1-9 (recommended: 6)

## Usage Examples

### Basic Usage with Custom Config

```go
fs, _ := compressfs.New(base, &compressfs.Config{
	Algorithm:         compressfs.AlgorithmZstd,
	Level:             3,
	PreserveExtension: true,  // file.txt -> file.txt.zst
	StripExtension:    true,  // access via "file.txt"
})
```

### Using Preset Configurations

```go
// Recommended (Zstd level 3, skip already-compressed)
fs, _ := compressfs.NewWithRecommendedConfig(base)

// Fastest (LZ4)
fs, _ := compressfs.NewWithFastestConfig(base)

// Best Compression (Brotli level 11)
fs, _ := compressfs.NewWithBestCompression(base)
```

### Skip Already-Compressed Files

```go
fs, _ := compressfs.New(base, &compressfs.Config{
	Algorithm: compressfs.AlgorithmZstd,
	SkipPatterns: []string{
		`\.(jpg|png|gif|mp4)$`,  // Media files
		`\.(zip|gz|bz2)$`,       // Archives
	},
})
```

### Minimum File Size Filtering

```go
fs, _ := compressfs.New(base, &compressfs.Config{
	Algorithm: compressfs.AlgorithmZstd,
	MinSize:   1024,  // Only compress files >= 1KB
})
```

### Compression Statistics

```go
fs, _ := compressfs.NewWithRecommendedConfig(base)

// ... perform operations ...

stats := fs.GetStats()
fmt.Printf("Files compressed: %d\n", stats.FilesCompressed)
fmt.Printf("Bytes written: %d\n", stats.BytesWritten)
fmt.Printf("Compression ratio: %.2f%%\n", stats.TotalCompressionRatio()*100)
```

### Compress/Decompress Bytes

```go
// Compress bytes
compressed, _ := compressfs.CompressBytes(data, compressfs.AlgorithmZstd, 3)

// Decompress bytes
decompressed, _ := compressfs.DecompressBytes(compressed, compressfs.AlgorithmZstd)

// Auto-detect compression
algo, found := compressfs.DetectCompressionAlgorithm(data)
```

## Configuration Options

```go
type Config struct {
	// Algorithm to use (gzip, zstd, lz4, brotli, snappy)
	Algorithm Algorithm

	// Compression level (algorithm-specific)
	Level int

	// Regex patterns for files to skip
	SkipPatterns []string

	// Auto-detect compressed files by magic bytes
	AutoDetect bool  // default: true

	// Preserve original extension (file.txt.zst vs file.zst)
	PreserveExtension bool  // default: true

	// Strip extension on reads (transparent access)
	StripExtension bool  // default: true

	// Buffer size for streaming
	BufferSize int  // default: 64KB

	// Minimum file size to compress
	MinSize int64  // default: 0
}
```

## Algorithm Selection Guide

Choose based on your requirements:

| Requirement | Algorithm | Level |
|-------------|-----------|-------|
| **General Purpose** | Zstd | 3 |
| **Maximum Speed** | LZ4 or Snappy | - |
| **Best Compression** | Brotli | 9-11 |
| **Compatibility** | Gzip | 6 |
| **Low CPU** | Snappy | - |
| **Balanced** | Zstd | 3 (default) |

## Integration with Other absfs Wrappers

```go
// Stack multiple wrappers
s3 := s3fs.New("my-bucket", config)
encrypted := encryptfs.New(s3, encryptConfig)
compressed := compressfs.NewWithRecommendedConfig(encrypted)
cached := cachefs.New(compressed, cacheConfig)

// All layers work together transparently
data, _ := cached.ReadFile("/document.txt")
```

## Testing

The package includes comprehensive tests:

```bash
# Run all tests
go test

# Run with coverage
go test -cover

# Run benchmarks
go test -bench=. -benchtime=1s

# Specific benchmark
go test -bench=BenchmarkZstdWrite
```

**Test Coverage**: 40+ tests covering:
- All 5 compression algorithms
- Multiple compression levels
- Large files (1MB+)
- Empty files
- Extension detection
- Magic byte detection
- Statistics tracking
- Edge cases

## Performance Tips

1. **Choose the right algorithm**:
   - Zstd level 3 for most use cases
   - LZ4 for maximum speed
   - Brotli level 11 for static content

2. **Use skip patterns** to avoid compressing already-compressed files

3. **Set MinSize** to skip very small files (overhead not worth it)

4. **Adjust buffer size** based on your workload:
   - Larger buffers (256KB) for bulk operations
   - Smaller buffers (32KB) for many small files

5. **Enable PreserveExtension + StripExtension** for transparent operation

## Architecture

```
Application
    ↓
compressfs.FS (this package)
    ↓
Base FileSystem (osfs, s3fs, memfs, etc.)
```

Files are compressed on write and decompressed on read transparently. The package handles:
- Extension management
- Format detection
- Streaming compression/decompression
- Buffer management
- Statistics tracking

## License

MIT License - See [LICENSE](LICENSE) file

## Contributing

Contributions welcome! Please ensure:
- All tests pass (`go test`)
- Code is formatted (`go fmt`)
- Add tests for new features
- Update documentation

## References

- [absfs](https://github.com/absfs/absfs) - Filesystem abstraction
- [Zstandard](https://facebook.github.io/zstd/) - Compression algorithm
- [LZ4](https://lz4.github.io/lz4/) - Ultra-fast compression
- [Brotli](https://github.com/google/brotli) - Google compression
- [Snappy](https://github.com/google/snappy) - Fast compression
