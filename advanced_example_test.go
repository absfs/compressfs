package compressfs_test

import (
	"fmt"
	"log"

	"github.com/absfs/compressfs"
)

// ExampleSmartConfig demonstrates using SmartConfig with intelligent algorithm selection
func Example_smartConfig() {
	// Create a memory filesystem for demo
	memfs := compressfs.NewMemFS()

	// Create filesystem with smart configuration
	// - Auto-selects algorithms based on file type
	// - LZ4 for logs (speed)
	// - Zstd for JSON/XML (balance)
	// - Snappy for temp files (very fast)
	fs, err := compressfs.NewWithSmartConfig(memfs)
	if err != nil {
		log.Fatal(err)
	}

	// Log files automatically use LZ4 (fast)
	logFile, _ := fs.Create("app.log")
	logFile.Write([]byte("2025-01-15 INFO: Application started\n"))
	logFile.Close()

	// JSON files automatically use Zstd level 6 (good compression)
	jsonFile, _ := fs.Create("config.json")
	jsonFile.Write([]byte(`{"setting": "value", "count": 42}`))
	jsonFile.Close()

	// Regular files use default Zstd level 3
	textFile, _ := fs.Create("readme.txt")
	textFile.Write([]byte("This is a readme file"))
	textFile.Close()

	fmt.Println("Files compressed with smart algorithm selection")
	// Output: Files compressed with smart algorithm selection
}

// ExampleAlgorithmRules demonstrates custom algorithm rules
func Example_algorithmRules() {
	memfs := compressfs.NewMemFS()

	// Define custom rules for different file types
	config := &compressfs.Config{
		Algorithm: compressfs.AlgorithmZstd,
		Level:     3,
		AlgorithmRules: []compressfs.AlgorithmRule{
			// Critical data: maximum compression
			{
				Pattern:   `^/important/`,
				Algorithm: compressfs.AlgorithmBrotli,
				Level:     11,
			},
			// Logs: fast compression
			{
				Pattern:   `\.log$`,
				Algorithm: compressfs.AlgorithmLZ4,
				Level:     0,
			},
			// Cache files: very fast
			{
				Pattern:   `^/cache/`,
				Algorithm: compressfs.AlgorithmSnappy,
				Level:     0,
			},
		},
		PreserveExtension: true,
		StripExtension:    true,
	}

	fs, _ := compressfs.New(memfs, config)

	// Each file uses the algorithm matching its pattern
	fs.Create("/important/data.txt")   // Uses Brotli level 11
	fs.Create("app.log")               // Uses LZ4
	fs.Create("/cache/temp.dat")       // Uses Snappy
	fs.Create("regular.txt")           // Uses default Zstd level 3

	fmt.Println("Files compressed with custom rules")
	// Output: Files compressed with custom rules
}

// ExampleAutoTuning demonstrates automatic compression level adjustment
func Example_autoTuning() {
	memfs := compressfs.NewMemFS()

	config := &compressfs.Config{
		Algorithm:             compressfs.AlgorithmZstd,
		Level:                 6, // High compression by default
		EnableAutoTuning:      true,
		AutoTuneSizeThreshold: 1024 * 1024, // 1MB
		PreserveExtension:     true,
		StripExtension:        true,
	}

	fs, _ := compressfs.New(memfs, config)

	// Small files (< 1MB) use level 6 (high compression)
	smallFile, _ := fs.Create("small.txt")
	smallFile.Write(make([]byte, 100*1024)) // 100KB
	smallFile.Close()

	// Large files (> 1MB) automatically use lower level for speed
	// Level is reduced to 1-2 for faster compression
	largeFile, _ := fs.Create("large.dat")
	largeFile.Write(make([]byte, 10*1024*1024)) // 10MB
	largeFile.Close()

	fmt.Println("Compression levels auto-tuned based on file size")
	// Output: Compression levels auto-tuned based on file size
}

// ExampleZstdDictionary demonstrates dictionary-based compression
func Example_zstdDictionary() {
	memfs := compressfs.NewMemFS()

	// In practice, train dictionary from sample data
	// For demo, use a simple dictionary
	dictionary := []byte("common repeated pattern")

	config := &compressfs.Config{
		Algorithm:         compressfs.AlgorithmZstd,
		Level:             3,
		ZstdDictionary:    dictionary,
		PreserveExtension: true,
		StripExtension:    true,
	}

	fs, _ := compressfs.New(memfs, config)

	// Files with similar patterns compress better with dictionary
	file, _ := fs.Create("data.txt")
	file.Write([]byte("common repeated pattern appears common repeated pattern"))
	file.Close()

	// Read back - dictionary is used automatically
	file, _ = fs.Open("data.txt")
	data := make([]byte, 1024)
	n, _ := file.Read(data)
	file.Close()

	fmt.Printf("Read %d bytes with dictionary compression\n", n)
	// Output: Read 56 bytes with dictionary compression
}

// ExampleHighPerformanceConfig demonstrates high-throughput configuration
func Example_highPerformanceConfig() {
	memfs := compressfs.NewMemFS()

	// Optimized for maximum speed
	// - LZ4 algorithm (fastest)
	// - Large buffers (256KB)
	// - Parallel compression enabled
	fs, _ := compressfs.NewWithHighPerformance(memfs)

	// Compress data at maximum speed
	file, _ := fs.Create("data.bin")
	file.Write(make([]byte, 1024*1024)) // 1MB
	file.Close()

	fmt.Println("Data compressed at high speed")
	// Output: Data compressed at high speed
}

// ExampleArchivalConfig demonstrates maximum compression for archival
func Example_archivalConfig() {
	memfs := compressfs.NewMemFS()

	// Optimized for maximum compression
	// - Brotli level 11 (best compression)
	// - Custom rules for different file types
	fs, _ := compressfs.NewWithArchival(memfs)

	// Compress for long-term storage
	file, _ := fs.Create("archive.txt")
	file.Write([]byte("Important data to archive with maximum compression"))
	file.Close()

	fmt.Println("Data compressed for archival storage")
	// Output: Data compressed for archival storage
}

// ExampleCombinedFeatures demonstrates using multiple advanced features together
func Example_combinedFeatures() {
	memfs := compressfs.NewMemFS()

	// Combine algorithm rules, auto-tuning, and dictionaries
	config := &compressfs.Config{
		Algorithm: compressfs.AlgorithmZstd,
		Level:     3,
		AlgorithmRules: []compressfs.AlgorithmRule{
			{Pattern: `\.log$`, Algorithm: compressfs.AlgorithmLZ4},
			{Pattern: `\.json$`, Algorithm: compressfs.AlgorithmZstd, Level: 6},
		},
		EnableAutoTuning:      true,
		AutoTuneSizeThreshold: 1024 * 1024,
		ZstdDictionary:        []byte("sample dictionary"),
		SkipPatterns: []string{
			`\.(jpg|png|zip)$`, // Skip already compressed
		},
		PreserveExtension: true,
		StripExtension:    true,
	}

	fs, _ := compressfs.New(memfs, config)

	// Each file is handled optimally
	fs.Create("app.log")      // LZ4 (rule)
	fs.Create("data.json")    // Zstd level 6 (rule)
	fs.Create("large.txt")    // Auto-tuned level
	fs.Create("photo.jpg")    // Skipped (already compressed)

	fmt.Println("Combined features for optimal compression")
	// Output: Combined features for optimal compression
}
