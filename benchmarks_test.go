package compressfs

import (
	"io"
	"testing"
)

// Benchmark data generators
func generateTestData(size int) []byte {
	// Generate semi-compressible data (mix of patterns and random)
	data := make([]byte, size)
	for i := range data {
		if i%4 == 0 {
			data[i] = byte(i % 256)
		} else {
			data[i] = byte(i % 64) // More repetitive for better compression
		}
	}
	return data
}

func generateHighlyCompressibleData(size int) []byte {
	// Generate highly compressible data (lots of repetition)
	data := make([]byte, size)
	pattern := []byte("The quick brown fox jumps over the lazy dog. ")
	for i := range data {
		data[i] = pattern[i%len(pattern)]
	}
	return data
}

func generateIncompressibleData(size int) []byte {
	// Generate pseudo-random data (hard to compress)
	data := make([]byte, size)
	seed := uint64(12345)
	for i := range data {
		seed = seed*1103515245 + 12345
		data[i] = byte(seed >> 16)
	}
	return data
}

// Benchmark write operations
func benchmarkCompressionWrite(b *testing.B, algo Algorithm, level int, dataSize int) {
	testData := generateTestData(dataSize)

	b.ResetTimer()
	b.SetBytes(int64(dataSize))

	for i := 0; i < b.N; i++ {
		base := NewMemFS()
		cfs, _ := New(base, &Config{
			Algorithm:         algo,
			Level:             level,
			PreserveExtension: true,
			StripExtension:    true,
		})

		f, _ := cfs.Create("test.bin")
		f.Write(testData)
		f.Close()
	}
}

// Benchmark read operations
func benchmarkCompressionRead(b *testing.B, algo Algorithm, level int, dataSize int) {
	testData := generateTestData(dataSize)

	// Prepare compressed file once
	base := NewMemFS()
	cfs, _ := New(base, &Config{
		Algorithm:         algo,
		Level:             level,
		PreserveExtension: true,
		StripExtension:    true,
	})

	f, _ := cfs.Create("test.bin")
	f.Write(testData)
	f.Close()

	b.ResetTimer()
	b.SetBytes(int64(dataSize))

	for i := 0; i < b.N; i++ {
		f, _ := cfs.Open("test.bin")
		io.ReadAll(f)
		f.Close()
	}
}

// Small files (4KB)
func BenchmarkGzipWrite4KB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmGzip, 6, 4*1024) }
func BenchmarkZstdWrite4KB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmZstd, 3, 4*1024) }
func BenchmarkLZ4Write4KB(b *testing.B)    { benchmarkCompressionWrite(b, AlgorithmLZ4, 0, 4*1024) }
func BenchmarkBrotliWrite4KB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmBrotli, 6, 4*1024) }
func BenchmarkSnappyWrite4KB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmSnappy, 0, 4*1024) }

func BenchmarkGzipRead4KB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmGzip, 6, 4*1024) }
func BenchmarkZstdRead4KB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmZstd, 3, 4*1024) }
func BenchmarkLZ4Read4KB(b *testing.B)    { benchmarkCompressionRead(b, AlgorithmLZ4, 0, 4*1024) }
func BenchmarkBrotliRead4KB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmBrotli, 6, 4*1024) }
func BenchmarkSnappyRead4KB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmSnappy, 0, 4*1024) }

// Medium files (256KB)
func BenchmarkGzipWrite256KB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmGzip, 6, 256*1024) }
func BenchmarkZstdWrite256KB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmZstd, 3, 256*1024) }
func BenchmarkLZ4Write256KB(b *testing.B)    { benchmarkCompressionWrite(b, AlgorithmLZ4, 0, 256*1024) }
func BenchmarkBrotliWrite256KB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmBrotli, 6, 256*1024) }
func BenchmarkSnappyWrite256KB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmSnappy, 0, 256*1024) }

func BenchmarkGzipRead256KB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmGzip, 6, 256*1024) }
func BenchmarkZstdRead256KB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmZstd, 3, 256*1024) }
func BenchmarkLZ4Read256KB(b *testing.B)    { benchmarkCompressionRead(b, AlgorithmLZ4, 0, 256*1024) }
func BenchmarkBrotliRead256KB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmBrotli, 6, 256*1024) }
func BenchmarkSnappyRead256KB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmSnappy, 0, 256*1024) }

// Large files (1MB)
func BenchmarkGzipWrite1MB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmGzip, 6, 1024*1024) }
func BenchmarkZstdWrite1MB(b *testing.B)   { benchmarkCompressionWrite(b, AlgorithmZstd, 3, 1024*1024) }
func BenchmarkLZ4Write1MB(b *testing.B)    { benchmarkCompressionWrite(b, AlgorithmLZ4, 0, 1024*1024) }
func BenchmarkBrotliWrite1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmBrotli, 6, 1024*1024) }
func BenchmarkSnappyWrite1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmSnappy, 0, 1024*1024) }

func BenchmarkGzipRead1MB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmGzip, 6, 1024*1024) }
func BenchmarkZstdRead1MB(b *testing.B)   { benchmarkCompressionRead(b, AlgorithmZstd, 3, 1024*1024) }
func BenchmarkLZ4Read1MB(b *testing.B)    { benchmarkCompressionRead(b, AlgorithmLZ4, 0, 1024*1024) }
func BenchmarkBrotliRead1MB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmBrotli, 6, 1024*1024) }
func BenchmarkSnappyRead1MB(b *testing.B) { benchmarkCompressionRead(b, AlgorithmSnappy, 0, 1024*1024) }

// Compression level comparison for Zstd
func BenchmarkZstdLevel1Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmZstd, 1, 1024*1024) }
func BenchmarkZstdLevel3Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmZstd, 3, 1024*1024) }
func BenchmarkZstdLevel6Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmZstd, 6, 1024*1024) }
func BenchmarkZstdLevel9Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmZstd, 9, 1024*1024) }
func BenchmarkZstdLevel12Write1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmZstd, 12, 1024*1024) }

// Compression level comparison for Gzip
func BenchmarkGzipLevel1Write1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmGzip, 1, 1024*1024) }
func BenchmarkGzipLevel6Write1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmGzip, 6, 1024*1024) }
func BenchmarkGzipLevel9Write1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmGzip, 9, 1024*1024) }

// Compression level comparison for Brotli
func BenchmarkBrotliLevel1Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmBrotli, 1, 1024*1024) }
func BenchmarkBrotliLevel6Write1MB(b *testing.B)  { benchmarkCompressionWrite(b, AlgorithmBrotli, 6, 1024*1024) }
func BenchmarkBrotliLevel11Write1MB(b *testing.B) { benchmarkCompressionWrite(b, AlgorithmBrotli, 11, 1024*1024) }

// Benchmark different data types
func benchmarkDataTypeWrite(b *testing.B, algo Algorithm, dataGenerator func(int) []byte, size int) {
	testData := dataGenerator(size)

	b.ResetTimer()
	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		base := NewMemFS()
		cfs, _ := New(base, &Config{
			Algorithm:         algo,
			Level:             3,
			PreserveExtension: true,
			StripExtension:    true,
		})

		f, _ := cfs.Create("test.bin")
		f.Write(testData)
		f.Close()
	}
}

// Highly compressible data
func BenchmarkZstdWriteHighlyCompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmZstd, generateHighlyCompressibleData, 1024*1024)
}

func BenchmarkGzipWriteHighlyCompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmGzip, generateHighlyCompressibleData, 1024*1024)
}

func BenchmarkLZ4WriteHighlyCompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmLZ4, generateHighlyCompressibleData, 1024*1024)
}

// Incompressible data
func BenchmarkZstdWriteIncompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmZstd, generateIncompressibleData, 1024*1024)
}

func BenchmarkGzipWriteIncompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmGzip, generateIncompressibleData, 1024*1024)
}

func BenchmarkLZ4WriteIncompressible1MB(b *testing.B) {
	benchmarkDataTypeWrite(b, AlgorithmLZ4, generateIncompressibleData, 1024*1024)
}

// Benchmark full round-trip (write + read)
func benchmarkRoundTrip(b *testing.B, algo Algorithm, level int, dataSize int) {
	testData := generateTestData(dataSize)

	b.ResetTimer()
	b.SetBytes(int64(dataSize * 2)) // Count both write and read

	for i := 0; i < b.N; i++ {
		base := NewMemFS()
		cfs, _ := New(base, &Config{
			Algorithm:         algo,
			Level:             level,
			PreserveExtension: true,
			StripExtension:    true,
		})

		// Write
		f, _ := cfs.Create("test.bin")
		f.Write(testData)
		f.Close()

		// Read
		f, _ = cfs.Open("test.bin")
		io.ReadAll(f)
		f.Close()
	}
}

func BenchmarkGzipRoundTrip1MB(b *testing.B)   { benchmarkRoundTrip(b, AlgorithmGzip, 6, 1024*1024) }
func BenchmarkZstdRoundTrip1MB(b *testing.B)   { benchmarkRoundTrip(b, AlgorithmZstd, 3, 1024*1024) }
func BenchmarkLZ4RoundTrip1MB(b *testing.B)    { benchmarkRoundTrip(b, AlgorithmLZ4, 0, 1024*1024) }
func BenchmarkBrotliRoundTrip1MB(b *testing.B) { benchmarkRoundTrip(b, AlgorithmBrotli, 6, 1024*1024) }
func BenchmarkSnappyRoundTrip1MB(b *testing.B) { benchmarkRoundTrip(b, AlgorithmSnappy, 0, 1024*1024) }
