package compressfs

import (
	"bytes"
	"io"
	"testing"
)

func TestPresetConfigs(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{"Fastest", FastestConfig()},
		{"Recommended", RecommendedConfig()},
		{"BestCompression", BestCompressionConfig()},
		{"Compatible", CompatibleConfig()},
		{"LowCPU", LowCPUConfig()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config == nil {
				t.Fatal("Config is nil")
			}
			if tt.config.Algorithm == "" {
				t.Error("Algorithm not set")
			}
			if tt.config.BufferSize == 0 {
				t.Error("BufferSize not set")
			}
		})
	}
}

func TestNewWithPresets(t *testing.T) {
	base := NewMemFS()

	tests := []struct {
		name   string
		create func(FileSystem) (*FS, error)
	}{
		{"Recommended", NewWithRecommendedConfig},
		{"Fastest", NewWithFastestConfig},
		{"BestCompression", NewWithBestCompression},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs, err := tt.create(base)
			if err != nil {
				t.Fatalf("Failed to create FS: %v", err)
			}
			if fs == nil {
				t.Fatal("FS is nil")
			}

			// Test basic operation with data large enough to compress
			f, err := fs.Create("test.txt")
			if err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}
			// Use larger data to exceed MinSize threshold
			data := make([]byte, 2048)
			for i := range data {
				data[i] = byte(i % 256)
			}
			f.Write(data)
			f.Close()

			// Read back
			f, err = fs.Open("test.txt")
			if err != nil {
				t.Fatalf("Failed to open file: %v", err)
			}
			readData, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}
			f.Close()

			if !bytes.Equal(data, readData) {
				t.Errorf("Data mismatch. Expected: %s, Got: %s", data, readData)
			}
		})
	}
}

func TestCompressBytes(t *testing.T) {
	testData := []byte("Hello, World! This is test data for byte compression.")

	algorithms := []Algorithm{
		AlgorithmGzip,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmBrotli,
		AlgorithmSnappy,
	}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			// Compress
			compressed, err := CompressBytes(testData, algo, 6)
			if err != nil {
				t.Fatalf("Failed to compress: %v", err)
			}

			if len(compressed) == 0 {
				t.Fatal("Compressed data is empty")
			}

			// Verify it's different from original (compressed)
			if bytes.Equal(testData, compressed) && len(testData) > 10 {
				t.Error("Compressed data is identical to original")
			}

			// Decompress
			decompressed, err := DecompressBytes(compressed, algo)
			if err != nil {
				t.Fatalf("Failed to decompress: %v", err)
			}

			// Verify decompressed matches original
			if !bytes.Equal(testData, decompressed) {
				t.Errorf("Decompressed data doesn't match original.\nExpected: %s\nGot: %s",
					testData, decompressed)
			}
		})
	}
}

func TestDetectCompressionAlgorithm(t *testing.T) {
	testData := []byte("Test data for compression detection")

	tests := []struct {
		algo Algorithm
	}{
		{AlgorithmGzip},
		{AlgorithmZstd},
		{AlgorithmLZ4},
	}

	for _, tt := range tests {
		t.Run(string(tt.algo), func(t *testing.T) {
			// Compress data
			compressed, err := CompressBytes(testData, tt.algo, 6)
			if err != nil {
				t.Fatalf("Failed to compress: %v", err)
			}

			// Detect algorithm
			detected, found := DetectCompressionAlgorithm(compressed)
			if !found {
				t.Error("Failed to detect compression")
			}

			if detected != tt.algo {
				t.Errorf("Expected algorithm %s, detected %s", tt.algo, detected)
			}
		})
	}

	// Test with uncompressed data
	t.Run("Uncompressed", func(t *testing.T) {
		_, found := DetectCompressionAlgorithm(testData)
		if found {
			t.Error("Incorrectly detected compression in plain data")
		}
	})
}

func TestGetCompressionRatio(t *testing.T) {
	tests := []struct {
		name           string
		original       int64
		compressed     int64
		expectedRatio  float64
		expectedPercent float64
	}{
		{"50% compression", 1000, 500, 0.5, 50.0},
		{"75% compression", 1000, 250, 0.25, 75.0},
		{"No compression", 1000, 1000, 1.0, 0.0},
		{"Zero original", 0, 500, 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratio := GetCompressionRatio(tt.original, tt.compressed)
			if ratio != tt.expectedRatio {
				t.Errorf("Expected ratio %.2f, got %.2f", tt.expectedRatio, ratio)
			}

			percent := GetCompressionPercentage(tt.original, tt.compressed)
			if percent != tt.expectedPercent {
				t.Errorf("Expected percentage %.2f, got %.2f", tt.expectedPercent, percent)
			}
		})
	}
}

func TestEmptyDataCompression(t *testing.T) {
	emptyData := []byte{}

	algorithms := []Algorithm{
		AlgorithmGzip,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmSnappy,
	}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			// Compress empty data
			compressed, err := CompressBytes(emptyData, algo, 6)
			if err != nil {
				t.Fatalf("Failed to compress empty data: %v", err)
			}

			// Decompress
			decompressed, err := DecompressBytes(compressed, algo)
			if err != nil {
				t.Fatalf("Failed to decompress: %v", err)
			}

			if len(decompressed) != 0 {
				t.Errorf("Expected empty decompressed data, got %d bytes", len(decompressed))
			}
		})
	}
}

func TestLargeDataBytesCompression(t *testing.T) {
	// Create 1MB of test data
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	// Test with Zstd (fastest for large data)
	compressed, err := CompressBytes(largeData, AlgorithmZstd, 3)
	if err != nil {
		t.Fatalf("Failed to compress large data: %v", err)
	}

	// Verify compression occurred
	ratio := GetCompressionRatio(int64(len(largeData)), int64(len(compressed)))
	if ratio >= 1.0 {
		t.Errorf("Expected compression, but data grew. Ratio: %.2f", ratio)
	}

	// Decompress and verify
	decompressed, err := DecompressBytes(compressed, AlgorithmZstd)
	if err != nil {
		t.Fatalf("Failed to decompress large data: %v", err)
	}

	if !bytes.Equal(largeData, decompressed) {
		t.Error("Large data mismatch after compression/decompression")
	}

	t.Logf("Large data compression: %.2f%% reduction",
		GetCompressionPercentage(int64(len(largeData)), int64(len(compressed))))
}
