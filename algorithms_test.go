package compressfs

import (
	"bytes"
	"io"
	"testing"
)

// Test all compression algorithms with the same data
func TestAllAlgorithms(t *testing.T) {
	testData := []byte("Hello, World! This is test data for compression algorithms. " +
		"Let's make it a bit longer to get better compression ratios. " +
		"Compression is the process of encoding information using fewer bits than the original representation.")

	algorithms := []struct {
		name  string
		algo  Algorithm
		level int
	}{
		{"gzip-default", AlgorithmGzip, 0},
		{"gzip-level6", AlgorithmGzip, 6},
		{"gzip-level9", AlgorithmGzip, 9},
		{"zstd-default", AlgorithmZstd, 0},
		{"zstd-level3", AlgorithmZstd, 3},
		{"zstd-level6", AlgorithmZstd, 6},
		{"lz4-default", AlgorithmLZ4, 0},
		{"lz4-level9", AlgorithmLZ4, 9},
		{"brotli-default", AlgorithmBrotli, 0},
		{"brotli-level6", AlgorithmBrotli, 6},
		{"brotli-level11", AlgorithmBrotli, 11},
		{"snappy", AlgorithmSnappy, 0},
	}

	for _, tt := range algorithms {
		t.Run(tt.name, func(t *testing.T) {
			base := NewMemFS()
			cfs, err := New(base, &Config{
				Algorithm:         tt.algo,
				Level:             tt.level,
				PreserveExtension: true,
				StripExtension:    true,
			})
			if err != nil {
				t.Fatalf("Failed to create compressfs: %v", err)
			}

			// Write data
			f, err := cfs.Create("test.txt")
			if err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}

			n, err := f.Write(testData)
			if err != nil {
				t.Fatalf("Failed to write data: %v", err)
			}
			if n != len(testData) {
				t.Fatalf("Expected to write %d bytes, wrote %d", len(testData), n)
			}

			err = f.Close()
			if err != nil {
				t.Fatalf("Failed to close file: %v", err)
			}

			// Read back data
			f, err = cfs.Open("test.txt")
			if err != nil {
				t.Fatalf("Failed to open file: %v", err)
			}

			readData, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("Failed to read data: %v", err)
			}

			err = f.Close()
			if err != nil {
				t.Fatalf("Failed to close file: %v", err)
			}

			// Verify data matches
			if !bytes.Equal(readData, testData) {
				t.Fatalf("Read data does not match written data.\nExpected length: %d, Got length: %d",
					len(testData), len(readData))
			}

			// Check stats
			stats := cfs.GetStats()
			if stats.FilesCompressed != 1 {
				t.Errorf("Expected 1 file compressed, got %d", stats.FilesCompressed)
			}
			if stats.FilesDecompressed != 1 {
				t.Errorf("Expected 1 file decompressed, got %d", stats.FilesDecompressed)
			}
		})
	}
}

func TestZstdCompression(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmZstd,
		Level:             3,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	testData := []byte("Zstandard is a real-time compression algorithm designed for fast compression and decompression.")

	// Write and read
	f, _ := cfs.Create("test.txt")
	f.Write(testData)
	f.Close()

	// Verify compressed file exists with .zst extension
	_, err = base.Stat("test.txt.zst")
	if err != nil {
		t.Fatalf("Compressed file not found: %v", err)
	}

	f, _ = cfs.Open("test.txt")
	readData, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(readData, testData) {
		t.Fatalf("Data mismatch")
	}
}

func TestLZ4Compression(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmLZ4,
		Level:             1,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	testData := []byte("LZ4 is extremely fast compression algorithm, achieving compression speed at >500 MB/s per core.")

	// Write and read
	f, _ := cfs.Create("test.txt")
	f.Write(testData)
	f.Close()

	// Verify compressed file exists with .lz4 extension
	_, err = base.Stat("test.txt.lz4")
	if err != nil {
		t.Fatalf("Compressed file not found: %v", err)
	}

	f, _ = cfs.Open("test.txt")
	readData, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(readData, testData) {
		t.Fatalf("Data mismatch")
	}
}

func TestBrotliCompression(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmBrotli,
		Level:             6,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	testData := []byte("Brotli is a compression format developed by Google for web font and asset compression.")

	// Write and read
	f, _ := cfs.Create("test.txt")
	f.Write(testData)
	f.Close()

	// Verify compressed file exists with .br extension
	_, err = base.Stat("test.txt.br")
	if err != nil {
		t.Fatalf("Compressed file not found: %v", err)
	}

	f, _ = cfs.Open("test.txt")
	readData, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(readData, testData) {
		t.Fatalf("Data mismatch")
	}
}

func TestSnappyCompression(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmSnappy,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	testData := []byte("Snappy is a compression library developed by Google, optimized for speed rather than compression ratio.")

	// Write and read
	f, _ := cfs.Create("test.txt")
	f.Write(testData)
	f.Close()

	// Verify compressed file exists with .sz extension
	_, err = base.Stat("test.txt.sz")
	if err != nil {
		t.Fatalf("Compressed file not found: %v", err)
	}

	f, _ = cfs.Open("test.txt")
	readData, _ := io.ReadAll(f)
	f.Close()

	if !bytes.Equal(readData, testData) {
		t.Fatalf("Data mismatch")
	}
}

func TestLargeDataCompression(t *testing.T) {
	// Create large test data (1MB)
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	algorithms := []Algorithm{
		AlgorithmGzip,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmBrotli,
		AlgorithmSnappy,
	}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			base := NewMemFS()
			cfs, err := New(base, &Config{
				Algorithm:         algo,
				PreserveExtension: true,
				StripExtension:    true,
				BufferSize:        64 * 1024,
			})
			if err != nil {
				t.Fatalf("Failed to create compressfs: %v", err)
			}

			// Write large data
			f, err := cfs.Create("large.bin")
			if err != nil {
				t.Fatalf("Failed to create file: %v", err)
			}

			n, err := f.Write(testData)
			if err != nil {
				t.Fatalf("Failed to write data: %v", err)
			}
			if n != len(testData) {
				t.Fatalf("Expected to write %d bytes, wrote %d", len(testData), n)
			}

			err = f.Close()
			if err != nil {
				t.Fatalf("Failed to close file: %v", err)
			}

			// Read back large data
			f, err = cfs.Open("large.bin")
			if err != nil {
				t.Fatalf("Failed to open file: %v", err)
			}

			readData, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("Failed to read data: %v", err)
			}

			err = f.Close()
			if err != nil {
				t.Fatalf("Failed to close file: %v", err)
			}

			// Verify data matches
			if !bytes.Equal(readData, testData) {
				t.Fatalf("Large data mismatch. Expected %d bytes, got %d bytes", len(testData), len(readData))
			}
		})
	}
}

func TestEmptyFileCompression(t *testing.T) {
	algorithms := []Algorithm{
		AlgorithmGzip,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmBrotli,
		AlgorithmSnappy,
	}

	for _, algo := range algorithms {
		t.Run(string(algo), func(t *testing.T) {
			base := NewMemFS()
			cfs, err := New(base, &Config{
				Algorithm:         algo,
				PreserveExtension: true,
				StripExtension:    true,
			})
			if err != nil {
				t.Fatalf("Failed to create compressfs: %v", err)
			}

			// Write empty file
			f, _ := cfs.Create("empty.txt")
			f.Close()

			// Read empty file
			f, err = cfs.Open("empty.txt")
			if err != nil {
				t.Fatalf("Failed to open empty file: %v", err)
			}

			readData, err := io.ReadAll(f)
			if err != nil {
				t.Fatalf("Failed to read empty file: %v", err)
			}
			f.Close()

			if len(readData) != 0 {
				t.Fatalf("Expected empty file, got %d bytes", len(readData))
			}
		})
	}
}
