package compressfs

import (
	"bytes"
	"io"
	"testing"
)

func TestNewCompressFS(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, nil)
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}
	if cfs == nil {
		t.Fatal("Expected non-nil compressfs")
	}
}

func TestGzipCompression(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmGzip,
		Level:             6,
		PreserveExtension: true,
		StripExtension:    true,
		MinSize:           0,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	// Write data
	testData := []byte("Hello, compressed world! This is a test of gzip compression.")
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

	// Verify compressed file exists
	_, err = base.Stat("test.txt.gz")
	if err != nil {
		t.Fatalf("Compressed file not found: %v", err)
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
		t.Fatalf("Read data does not match written data.\nExpected: %s\nGot: %s", testData, readData)
	}
}

func TestSkipPatterns(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:    AlgorithmGzip,
		SkipPatterns: []string{`\.jpg$`, `\.png$`},
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	// Write image file (should be skipped)
	f, err := cfs.Create("image.jpg")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	data := []byte("fake image data")
	f.Write(data)
	f.Close()

	// Verify file was NOT compressed (no .gz extension)
	_, err = base.Stat("image.jpg")
	if err != nil {
		t.Fatalf("Uncompressed file should exist: %v", err)
	}

	_, err = base.Stat("image.jpg.gz")
	if err == nil {
		t.Fatal("File should not be compressed")
	}
}

func TestExtensionDetection(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		wantAlgo Algorithm
		wantOk   bool
	}{
		{"gzip", "file.gz", AlgorithmGzip, true},
		{"zstd", "file.zst", AlgorithmZstd, true},
		{"lz4", "file.lz4", AlgorithmLZ4, true},
		{"brotli", "file.br", AlgorithmBrotli, true},
		{"snappy", "file.sz", AlgorithmSnappy, true},
		{"none", "file.txt", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algo, ok := DetectAlgorithmFromExtension(tt.filename)
			if ok != tt.wantOk {
				t.Errorf("Expected ok=%v, got %v", tt.wantOk, ok)
			}
			if algo != tt.wantAlgo {
				t.Errorf("Expected algorithm=%s, got %s", tt.wantAlgo, algo)
			}
		})
	}
}

func TestMagicBytesDetection(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantAlgo Algorithm
		wantOk   bool
	}{
		{"gzip", []byte{0x1f, 0x8b, 0x08, 0x00}, AlgorithmGzip, true},
		{"zstd", []byte{0x28, 0xb5, 0x2f, 0xfd}, AlgorithmZstd, true},
		{"lz4", []byte{0x04, 0x22, 0x4d, 0x18}, AlgorithmLZ4, true},
		{"none", []byte("plain text"), "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			algo, ok := IsCompressed(tt.data)
			if ok != tt.wantOk {
				t.Errorf("Expected ok=%v, got %v", tt.wantOk, ok)
			}
			if algo != tt.wantAlgo {
				t.Errorf("Expected algorithm=%s, got %s", tt.wantAlgo, algo)
			}
		})
	}
}

func TestStats(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmGzip,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	// Write and read a file
	testData := []byte("Test data for statistics")
	f, err := cfs.Create("test.txt")
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	f.Write(testData)
	f.Close()

	f, err = cfs.Open("test.txt")
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	io.ReadAll(f)
	f.Close()

	// Check stats
	stats := cfs.GetStats()
	if stats.FilesCompressed != 1 {
		t.Errorf("Expected 1 file compressed, got %d", stats.FilesCompressed)
	}
	if stats.FilesDecompressed != 1 {
		t.Errorf("Expected 1 file decompressed, got %d", stats.FilesDecompressed)
	}

	// Reset stats
	cfs.ResetStats()
	stats = cfs.GetStats()
	if stats.FilesCompressed != 0 {
		t.Errorf("Expected 0 files after reset, got %d", stats.FilesCompressed)
	}
}

func TestMinSize(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm: AlgorithmGzip,
		MinSize:   100, // Only compress files >= 100 bytes
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	// Write small file (should not be compressed)
	smallData := []byte("small")
	f, _ := cfs.Create("small.txt")
	f.Write(smallData)
	f.Close()

	// Check that file is not compressed
	stats := cfs.GetStats()
	if stats.FilesCompressed != 0 {
		t.Errorf("Small file should not be compressed")
	}
}

func TestReadDir(t *testing.T) {
	base := NewMemFS()
	cfs, err := New(base, &Config{
		Algorithm:         AlgorithmGzip,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		t.Fatalf("Failed to create compressfs: %v", err)
	}

	// Create some files
	f1, _ := cfs.Create("file1.txt")
	f1.Write([]byte("data1"))
	f1.Close()

	f2, _ := cfs.Create("file2.txt")
	f2.Write([]byte("data2"))
	f2.Close()

	// Read directory
	entries, err := cfs.ReadDir(".")
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	// Should see file1.txt and file2.txt (without .gz extension)
	if len(entries) != 2 {
		t.Errorf("Expected 2 entries, got %d", len(entries))
	}

	names := make(map[string]bool)
	for _, entry := range entries {
		names[entry.Name()] = true
	}

	if !names["file1.txt"] || !names["file2.txt"] {
		t.Errorf("Expected file1.txt and file2.txt, got: %v", names)
	}
}
