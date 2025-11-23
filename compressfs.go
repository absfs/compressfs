package compressfs

import (
	"errors"
	"io"
	"io/fs"
	"regexp"
	"sync"
	"sync/atomic"
)

// Algorithm represents a compression algorithm
type Algorithm string

const (
	AlgorithmGzip   Algorithm = "gzip"
	AlgorithmZstd   Algorithm = "zstd"
	AlgorithmLZ4    Algorithm = "lz4"
	AlgorithmBrotli Algorithm = "brotli"
	AlgorithmSnappy Algorithm = "snappy"
	AlgorithmAuto   Algorithm = "auto"
)

// Config holds compression filesystem configuration
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

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		SkipPatterns:      nil,
		AutoDetect:        true,
		PreserveExtension: true,
		StripExtension:    true,
		BufferSize:        64 * 1024, // 64KB
		MinSize:           0,
	}
}

// Stats holds compression statistics
type Stats struct {
	FilesCompressed   int64
	FilesDecompressed int64
	FilesSkipped      int64

	BytesRead         int64
	BytesWritten      int64
	BytesCompressed   int64
	BytesDecompressed int64

	AlgorithmCounts sync.Map // map[Algorithm]int64
}

// GetAlgorithmCount returns the count for a specific algorithm
func (s *Stats) GetAlgorithmCount(algo Algorithm) int64 {
	if val, ok := s.AlgorithmCounts.Load(algo); ok {
		return val.(int64)
	}
	return 0
}

// IncrementAlgorithmCount increments the count for a specific algorithm
func (s *Stats) IncrementAlgorithmCount(algo Algorithm) {
	val, _ := s.AlgorithmCounts.LoadOrStore(algo, int64(0))
	s.AlgorithmCounts.Store(algo, val.(int64)+1)
}

// TotalCompressionRatio returns the overall compression ratio
func (s *Stats) TotalCompressionRatio() float64 {
	if s.BytesWritten == 0 {
		return 0
	}
	return float64(s.BytesCompressed) / float64(s.BytesWritten)
}

// TotalDecompressionRatio returns the overall decompression ratio
func (s *Stats) TotalDecompressionRatio() float64 {
	if s.BytesDecompressed == 0 {
		return 0
	}
	return float64(s.BytesRead) / float64(s.BytesDecompressed)
}

var (
	ErrUnsupportedAlgorithm = errors.New("compressfs: unsupported compression algorithm")
	ErrInvalidLevel         = errors.New("compressfs: invalid compression level")
	ErrSeekNotSupported     = errors.New("compressfs: seek not supported for compressed files")
	ErrAlreadyCompressed    = errors.New("compressfs: file already compressed")
	ErrCorruptedData        = errors.New("compressfs: corrupted compressed data")
)

// FileSystem interface that compressfs wraps
type FileSystem interface {
	Open(name string) (File, error)
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	Create(name string) (File, error)
	Mkdir(name string, perm fs.FileMode) error
	Remove(name string) error
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]fs.DirEntry, error)
}

// File interface for compressed files
type File interface {
	io.Reader
	io.Writer
	io.Closer
	io.Seeker
	Stat() (fs.FileInfo, error)
	Sync() error
}

// FS wraps a FileSystem with compression capabilities
type FS struct {
	base   FileSystem
	config *Config
	skip   *regexp.Regexp // Compiled skip patterns
	stats  Stats
	mu     sync.RWMutex
}

// New creates a new compressed filesystem wrapper
func New(base FileSystem, config *Config) (*FS, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Compile skip patterns
	var skip *regexp.Regexp
	if len(config.SkipPatterns) > 0 {
		pattern := "(?:" + config.SkipPatterns[0]
		for i := 1; i < len(config.SkipPatterns); i++ {
			pattern += "|" + config.SkipPatterns[i]
		}
		pattern += ")"
		var err error
		skip, err = regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
	}

	return &FS{
		base:   base,
		config: config,
		skip:   skip,
		stats:  Stats{},
	}, nil
}

// shouldSkip returns true if the file should not be compressed
func (cfs *FS) shouldSkip(name string) bool {
	if cfs.skip == nil {
		return false
	}
	return cfs.skip.MatchString(name)
}

// GetStats returns current statistics
func (cfs *FS) GetStats() *Stats {
	cfs.mu.RLock()
	defer cfs.mu.RUnlock()
	// Return a copy
	return &Stats{
		FilesCompressed:   atomic.LoadInt64(&cfs.stats.FilesCompressed),
		FilesDecompressed: atomic.LoadInt64(&cfs.stats.FilesDecompressed),
		FilesSkipped:      atomic.LoadInt64(&cfs.stats.FilesSkipped),
		BytesRead:         atomic.LoadInt64(&cfs.stats.BytesRead),
		BytesWritten:      atomic.LoadInt64(&cfs.stats.BytesWritten),
		BytesCompressed:   atomic.LoadInt64(&cfs.stats.BytesCompressed),
		BytesDecompressed: atomic.LoadInt64(&cfs.stats.BytesDecompressed),
	}
}

// ResetStats resets statistics to zero
func (cfs *FS) ResetStats() {
	cfs.mu.Lock()
	defer cfs.mu.Unlock()
	atomic.StoreInt64(&cfs.stats.FilesCompressed, 0)
	atomic.StoreInt64(&cfs.stats.FilesDecompressed, 0)
	atomic.StoreInt64(&cfs.stats.FilesSkipped, 0)
	atomic.StoreInt64(&cfs.stats.BytesRead, 0)
	atomic.StoreInt64(&cfs.stats.BytesWritten, 0)
	atomic.StoreInt64(&cfs.stats.BytesCompressed, 0)
	atomic.StoreInt64(&cfs.stats.BytesDecompressed, 0)
	cfs.stats.AlgorithmCounts = sync.Map{}
}

// SetAlgorithm changes the compression algorithm
func (cfs *FS) SetAlgorithm(algo Algorithm) error {
	cfs.mu.Lock()
	defer cfs.mu.Unlock()
	cfs.config.Algorithm = algo
	return nil
}

// SetLevel changes the compression level
func (cfs *FS) SetLevel(level int) error {
	cfs.mu.Lock()
	defer cfs.mu.Unlock()
	cfs.config.Level = level
	return nil
}
