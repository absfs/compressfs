package compressfs_test

import (
	"fmt"
	"io"
	"log"

	"github.com/absfs/compressfs"
)

func Example_basic() {
	// Create an in-memory filesystem for demonstration
	base := compressfs.NewMemFS()

	// Wrap with compression using gzip
	cfs, err := compressfs.New(base, &compressfs.Config{
		Algorithm:         compressfs.AlgorithmGzip,
		Level:             6,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Write a file - it will be automatically compressed
	f, err := cfs.Create("data.txt")
	if err != nil {
		log.Fatal(err)
	}

	data := []byte("Hello, compressed world! This data will be automatically compressed.")
	_, err = f.Write(data)
	if err != nil {
		log.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		log.Fatal(err)
	}

	// Read the file back - it will be automatically decompressed
	f, err = cfs.Open("data.txt")
	if err != nil {
		log.Fatal(err)
	}

	readData, err := io.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	err = f.Close()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(readData))
	// Output: Hello, compressed world! This data will be automatically compressed.
}

func Example_skipPatterns() {
	base := compressfs.NewMemFS()

	// Configure to skip already-compressed formats
	cfs, err := compressfs.New(base, &compressfs.Config{
		Algorithm:         compressfs.AlgorithmGzip,
		PreserveExtension: true,
		StripExtension:    true,
		SkipPatterns: []string{
			`\.(jpg|jpeg|png|gif)$`, // Images
			`\.(zip|gz|bz2)$`,       // Archives
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// This file will NOT be compressed (matches skip pattern)
	f, _ := cfs.Create("image.jpg")
	f.Write([]byte("fake image data"))
	f.Close()

	// This file WILL be compressed (doesn't match skip pattern)
	f, _ = cfs.Create("document.txt")
	f.Write([]byte("document content"))
	f.Close()

	fmt.Println("Files processed with skip patterns")
	// Output: Files processed with skip patterns
}

func Example_statistics() {
	base := compressfs.NewMemFS()

	cfs, err := compressfs.New(base, &compressfs.Config{
		Algorithm:         compressfs.AlgorithmGzip,
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Write some files
	for i := 0; i < 3; i++ {
		f, _ := cfs.Create(fmt.Sprintf("file%d.txt", i))
		f.Write([]byte(fmt.Sprintf("Content for file %d", i)))
		f.Close()
	}

	// Check statistics
	stats := cfs.GetStats()
	fmt.Printf("Files compressed: %d\n", stats.FilesCompressed)
	fmt.Printf("Bytes written: %d\n", stats.BytesWritten)

	// Output:
	// Files compressed: 3
	// Bytes written: 54
}

func Example_minSize() {
	base := compressfs.NewMemFS()

	cfs, err := compressfs.New(base, &compressfs.Config{
		Algorithm:         compressfs.AlgorithmGzip,
		MinSize:           100, // Only compress files >= 100 bytes
		PreserveExtension: true,
		StripExtension:    true,
	})
	if err != nil {
		log.Fatal(err)
	}

	// Small file - won't be compressed
	f, _ := cfs.Create("small.txt")
	f.Write([]byte("tiny"))
	f.Close()

	// Large file - will be compressed
	f, _ = cfs.Create("large.txt")
	largeData := make([]byte, 200)
	for i := range largeData {
		largeData[i] = 'a'
	}
	f.Write(largeData)
	f.Close()

	stats := cfs.GetStats()
	fmt.Printf("Files compressed: %d\n", stats.FilesCompressed)
	fmt.Printf("Files skipped: %d\n", stats.FilesSkipped)

	// Output:
	// Files compressed: 1
	// Files skipped: 1
}

func Example_transparentExtensions() {
	base := compressfs.NewMemFS()

	cfs, err := compressfs.New(base, &compressfs.Config{
		Algorithm:         compressfs.AlgorithmGzip,
		PreserveExtension: true, // file.txt -> file.txt.gz
		StripExtension:    true, // access via "file.txt"
	})
	if err != nil {
		log.Fatal(err)
	}

	// Write to "data.txt" - actually stored as "data.txt.gz"
	f, _ := cfs.Create("data.txt")
	f.Write([]byte("transparent compression"))
	f.Close()

	// Read from "data.txt" - automatically finds "data.txt.gz"
	f, _ = cfs.Open("data.txt")
	content, _ := io.ReadAll(f)
	f.Close()

	fmt.Println(string(content))
	// Output: transparent compression
}
