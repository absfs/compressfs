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

// AlgorithmRule defines algorithm selection based on file patterns
type AlgorithmRule struct {
	// Pattern to match file names (regex)
	Pattern string

	// Algorithm to use for matching files
	Algorithm Algorithm

	// Compression level override (0 = use default)
	Level int
}

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

	// ===== ADVANCED FEATURES (Phase 5) =====

	// AlgorithmRules defines file-specific algorithm selection
	// Rules are evaluated in order, first match wins
	AlgorithmRules []AlgorithmRule

	// EnableAutoTuning enables automatic compression level adjustment
	// based on file size and type
	EnableAutoTuning bool

	// AutoTuneSizeThreshold is the file size threshold for auto-tuning (bytes)
	// Files larger than this may use lower compression levels for speed
	AutoTuneSizeThreshold int64 // default: 1MB

	// ZstdDictionary is a pre-trained dictionary for zstd compression
	// Improves compression ratio for similar files
	ZstdDictionary []byte

	// EnableParallelCompression enables parallel compression for large files
	// Only applies to files larger than ParallelThreshold
	EnableParallelCompression bool

	// ParallelThreshold is the minimum file size for parallel compression
	ParallelThreshold int64 // default: 10MB

	// ParallelChunkSize is the chunk size for parallel compression
	ParallelChunkSize int // default: 1MB

	// AllowRecompression allows transparent re-compression when reading
	// files compressed with a different algorithm
	AllowRecompression bool

	// RecompressionTarget is the target algorithm for re-compression
	RecompressionTarget Algorithm
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Algorithm:                 AlgorithmZstd,
		Level:                     3,
		SkipPatterns:              nil,
		AutoDetect:                true,
		PreserveExtension:         true,
		StripExtension:            true,
		BufferSize:                64 * 1024,  // 64KB
		MinSize:                   0,
		AlgorithmRules:            nil,
		EnableAutoTuning:          false,
		AutoTuneSizeThreshold:     1024 * 1024,      // 1MB
		ZstdDictionary:            nil,
		EnableParallelCompression: false,
		ParallelThreshold:         10 * 1024 * 1024, // 10MB
		ParallelChunkSize:         1024 * 1024,      // 1MB
		AllowRecompression:        false,
		RecompressionTarget:       AlgorithmZstd,
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

// compiledRule holds a compiled algorithm rule
type compiledRule struct {
	pattern   *regexp.Regexp
	algorithm Algorithm
	level     int
}

// FS wraps a FileSystem with compression capabilities
type FS struct {
	base   FileSystem
	config *Config
	skip   *regexp.Regexp // Compiled skip patterns
	rules  []compiledRule  // Compiled algorithm rules
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

	// Compile algorithm rules
	var rules []compiledRule
	if len(config.AlgorithmRules) > 0 {
		rules = make([]compiledRule, 0, len(config.AlgorithmRules))
		for _, rule := range config.AlgorithmRules {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return nil, err
			}
			rules = append(rules, compiledRule{
				pattern:   re,
				algorithm: rule.Algorithm,
				level:     rule.Level,
			})
		}
	}

	return &FS{
		base:   base,
		config: config,
		skip:   skip,
		rules:  rules,
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

// selectAlgorithm selects the compression algorithm and level based on rules
// Returns (algorithm, level, useDefaults)
func (cfs *FS) selectAlgorithm(name string, fileSize int64) (Algorithm, int, bool) {
	cfs.mu.RLock()
	defer cfs.mu.RUnlock()

	// Check algorithm rules first (highest priority)
	for _, rule := range cfs.rules {
		if rule.pattern.MatchString(name) {
			algo := rule.algorithm
			level := rule.level
			if level == 0 {
				// Use default level for this algorithm
				level = cfs.getDefaultLevel(algo)
			}
			return algo, level, false
		}
	}

	// Use default algorithm and level
	algo := cfs.config.Algorithm
	level := cfs.config.Level

	// Apply auto-tuning if enabled
	if cfs.config.EnableAutoTuning && fileSize > 0 {
		level = cfs.autoTuneLevel(algo, fileSize)
	}

	return algo, level, true
}

// getDefaultLevel returns the default compression level for an algorithm
func (cfs *FS) getDefaultLevel(algo Algorithm) int {
	switch algo {
	case AlgorithmGzip:
		return 6
	case AlgorithmZstd:
		return 3
	case AlgorithmLZ4:
		return 1
	case AlgorithmBrotli:
		return 6
	case AlgorithmSnappy:
		return 0 // No levels for snappy
	default:
		return cfs.config.Level
	}
}

// autoTuneLevel adjusts compression level based on file size
func (cfs *FS) autoTuneLevel(algo Algorithm, fileSize int64) int {
	// If file is smaller than threshold, use configured level
	if fileSize < cfs.config.AutoTuneSizeThreshold {
		return cfs.config.Level
	}

	// For larger files, use faster compression
	switch algo {
	case AlgorithmGzip:
		// For large files, use level 3-4 instead of default 6
		if fileSize > 10*1024*1024 { // > 10MB
			return 3
		}
		return 4
	case AlgorithmZstd:
		// For large files, use level 1-2 instead of default 3
		if fileSize > 10*1024*1024 { // > 10MB
			return 1
		}
		return 2
	case AlgorithmBrotli:
		// For large files, use much lower level
		if fileSize > 10*1024*1024 { // > 10MB
			return 3
		}
		return 4
	case AlgorithmLZ4:
		// LZ4 is already fast, keep level 1
		return 1
	case AlgorithmSnappy:
		// No levels for snappy
		return 0
	default:
		return cfs.config.Level
	}
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
