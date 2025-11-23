# compressfs

A transparent compression/decompression wrapper for the absfs filesystem abstraction layer.

## Overview

`compressfs` provides transparent compression and decompression capabilities for any `absfs.FileSystem` implementation. It automatically compresses data when writing files and decompresses when reading, supporting multiple compression algorithms with configurable levels and smart content detection.

## Purpose

- **Transparent Operations**: Applications interact with files normally while compression/decompression happens automatically
- **Storage Optimization**: Reduce storage costs and improve transfer efficiency
- **Algorithm Flexibility**: Choose the best compression algorithm for your use case
- **Integration**: Works seamlessly with other absfs wrappers (cachefs, s3fs, encryptfs, etc.)
- **Performance Tuning**: Configurable compression levels and skip patterns for optimal performance

## Supported Compression Algorithms

### gzip (compress/gzip)
- **Library**: Go standard library
- **Extension**: `.gz`
- **Compression Ratio**: Good (typically 60-70% reduction)
- **Speed**: Moderate
- **CPU Usage**: Moderate
- **Best For**: General-purpose compression, compatibility requirements
- **Levels**: 1-9 (plus BestSpeed=-1, BestCompression=-2, DefaultCompression=-3)

### zstd (github.com/klauspost/compress/zstd) - RECOMMENDED
- **Library**: `github.com/klauspost/compress/zstd`
- **Extension**: `.zst`
- **Compression Ratio**: Excellent (typically 65-75% reduction)
- **Speed**: Very Fast (3-5x faster than gzip)
- **CPU Usage**: Low to Moderate
- **Best For**: Most use cases, especially high-throughput scenarios
- **Levels**: 1-22 (SpeedFastest to SpeedBestCompression)
- **Features**: Dictionary compression, streaming, parallel compression

### lz4 (github.com/pierrec/lz4)
- **Library**: `github.com/pierrec/lz4`
- **Extension**: `.lz4`
- **Compression Ratio**: Moderate (typically 50-60% reduction)
- **Speed**: Extremely Fast (5-10x faster than gzip)
- **CPU Usage**: Very Low
- **Best For**: Real-time compression, latency-sensitive applications
- **Levels**: 1-16 (Fast mode) or compression level
- **Features**: Block independence, streaming

### brotli (github.com/andybalholm/brotli)
- **Library**: `github.com/andybalholm/brotli`
- **Extension**: `.br`
- **Compression Ratio**: Excellent (typically 70-80% reduction, better than gzip)
- **Speed**: Slow (compression), Fast (decompression)
- **CPU Usage**: High (compression), Low (decompression)
- **Best For**: Static content, write-once/read-many scenarios, web assets
- **Levels**: 0-11
- **Features**: Dictionary-based, optimized for text/HTML

### snappy (github.com/golang/snappy)
- **Library**: `github.com/golang/snappy`
- **Extension**: `.sz` or `.snappy`
- **Compression Ratio**: Low (typically 40-50% reduction)
- **Speed**: Extremely Fast (similar to lz4)
- **CPU Usage**: Very Low
- **Best For**: CPU-constrained environments, bulk data processing
- **Levels**: N/A (single algorithm, no levels)
- **Features**: No compression level tuning, consistent performance

## Performance Characteristics

| Algorithm | Compression Speed | Decompression Speed | Ratio | CPU Usage | Memory Usage |
|-----------|------------------|---------------------|-------|-----------|--------------|
| snappy    | Fastest          | Fastest             | Low   | Lowest    | Low          |
| lz4       | Fastest          | Fastest             | Medium| Very Low  | Low          |
| zstd      | Fast             | Fast                | High  | Moderate  | Moderate     |
| gzip      | Moderate         | Moderate            | Good  | Moderate  | Low          |
| brotli    | Slow             | Fast                | Highest| High     | Moderate     |

### Recommendation Matrix

**High-Throughput Systems**: zstd (level 3-6) or lz4
**Storage-Constrained**: brotli (level 9-11) or zstd (level 15+)
**CPU-Constrained**: snappy or lz4
**Latency-Sensitive**: lz4 or snappy
**General Purpose**: zstd (level 3)
**Maximum Compatibility**: gzip
**Static Content**: brotli (level 11)

## Architecture Design

### Core Components

```
┌─────────────────────────────────────────────────────────┐
│                    Application Layer                     │
│          (Reads/Writes via absfs.FileSystem)            │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│                    compressfs.FS                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Configuration                                     │  │
│  │  - Algorithm selection                            │  │
│  │  - Compression level                              │  │
│  │  - Skip patterns (regex)                          │  │
│  │  - Extension handling                             │  │
│  │  - Auto-detection settings                        │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │  File Operations Wrapper                          │  │
│  │  - OpenFile() → compressfs.File                   │  │
│  │  - Create() → compressfs.File                     │  │
│  │  - Stat() (transparent extension handling)        │  │
│  │  - ReadDir() (transparent extension handling)     │  │
│  └───────────────────────────────────────────────────┘  │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│                 compressfs.File                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Read Path (Decompression)                        │  │
│  │  1. Detect compression from extension/header      │  │
│  │  2. Create decompressor stream                    │  │
│  │  3. Stream decompressed data to caller           │  │
│  │  4. Handle buffer management                      │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Write Path (Compression)                         │  │
│  │  1. Check skip patterns                           │  │
│  │  2. Detect if already compressed (magic bytes)    │  │
│  │  3. Create compressor stream with selected algo   │  │
│  │  4. Buffer writes, compress on flush/close        │  │
│  │  5. Add appropriate extension                     │  │
│  └───────────────────────────────────────────────────┘  │
└────────────────────┬────────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────────┐
│              Underlying absfs.FileSystem                 │
│         (osfs, s3fs, memfs, cachefs, etc.)              │
└─────────────────────────────────────────────────────────┘
```

### Streaming Architecture

```go
// Write path with streaming compression
Application Write → Buffer → Compressor Stream → Underlying File
                     ↓
              (chunk on buffer full)

// Read path with streaming decompression
Underlying File → Decompressor Stream → Buffer → Application Read
                        ↓
                 (stream on demand)
```

## Configuration Options

### Algorithm Selection

```go
type Algorithm string

const (
    AlgorithmGzip   Algorithm = "gzip"
    AlgorithmZstd   Algorithm = "zstd"    // Recommended
    AlgorithmLZ4    Algorithm = "lz4"
    AlgorithmBrotli Algorithm = "brotli"
    AlgorithmSnappy Algorithm = "snappy"
    AlgorithmAuto   Algorithm = "auto"    // Detect from extension
)
```

### Compression Levels

```go
type Config struct {
    // Algorithm to use for compression (default: zstd)
    Algorithm Algorithm

    // Compression level (algorithm-specific)
    // gzip: 1-9 (6 default)
    // zstd: 1-22 (3 default)
    // lz4: 1-16 (1 default)
    // brotli: 0-11 (6 default)
    // snappy: ignored (no levels)
    Level int

    // Skip patterns - regex patterns for files to skip compression
    // Examples: []string{`\.jpg$`, `\.png$`, `\.mp4$`, `\.zip$`}
    SkipPatterns []string

    // Auto-detect already compressed content by magic bytes
    AutoDetect bool // default: true

    // Preserve original extension (e.g., file.txt.gz vs file.gz)
    PreserveExtension bool // default: true

    // Strip compression extensions on reads (transparent)
    StripExtension bool // default: true

    // Buffer size for streaming (default: 64KB)
    BufferSize int

    // Minimum file size to compress (skip smaller files)
    MinSize int64 // default: 0 (compress all)
}
```

### Extension Handling

| Mode | Write "data.txt" | Read Request | Actual File | Behavior |
|------|------------------|--------------|-------------|----------|
| Preserve=true, Strip=true | data.txt.zst | data.txt | data.txt.zst | Transparent |
| Preserve=true, Strip=false | data.txt.zst | data.txt.zst | data.txt.zst | Explicit |
| Preserve=false, Strip=true | data.zst | data.txt | data.zst | Transparent |
| Preserve=false, Strip=false | data.zst | data.zst | data.zst | Explicit |

## Implementation Phases

### Phase 1: Core Infrastructure
- Repository setup and module initialization
- Define core interfaces and types
- Implement basic FS wrapper structure
- Configuration system
- Extension mapping and detection

### Phase 2: Algorithm Integration
- Gzip implementation (standard library)
- Zstd implementation and benchmarking
- LZ4 implementation and benchmarking
- Snappy implementation and benchmarking
- Brotli implementation and benchmarking

### Phase 3: Streaming Implementation
- Streaming compression writer
- Streaming decompression reader
- Buffer management and pooling
- Chunk handling and flush semantics
- Seek support (where possible)

### Phase 4: Smart Features
- Content-type detection (magic bytes)
- Skip patterns (regex matching)
- Auto-detection of compressed content
- Metadata preservation
- Compression statistics/metrics

### Phase 5: Advanced Features
- Concurrent compression/decompression
- Dictionary support (zstd)
- Compression level auto-tuning
- File-specific algorithm selection
- Transparent re-compression (algorithm migration)

### Phase 6: Testing & Documentation
- Unit tests for all algorithms
- Integration tests with other absfs wrappers
- Benchmark suite (all algorithms)
- Performance optimization
- API documentation and examples
- Migration guides

## Usage Examples

### Basic Usage - Recommended (Zstd)

```go
package main

import (
    "github.com/absfs/absfs"
    "github.com/absfs/compressfs"
    "github.com/absfs/osfs"
)

func main() {
    // Create base filesystem
    base := osfs.New("/data")

    // Wrap with compression (zstd, level 3)
    fs := compressfs.New(base, &compressfs.Config{
        Algorithm: compressfs.AlgorithmZstd,
        Level:     3, // Fast, good compression
    })

    // Write file - automatically compressed as data.txt.zst
    f, _ := fs.Create("/data.txt")
    f.Write([]byte("Hello, compressed world!"))
    f.Close()

    // Read file - automatically decompressed
    f, _ = fs.Open("/data.txt")
    data := make([]byte, 100)
    n, _ := f.Read(data)
    println(string(data[:n])) // "Hello, compressed world!"
}
```

### High-Performance LZ4

```go
// Ultra-low latency compression
fs := compressfs.New(base, &compressfs.Config{
    Algorithm: compressfs.AlgorithmLZ4,
    Level:     1, // Fastest
    MinSize:   1024, // Only compress files > 1KB
})
```

### Maximum Compression (Brotli)

```go
// Static content, write-once/read-many
fs := compressfs.New(base, &compressfs.Config{
    Algorithm: compressfs.AlgorithmBrotli,
    Level:     11, // Maximum compression
    SkipPatterns: []string{
        `\.jpg$`, `\.png$`, `\.gif$`, // Already compressed
        `\.mp4$`, `\.zip$`, `\.gz$`,
    },
})
```

### Multi-Algorithm with Auto-Detection

```go
// Decompress any format, compress with zstd
fs := compressfs.New(base, &compressfs.Config{
    Algorithm:  compressfs.AlgorithmZstd, // For writes
    Level:      6,
    AutoDetect: true, // Detect any format on reads
})

// Reads any of: file.gz, file.zst, file.lz4, file.br, file.sz
data, _ := fs.ReadFile("/file.txt")
```

### Selective Compression

```go
// Skip already-compressed and media files
fs := compressfs.New(base, &compressfs.Config{
    Algorithm: compressfs.AlgorithmZstd,
    Level:     3,
    SkipPatterns: []string{
        `\.(jpg|jpeg|png|gif|webp)$`,      // Images
        `\.(mp4|mkv|avi|mov)$`,            // Videos
        `\.(mp3|flac|ogg|m4a)$`,           // Audio
        `\.(zip|gz|bz2|xz|7z|rar)$`,       // Archives
        `\.(zst|lz4|br|sz)$`,              // Already compressed
    },
    AutoDetect: true,
    MinSize:    512, // Don't compress tiny files
})
```

### Integration with Other Wrappers

```go
// Stack multiple wrappers: S3 → Encryption → Compression → Cache
s3 := s3fs.New("my-bucket", s3Config)

encrypted := encryptfs.New(s3, &encryptfs.Config{
    Key: secretKey,
})

compressed := compressfs.New(encrypted, &compressfs.Config{
    Algorithm: compressfs.AlgorithmZstd,
    Level:     6,
})

cached := cachefs.New(compressed, &cachefs.Config{
    MaxSize: 1024 * 1024 * 1024, // 1GB cache
})

// Use cached - benefits from all layers
data, _ := cached.ReadFile("/document.txt")
// Flow: Cache miss → Decompress → Decrypt → Download from S3
```

### Performance Tuning Example

```go
// Different algorithms for different paths
func NewTunedFS(base absfs.FileSystem) absfs.FileSystem {
    return compressfs.New(base, &compressfs.Config{
        Algorithm: compressfs.AlgorithmZstd,
        Level:     3,
        BufferSize: 256 * 1024, // 256KB buffers
        SkipPatterns: []string{
            // Logs: fast compression (lz4 preferred, but using skip here)
            `^/logs/.*\.log$`,
            // Media: skip
            `\.(jpg|png|mp4|mp3)$`,
        },
    })
}
```

## API Design

### Core Types

```go
// FS wraps an absfs.FileSystem with compression
type FS struct {
    base   absfs.FileSystem
    config *Config
    skip   *regexp.Regexp // Compiled skip patterns
}

// New creates a new compressed filesystem wrapper
func New(base absfs.FileSystem, config *Config) *FS

// File wraps an absfs.File with compression/decompression
type File struct {
    base       absfs.File
    mode       int // O_RDONLY, O_WRONLY, etc.

    // Compression state (write mode)
    compressor io.WriteCloser
    buffer     *bytes.Buffer

    // Decompression state (read mode)
    decompressor io.ReadCloser

    // Metadata
    algorithm Algorithm
    originalName string
    compressedName string
}
```

### Filesystem Methods

```go
// Standard absfs.FileSystem interface
func (fs *FS) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error)
func (fs *FS) Create(name string) (absfs.File, error)
func (fs *FS) Open(name string) (absfs.File, error)
func (fs *FS) Stat(name string) (os.FileInfo, error)
func (fs *FS) Remove(name string) error
func (fs *FS) Mkdir(name string, perm os.FileMode) error
func (fs *FS) ReadDir(name string) ([]os.FileInfo, error)

// Compression-specific methods
func (fs *FS) SetAlgorithm(algo Algorithm) error
func (fs *FS) SetLevel(level int) error
func (fs *FS) GetStats() *Stats
func (fs *FS) ResetStats()
```

### File Methods

```go
// Standard absfs.File interface
func (f *File) Read(p []byte) (n int, err error)
func (f *File) Write(p []byte) (n int, err error)
func (f *File) Close() error
func (f *File) Stat() (os.FileInfo, error)
func (f *File) Sync() error

// Limited seek support (depends on algorithm)
func (f *File) Seek(offset int64, whence int) (int64, error)

// Compression-specific methods
func (f *File) Algorithm() Algorithm
func (f *File) CompressionRatio() float64
func (f *File) OriginalSize() int64
func (f *File) CompressedSize() int64
```

### Statistics

```go
type Stats struct {
    FilesCompressed   int64
    FilesDecompressed int64
    FilesSkipped      int64

    BytesRead         int64
    BytesWritten      int64
    BytesCompressed   int64
    BytesDecompressed int64

    TotalCompressionRatio   float64
    TotalDecompressionRatio float64

    AlgorithmCounts map[Algorithm]int64
}
```

### Magic Bytes Detection

```go
var magicBytes = map[Algorithm][]byte{
    AlgorithmGzip:   {0x1f, 0x8b},                     // gzip
    AlgorithmZstd:   {0x28, 0xb5, 0x2f, 0xfd},         // zstd
    AlgorithmLZ4:    {0x04, 0x22, 0x4d, 0x18},         // lz4
    AlgorithmBrotli: {0xce, 0xb2, 0xcf, 0x81},         // brotli (partial)
    AlgorithmSnappy: {0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50, 0x70, 0x59}, // snappy
}

func DetectAlgorithm(r io.Reader) (Algorithm, error)
```

## Testing Strategy

### Unit Tests

```go
// Algorithm-specific tests
func TestGzipCompression(t *testing.T)
func TestZstdCompression(t *testing.T)
func TestLZ4Compression(t *testing.T)
func TestBrotliCompression(t *testing.T)
func TestSnappyCompression(t *testing.T)

// Feature tests
func TestAutoDetection(t *testing.T)
func TestSkipPatterns(t *testing.T)
func TestExtensionHandling(t *testing.T)
func TestStreamingLargeFiles(t *testing.T)
func TestConcurrentAccess(t *testing.T)
func TestMetadataPreservation(t *testing.T)
```

### Integration Tests

```go
// Wrapper integration
func TestWithMemFS(t *testing.T)
func TestWithOsFS(t *testing.T)
func TestWithS3FS(t *testing.T)
func TestWithCacheFS(t *testing.T)
func TestWithEncryptFS(t *testing.T)

// Stacked wrappers
func TestMultiLayerStack(t *testing.T)
```

### Benchmarks

```go
// Algorithm comparison
func BenchmarkGzipWrite(b *testing.B)
func BenchmarkZstdWrite(b *testing.B)
func BenchmarkLZ4Write(b *testing.B)
func BenchmarkBrotliWrite(b *testing.B)
func BenchmarkSnappyWrite(b *testing.B)

// Level comparison (per algorithm)
func BenchmarkZstdLevel1(b *testing.B)
func BenchmarkZstdLevel3(b *testing.B)
func BenchmarkZstdLevel6(b *testing.B)
func BenchmarkZstdLevel9(b *testing.B)

// File sizes
func BenchmarkSmallFiles(b *testing.B)  // < 1KB
func BenchmarkMediumFiles(b *testing.B) // 1KB - 1MB
func BenchmarkLargeFiles(b *testing.B)  // > 1MB

// Content types
func BenchmarkTextCompression(b *testing.B)
func BenchmarkJSONCompression(b *testing.B)
func BenchmarkBinaryCompression(b *testing.B)
```

### Test Data

```go
// Various content types for realistic testing
var testData = []struct {
    name string
    data []byte
}{
    {"text", generateText(1024 * 1024)},           // Highly compressible
    {"json", generateJSON(1024 * 1024)},           // Structured data
    {"random", generateRandom(1024 * 1024)},       // Incompressible
    {"repetitive", generateRepetitive(1024 * 1024)}, // Very compressible
    {"html", generateHTML(1024 * 1024)},           // Web content
}
```

## Error Handling

```go
var (
    ErrUnsupportedAlgorithm = errors.New("compressfs: unsupported compression algorithm")
    ErrInvalidLevel         = errors.New("compressfs: invalid compression level")
    ErrSeekNotSupported     = errors.New("compressfs: seek not supported for compressed files")
    ErrAlreadyCompressed    = errors.New("compressfs: file already compressed")
    ErrCorruptedData        = errors.New("compressfs: corrupted compressed data")
)
```

## File Extension Mapping

```go
var extensionMap = map[Algorithm]string{
    AlgorithmGzip:   ".gz",
    AlgorithmZstd:   ".zst",
    AlgorithmLZ4:    ".lz4",
    AlgorithmBrotli: ".br",
    AlgorithmSnappy: ".sz",
}

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
```

## Metadata Preservation

Compression should preserve:
- File modification time
- File permissions
- Extended attributes (where supported)
- Original filename (in compressed stream metadata where possible)

## Dependencies

```
github.com/absfs/absfs            - Core filesystem abstraction
github.com/klauspost/compress/zstd - Zstandard compression
github.com/pierrec/lz4            - LZ4 compression
github.com/andybalholm/brotli     - Brotli compression
github.com/golang/snappy          - Snappy compression
```

## License

MIT License - See LICENSE file for details

## Contributing

Contributions welcome! Please ensure:
- All tests pass
- Benchmarks show no regression
- Documentation is updated
- Code follows Go best practices

## References

- [Zstandard RFC](https://datatracker.ietf.org/doc/html/rfc8878)
- [LZ4 Specification](https://github.com/lz4/lz4/blob/dev/doc/lz4_Frame_format.md)
- [Brotli RFC](https://datatracker.ietf.org/doc/html/rfc7932)
- [Snappy Format](https://github.com/google/snappy/blob/main/format_description.txt)
- [GZIP RFC](https://datatracker.ietf.org/doc/html/rfc1952)
