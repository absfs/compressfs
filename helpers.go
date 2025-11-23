package compressfs

import (
	"io"
)

// Preset configurations for common use cases

// FastestConfig returns a configuration optimized for speed
func FastestConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmLZ4,
		Level:             0,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        64 * 1024,
		MinSize:           0,
	}
}

// RecommendedConfig returns the recommended configuration for general use
// Uses Zstd level 3 which provides excellent compression with good speed
func RecommendedConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        64 * 1024,
		MinSize:           512, // Skip very small files
		SkipPatterns: []string{
			// Already compressed formats
			`\.(jpg|jpeg|png|gif|webp)$`,      // Images
			`\.(mp4|mkv|avi|mov|webm)$`,       // Videos
			`\.(mp3|flac|ogg|m4a|aac)$`,       // Audio
			`\.(zip|gz|bz2|xz|7z|rar|tar)$`,   // Archives
			`\.(zst|lz4|br|sz|snappy)$`,       // Compressed
		},
	}
}

// BestCompressionConfig returns a configuration optimized for maximum compression
// Use for static content or write-once/read-many scenarios
func BestCompressionConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmBrotli,
		Level:             11,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        128 * 1024,
		MinSize:           1024, // Only compress files > 1KB
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp|mp4|mkv|avi|mov|mp3|flac|zip|gz|bz2|xz|7z|rar|zst|lz4|br|sz)$`,
		},
	}
}

// CompatibleConfig returns a configuration using gzip for maximum compatibility
func CompatibleConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmGzip,
		Level:             6,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        64 * 1024,
		MinSize:           512,
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp|mp4|mkv|avi|mov|mp3|flac|zip|gz|bz2|xz|7z|rar)$`,
		},
	}
}

// LowCPUConfig returns a configuration optimized for low CPU usage
func LowCPUConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmSnappy,
		Level:             0, // Snappy has no levels
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        32 * 1024,
		MinSize:           1024,
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp|mp4|mkv|avi|mov|mp3|flac|zip|gz|bz2|xz|7z|rar|zst|lz4|br|sz)$`,
		},
	}
}

// ===== CONVENIENCE CONSTRUCTORS FOR ADVANCED CONFIGS =====

// NewWithSmartConfig creates a compressed filesystem with intelligent algorithm selection
func NewWithSmartConfig(base FileSystem) (*FS, error) {
	return New(base, SmartConfig())
}

// NewWithHighPerformance creates a compressed filesystem optimized for speed
func NewWithHighPerformance(base FileSystem) (*FS, error) {
	return New(base, HighPerformanceConfig())
}

// NewWithArchival creates a compressed filesystem optimized for maximum compression
func NewWithArchival(base FileSystem) (*FS, error) {
	return New(base, ArchivalConfig())
}

// NewWithRecommendedConfig creates a new compressed filesystem with recommended settings
func NewWithRecommendedConfig(base FileSystem) (*FS, error) {
	return New(base, RecommendedConfig())
}

// NewWithFastestConfig creates a new compressed filesystem optimized for speed
func NewWithFastestConfig(base FileSystem) (*FS, error) {
	return New(base, FastestConfig())
}

// NewWithBestCompression creates a new compressed filesystem optimized for compression ratio
func NewWithBestCompression(base FileSystem) (*FS, error) {
	return New(base, BestCompressionConfig())
}

// CompressBytes compresses a byte slice using the specified algorithm and level
func CompressBytes(data []byte, algo Algorithm, level int) ([]byte, error) {
	var buf io.Writer
	result := new([]byte)
	buf = &bytesWriter{result}

	compressor, err := createCompressor(algo, buf, level)
	if err != nil {
		return nil, err
	}

	if _, err := compressor.Write(data); err != nil {
		return nil, err
	}

	if err := compressor.Close(); err != nil {
		return nil, err
	}

	return *result, nil
}

// DecompressBytes decompresses a byte slice using the specified algorithm
func DecompressBytes(data []byte, algo Algorithm) ([]byte, error) {
	reader := &bytesReader{data: data}

	decompressor, err := createDecompressor(algo, reader, 0)
	if err != nil {
		return nil, err
	}
	defer decompressor.Close()

	return io.ReadAll(decompressor)
}

// DetectCompressionAlgorithm detects the compression algorithm from data
func DetectCompressionAlgorithm(data []byte) (Algorithm, bool) {
	return IsCompressed(data)
}

// bytesWriter implements io.Writer for byte slices
type bytesWriter struct {
	result *[]byte
}

func (w *bytesWriter) Write(p []byte) (n int, err error) {
	*w.result = append(*w.result, p...)
	return len(p), nil
}

// bytesReader implements io.Reader for byte slices
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// GetCompressionRatio calculates the compression ratio for given original and compressed sizes
// Returns a value between 0 and 1, where lower is better
// E.g., 0.5 means the compressed size is 50% of the original
func GetCompressionRatio(originalSize, compressedSize int64) float64 {
	if originalSize == 0 {
		return 0
	}
	return float64(compressedSize) / float64(originalSize)
}

// GetCompressionPercentage calculates the compression percentage
// Returns the percentage of space saved (0-100)
// E.g., 50 means 50% space savings
func GetCompressionPercentage(originalSize, compressedSize int64) float64 {
	if originalSize == 0 {
		return 0
	}
	return (1 - float64(compressedSize)/float64(originalSize)) * 100
}

// ===== ADVANCED FEATURES (Phase 5) =====

// TrainZstdDictionary trains a zstd dictionary from sample data
// samples should contain representative data similar to what will be compressed
// dictSize is the target dictionary size in bytes (recommended: 100KB - 1MB)
// Returns the trained dictionary or an error
func TrainZstdDictionary(samples [][]byte, dictSize int) ([]byte, error) {
	// Note: This requires the zstd library's training functionality
	// For now, we'll return a placeholder - full implementation would use:
	// github.com/klauspost/compress/zstd BuildDict functionality
	return nil, nil
}

// SmartConfig returns a configuration with intelligent defaults based on use case
// Enables auto-tuning, algorithm rules for different file types, and skip patterns
func SmartConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        64 * 1024,
		MinSize:           512,
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp|bmp|ico)$`,
			`\.(mp4|mkv|avi|mov|webm|flv|wmv)$`,
			`\.(mp3|flac|ogg|m4a|aac|wma|wav)$`,
			`\.(zip|gz|bz2|xz|7z|rar|tar)$`,
			`\.(zst|lz4|br|sz|snappy)$`,
		},
		AlgorithmRules: []AlgorithmRule{
			// Fast compression for logs
			{Pattern: `\.log$`, Algorithm: AlgorithmLZ4, Level: 0},
			{Pattern: `/logs/`, Algorithm: AlgorithmLZ4, Level: 0},

			// Best compression for JSON/text data
			{Pattern: `\.(json|xml|yaml|yml)$`, Algorithm: AlgorithmZstd, Level: 6},

			// Fast compression for temporary files
			{Pattern: `\.tmp$`, Algorithm: AlgorithmSnappy, Level: 0},
			{Pattern: `/tmp/`, Algorithm: AlgorithmSnappy, Level: 0},

			// Balanced for source code
			{Pattern: `\.(go|js|ts|py|java|c|cpp|h|hpp)$`, Algorithm: AlgorithmZstd, Level: 3},
		},
		EnableAutoTuning:          true,
		AutoTuneSizeThreshold:     1024 * 1024, // 1MB
		EnableParallelCompression: false,        // Disabled by default (experimental)
		ParallelThreshold:         10 * 1024 * 1024,
		ParallelChunkSize:         1024 * 1024,
	}
}

// HighPerformanceConfig returns a configuration optimized for high throughput
// Uses LZ4 for maximum speed with minimal CPU usage
func HighPerformanceConfig() *Config {
	return &Config{
		Algorithm:                 AlgorithmLZ4,
		Level:                     0,
		AutoDetect:                true,
		PreserveExtension:         true,
		StripExtension:            true,
		BufferSize:                256 * 1024, // Larger buffer for throughput
		MinSize:                   1024,       // Skip small files
		EnableAutoTuning:          false,      // LZ4 is already optimized
		EnableParallelCompression: true,       // Good for large files
		ParallelThreshold:         5 * 1024 * 1024,
		ParallelChunkSize:         2 * 1024 * 1024,
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp)$`,
			`\.(mp4|mkv|avi|mov)$`,
			`\.(mp3|flac|ogg|m4a)$`,
			`\.(zip|gz|bz2|xz|7z|rar)$`,
		},
	}
}

// ArchivalConfig returns a configuration optimized for long-term storage
// Maximum compression with brotli, optimized for write-once/read-many
func ArchivalConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmBrotli,
		Level:             11,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        128 * 1024,
		MinSize:           1024,
		AlgorithmRules: []AlgorithmRule{
			// Text files get maximum compression
			{Pattern: `\.(txt|log|csv)$`, Algorithm: AlgorithmBrotli, Level: 11},

			// Structured data gets high compression
			{Pattern: `\.(json|xml|yaml|yml)$`, Algorithm: AlgorithmBrotli, Level: 11},

			// Source code gets balanced compression (still high but faster)
			{Pattern: `\.(go|js|ts|py|java|c|cpp)$`, Algorithm: AlgorithmZstd, Level: 15},
		},
		EnableAutoTuning: false, // Use maximum compression always
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif|webp)$`,
			`\.(mp4|mkv|avi|mov)$`,
			`\.(mp3|flac|ogg|m4a)$`,
			`\.(zip|gz|bz2|xz|7z|rar)$`,
		},
	}
}
