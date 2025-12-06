package compressfs

import (
	"bytes"
	"testing"
)

// TestAlgorithmRules tests file-specific algorithm selection
func TestAlgorithmRules(t *testing.T) {
	memfs := NewMemFS()

	// Configure with algorithm rules
	config := &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		PreserveExtension: true,
		StripExtension:    true,
		AlgorithmRules: []AlgorithmRule{
			// Logs use LZ4
			{Pattern: `\.log$`, Algorithm: AlgorithmLZ4, Level: 0},
			// JSON uses high compression
			{Pattern: `\.json$`, Algorithm: AlgorithmBrotli, Level: 9},
			// Text uses default
		},
	}

	fs, err := New(memfs, config)
	if err != nil {
		t.Fatalf("Failed to create FS: %v", err)
	}

	testCases := []struct {
		name            string
		filename        string
		expectedAlgo    Algorithm
		expectedLevel   int
	}{
		{"Log file", "test.log", AlgorithmLZ4, 0},
		{"JSON file", "data.json", AlgorithmBrotli, 9},
		{"Text file", "readme.txt", AlgorithmZstd, 3},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			algo, level, _ := fs.selectAlgorithm(tc.filename, 1024)
			if algo != tc.expectedAlgo {
				t.Errorf("Expected algorithm %s, got %s", tc.expectedAlgo, algo)
			}
			if level != tc.expectedLevel {
				t.Errorf("Expected level %d, got %d", tc.expectedLevel, level)
			}
		})
	}
}

// TestAutoTuning tests compression level auto-tuning based on file size
func TestAutoTuning(t *testing.T) {
	memfs := NewMemFS()

	config := &Config{
		Algorithm:             AlgorithmZstd,
		Level:                 6, // High level by default
		EnableAutoTuning:      true,
		AutoTuneSizeThreshold: 1024 * 1024, // 1MB
	}

	fs, err := New(memfs, config)
	if err != nil {
		t.Fatalf("Failed to create FS: %v", err)
	}

	testCases := []struct {
		name         string
		fileSize     int64
		shouldReduce bool // Should reduce level for large files
	}{
		{"Small file", 512 * 1024, false},           // 512KB - use high level
		{"Medium file", 2 * 1024 * 1024, true},      // 2MB - reduce level
		{"Large file", 20 * 1024 * 1024, true},      // 20MB - reduce more
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, level, _ := fs.selectAlgorithm("test.dat", tc.fileSize)

			if tc.shouldReduce && level >= config.Level {
				t.Errorf("Expected level to be reduced for large file, got %d (config: %d)", level, config.Level)
			}
			if !tc.shouldReduce && level != config.Level {
				t.Errorf("Expected level to stay at %d for small file, got %d", config.Level, level)
			}
		})
	}
}

// TestZstdDictionaryCompression tests dictionary-based compression
func TestZstdDictionaryCompression(t *testing.T) {
	memfs := NewMemFS()

	// Create a simple dictionary (in real use, this would be trained)
	dict := []byte("common pattern common pattern common pattern")

	config := &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		ZstdDictionary:    dict,
		PreserveExtension: true,
		StripExtension:    true,
	}

	fs, err := New(memfs, config)
	if err != nil {
		t.Fatalf("Failed to create FS: %v", err)
	}

	// Write data with common patterns
	data := []byte("common pattern repeated common pattern again common pattern once more")

	f, err := fs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	n, err := f.Write(data)
	if err != nil {
		t.Fatalf("Failed to write: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	// Read back and verify
	f, err = fs.Open("test.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}

	result := make([]byte, len(data)*2)
	n, err = f.Read(result)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	result = result[:n]
	if !bytes.Equal(data, result) {
		t.Errorf("Data mismatch after dictionary compression/decompression")
	}

	f.Close()
}

// TestSmartConfig tests the smart configuration preset
func TestSmartConfig(t *testing.T) {
	config := SmartConfig()

	// Verify it has algorithm rules
	if len(config.AlgorithmRules) == 0 {
		t.Error("SmartConfig should have algorithm rules")
	}

	// Verify auto-tuning is enabled
	if !config.EnableAutoTuning {
		t.Error("SmartConfig should enable auto-tuning")
	}

	// Verify skip patterns are set
	if len(config.SkipPatterns) == 0 {
		t.Error("SmartConfig should have skip patterns")
	}

	memfs := NewMemFS()
	fs, err := New(memfs, config)
	if err != nil {
		t.Fatalf("Failed to create FS with SmartConfig: %v", err)
	}

	// Test that log files use LZ4
	algo, _, _ := fs.selectAlgorithm("test.log", 1024)
	if algo != AlgorithmLZ4 {
		t.Errorf("Expected LZ4 for .log files, got %s", algo)
	}

	// Test that JSON files use high compression
	algo, level, _ := fs.selectAlgorithm("data.json", 1024)
	if algo != AlgorithmZstd {
		t.Errorf("Expected Zstd for .json files, got %s", algo)
	}
	if level < 6 {
		t.Errorf("Expected high compression level for JSON, got %d", level)
	}
}

// TestHighPerformanceConfig tests the high-performance configuration
func TestHighPerformanceConfig(t *testing.T) {
	config := HighPerformanceConfig()

	if config.Algorithm != AlgorithmLZ4 {
		t.Errorf("HighPerformanceConfig should use LZ4, got %s", config.Algorithm)
	}

	if !config.EnableParallelCompression {
		t.Error("HighPerformanceConfig should enable parallel compression")
	}

	if config.BufferSize < 128*1024 {
		t.Error("HighPerformanceConfig should use large buffers")
	}
}

// TestArchivalConfig tests the archival configuration
func TestArchivalConfig(t *testing.T) {
	config := ArchivalConfig()

	if config.Algorithm != AlgorithmBrotli {
		t.Errorf("ArchivalConfig should use Brotli, got %s", config.Algorithm)
	}

	if config.Level < 10 {
		t.Errorf("ArchivalConfig should use high compression level, got %d", config.Level)
	}

	if len(config.AlgorithmRules) == 0 {
		t.Error("ArchivalConfig should have algorithm rules")
	}
}

// TestNewConstructors tests the convenience constructors
func TestNewConstructors(t *testing.T) {
	memfs := NewMemFS()

	constructors := []struct {
		name string
		fn   func(interface{}) (*FS, error)
	}{
		{"SmartConfig", NewWithSmartConfig},
		{"HighPerformance", NewWithHighPerformance},
		{"Archival", NewWithArchival},
	}

	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			fs, err := tc.fn(memfs)
			if err != nil {
				t.Fatalf("Failed to create FS with %s: %v", tc.name, err)
			}

			// Test basic write/read
			data := []byte("test data for " + tc.name)
			f, err := fs.Create("test.txt")
			if err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}

			if _, err := f.Write(data); err != nil {
				t.Fatalf("Failed to write: %v", err)
			}

			if err := f.Close(); err != nil {
				t.Fatalf("Failed to close: %v", err)
			}

			// Read back
			f, err = fs.Open("test.txt")
			if err != nil {
				t.Fatalf("Failed to open: %v", err)
			}

			result := make([]byte, len(data)*2)
			n, err := f.Read(result)
			if err != nil {
				t.Fatalf("Failed to read: %v", err)
			}

			result = result[:n]
			if !bytes.Equal(data, result) {
				t.Errorf("Data mismatch with %s", tc.name)
			}

			f.Close()
		})
	}
}

// TestAlgorithmRulePrecedence tests that rules are evaluated in order
func TestAlgorithmRulePrecedence(t *testing.T) {
	memfs := NewMemFS()

	config := &Config{
		Algorithm: AlgorithmZstd,
		Level:     3,
		AlgorithmRules: []AlgorithmRule{
			// More specific rule should win
			{Pattern: `important\.log$`, Algorithm: AlgorithmBrotli, Level: 11},
			// General rule
			{Pattern: `\.log$`, Algorithm: AlgorithmLZ4, Level: 0},
		},
	}

	fs, err := New(memfs, config)
	if err != nil {
		t.Fatalf("Failed to create FS: %v", err)
	}

	// Test that first matching rule wins
	algo, level, _ := fs.selectAlgorithm("important.log", 1024)
	if algo != AlgorithmBrotli || level != 11 {
		t.Errorf("First matching rule should win: got %s level %d, want Brotli level 11", algo, level)
	}

	// Test that general rule matches other logs
	algo, level, _ = fs.selectAlgorithm("other.log", 1024)
	if algo != AlgorithmLZ4 || level != 0 {
		t.Errorf("General rule should match: got %s level %d, want LZ4 level 0", algo, level)
	}
}

// TestAutoTuningWithDifferentAlgorithms tests auto-tuning across algorithms
func TestAutoTuningWithDifferentAlgorithms(t *testing.T) {
	algorithms := []Algorithm{
		AlgorithmGzip,
		AlgorithmZstd,
		AlgorithmBrotli,
	}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			memfs := NewMemFS()

			config := &Config{
				Algorithm:             algo,
				Level:                 9, // High level
				EnableAutoTuning:      true,
				AutoTuneSizeThreshold: 1024 * 1024,
			}

			fs, err := New(memfs, config)
			if err != nil {
				t.Fatalf("Failed to create FS: %v", err)
			}

			// Large file should get reduced level
			_, level, _ := fs.selectAlgorithm("large.dat", 20*1024*1024)
			if level >= config.Level {
				t.Errorf("Large file should get reduced level for %s: got %d, config %d", algo, level, config.Level)
			}
		})
	}
}
