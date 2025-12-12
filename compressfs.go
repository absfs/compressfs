package compressfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/absfs/absfs"
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

	// Compression level override (-1 = use default, 0+ = specific level)
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
// Deprecated: Use absfs.FileSystem instead. This interface is maintained for backward compatibility.
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
	base   absfs.FileSystem
	config *Config
	skip   *regexp.Regexp // Compiled skip patterns
	rules  []compiledRule  // Compiled algorithm rules
	stats  Stats
	cwd    string          // Current working directory
	mu     sync.RWMutex
}

// New creates a new compressed filesystem wrapper
// The base parameter can be:
// - absfs.FileSystem
// - absfs.Filer (will be extended to FileSystem)
// - FileSystem (deprecated interface, will be adapted)
func New(base interface{}, config *Config) (*FS, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Convert the base filesystem to absfs.FileSystem
	var absBase absfs.FileSystem

	switch b := base.(type) {
	case absfs.FileSystem:
		// Already the right type
		absBase = b
	case absfs.Filer:
		// Extend Filer to FileSystem
		absBase = absfs.ExtendFiler(b)
	case FileSystem:
		// Old deprecated interface - adapt it
		if filer, ok := base.(absfs.Filer); ok {
			absBase = absfs.ExtendFiler(filer)
		} else {
			// Create a filerAdapter to make it compatible
			absBase = absfs.ExtendFiler(&filerAdapter{base: b})
		}
	default:
		return nil, errors.New("compressfs: base must be absfs.FileSystem, absfs.Filer, or compressfs.FileSystem")
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

	// Initialize cwd
	cwd := "/"
	if wd, err := absBase.Getwd(); err == nil {
		cwd = wd
	}

	return &FS{
		base:   absBase,
		config: config,
		skip:   skip,
		rules:  rules,
		stats:  Stats{},
		cwd:    cwd,
	}, nil
}

// filerAdapter adapts the old FileSystem interface to absfs.Filer
type filerAdapter struct {
	base FileSystem
}

func (a *filerAdapter) OpenFile(name string, flag int, perm os.FileMode) (absfs.File, error) {
	f, err := a.base.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f.(absfs.File), nil
}

func (a *filerAdapter) Mkdir(name string, perm os.FileMode) error {
	return a.base.Mkdir(name, perm)
}

func (a *filerAdapter) Remove(name string) error {
	return a.base.Remove(name)
}

func (a *filerAdapter) Rename(oldpath, newpath string) error {
	// Not supported in old FileSystem interface
	return os.ErrPermission
}

func (a *filerAdapter) Stat(name string) (os.FileInfo, error) {
	return a.base.Stat(name)
}

func (a *filerAdapter) Chmod(name string, mode os.FileMode) error {
	// Not supported in old FileSystem interface
	return os.ErrPermission
}

func (a *filerAdapter) Chtimes(name string, atime time.Time, mtime time.Time) error {
	// Not supported in old FileSystem interface
	return os.ErrPermission
}

func (a *filerAdapter) Chown(name string, uid, gid int) error {
	// Not supported in old FileSystem interface
	return os.ErrPermission
}

func (a *filerAdapter) ReadDir(name string) ([]fs.DirEntry, error) {
	return a.base.ReadDir(name)
}

func (a *filerAdapter) ReadFile(name string) ([]byte, error) {
	// Fallback implementation via Open and ReadAll
	f, err := a.base.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (a *filerAdapter) Sub(dir string) (fs.FS, error) {
	// Not supported in old FileSystem interface
	return nil, os.ErrPermission
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
			if level < 0 {
				// Negative level means use default for this algorithm
				level = cfs.getDefaultLevel(algo)
			}
			// Otherwise use the specified level (including 0)
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

// ============================================================================
// absfs.FileSystem interface implementation
// ============================================================================

// Rename renames (moves) a file from oldpath to newpath
func (cfs *FS) Rename(oldpath, newpath string) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Determine actual file names considering compression extensions
	actualOldpath := oldpath
	actualNewpath := newpath

	// For oldpath, try to find the actual file with compression extension
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := oldpath + ext
			if _, err := cfs.base.Stat(testName); err == nil {
				actualOldpath = testName
				// If we found a compressed file, the new path should also have the extension
				if !HasCompressionExtension(newpath) {
					actualNewpath = newpath + ext
				}
				break
			}
		}
	}

	return cfs.base.Rename(actualOldpath, actualNewpath)
}

// Chmod changes the mode of the named file
func (cfs *FS) Chmod(name string, mode os.FileMode) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try exact name first
	err := cfs.base.Chmod(name, mode)
	if err == nil {
		return nil
	}

	// If StripExtension is enabled, try with compression extensions
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if err := cfs.base.Chmod(testName, mode); err == nil {
				return nil
			}
		}
	}

	return err
}

// Chtimes changes the access and modification times of the named file
func (cfs *FS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try exact name first
	err := cfs.base.Chtimes(name, atime, mtime)
	if err == nil {
		return nil
	}

	// If StripExtension is enabled, try with compression extensions
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if err := cfs.base.Chtimes(testName, atime, mtime); err == nil {
				return nil
			}
		}
	}

	return err
}

// Chown changes the owner and group ids of the named file
func (cfs *FS) Chown(name string, uid, gid int) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try exact name first
	err := cfs.base.Chown(name, uid, gid)
	if err == nil {
		return nil
	}

	// If StripExtension is enabled, try with compression extensions
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if err := cfs.base.Chown(testName, uid, gid); err == nil {
				return nil
			}
		}
	}

	return err
}

// Chdir changes the current working directory
func (cfs *FS) Chdir(dir string) error {
	cfs.mu.Lock()
	defer cfs.mu.Unlock()

	// First check if base supports Chdir
	if err := cfs.base.Chdir(dir); err != nil {
		// If base doesn't support it or it fails, just update our internal state
		// after verifying the directory exists
		if _, err := cfs.base.Stat(dir); err != nil {
			return err
		}
	}

	// Update internal cwd
	if !filepath.IsAbs(dir) {
		cfs.cwd = filepath.Join(cfs.cwd, dir)
	} else {
		cfs.cwd = dir
	}

	return nil
}

// Getwd returns the current working directory
func (cfs *FS) Getwd() (string, error) {
	cfs.mu.RLock()
	defer cfs.mu.RUnlock()

	// Try to get from base first
	if wd, err := cfs.base.Getwd(); err == nil {
		return wd, nil
	}

	// Fall back to internal cwd
	return cfs.cwd, nil
}

// TempDir returns the temporary directory
func (cfs *FS) TempDir() string {
	return cfs.base.TempDir()
}

// MkdirAll creates a directory path, creating parent directories as needed
func (cfs *FS) MkdirAll(name string, perm os.FileMode) error {
	return cfs.base.MkdirAll(name, perm)
}

// RemoveAll removes path and any children it contains
func (cfs *FS) RemoveAll(path string) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try exact path first
	err := cfs.base.RemoveAll(path)
	if err == nil {
		return nil
	}

	// If StripExtension is enabled, try with compression extensions
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := path + ext
			if err := cfs.base.RemoveAll(testName); err == nil {
				return nil
			}
		}
	}

	return err
}

// Truncate changes the size of the named file
func (cfs *FS) Truncate(name string, size int64) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Determine actual filename considering compression extension
	actualName := name
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if _, err := cfs.base.Stat(testName); err == nil {
				actualName = testName
				break
			}
		}
	}

	return cfs.base.Truncate(actualName, size)
}

// Ensure FS implements absfs.FileSystem at compile time
var _ absfs.FileSystem = (*FS)(nil)
