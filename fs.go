package compressfs

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"sync/atomic"

	"github.com/absfs/absfs"
)

// Open opens a file for reading
func (cfs *FS) Open(name string) (absfs.File, error) {
	return cfs.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile opens a file with specified flags and permissions
func (cfs *FS) OpenFile(name string, flag int, perm fs.FileMode) (absfs.File, error) {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Determine the actual filename to open
	actualName := name
	var detectedAlgo Algorithm
	var isCreate = (flag & os.O_CREATE) != 0
	var isWrite = (flag & (os.O_WRONLY | os.O_RDWR)) != 0

	// For create/write operations, add compression extension if needed
	if (isCreate || isWrite) && !cfs.shouldSkip(name) {
		if !HasCompressionExtension(name) {
			actualName = AddExtension(name, config.Algorithm, config.PreserveExtension)
			detectedAlgo = config.Algorithm
		}
	} else if config.StripExtension {
		// For read operations, try to find compressed version
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if _, err := cfs.base.Stat(testName); err == nil {
				actualName = testName
				detectedAlgo = algo
				break
			}
		}
	}

	// Open the underlying file
	baseFile, err := cfs.base.OpenFile(actualName, flag, perm)
	if err != nil {
		return nil, err
	}

	// Wrap with compression/decompression
	return newCompressedFile(cfs, baseFile, name, actualName, flag, detectedAlgo)
}

// Create creates a new file for writing
func (cfs *FS) Create(name string) (absfs.File, error) {
	return cfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Mkdir creates a directory
func (cfs *FS) Mkdir(name string, perm fs.FileMode) error {
	return cfs.base.Mkdir(name, perm)
}

// Remove removes a file or directory
func (cfs *FS) Remove(name string) error {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try to remove with and without compression extension
	err := cfs.base.Remove(name)
	if err != nil && config.StripExtension {
		// Try with compression extension
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if removeErr := cfs.base.Remove(testName); removeErr == nil {
				return nil
			}
		}
	}
	return err
}

// Stat returns file information
func (cfs *FS) Stat(name string) (fs.FileInfo, error) {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Try exact name first
	info, err := cfs.base.Stat(name)
	if err == nil {
		return info, nil
	}

	// If StripExtension is enabled, try with compression extensions
	if config.StripExtension {
		for _, algo := range []Algorithm{config.Algorithm, AlgorithmGzip, AlgorithmZstd, AlgorithmLZ4, AlgorithmBrotli, AlgorithmSnappy} {
			ext := GetExtension(algo)
			if ext == "" {
				continue
			}
			testName := name + ext
			if info, err := cfs.base.Stat(testName); err == nil {
				return info, nil
			}
		}
	}

	return nil, err
}

// ReadDir reads directory contents
func (cfs *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	cfs.mu.RLock()
	config := cfs.config
	cfs.mu.RUnlock()

	// Delegate to base implementation if available
	entries, err := cfs.base.ReadDir(name)
	if err != nil {
		return nil, err
	}

	// If StripExtension is enabled, remove compression extensions from names
	if config.StripExtension {
		result := make([]fs.DirEntry, 0, len(entries))
		seen := make(map[string]bool)

		for _, entry := range entries {
			entryName := entry.Name()
			stripped, _, hasCompExt := StripExtension(entryName)

			// If it has compression extension, use stripped name
			if hasCompExt {
				if !seen[stripped] {
					result = append(result, &renamedDirEntry{
						DirEntry: entry,
						name:     stripped,
					})
					seen[stripped] = true
				}
			} else {
				if !seen[entryName] {
					result = append(result, entry)
					seen[entryName] = true
				}
			}
		}
		return result, nil
	}

	return entries, nil
}

// ReadFile reads the named file and returns its contents.
// This reads and decompresses the file if it's compressed.
func (cfs *FS) ReadFile(name string) ([]byte, error) {
	f, err := cfs.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get size hint for efficient allocation
	var size int64
	if info, err := f.Stat(); err == nil {
		size = info.Size()
	}

	// Pre-allocate buffer if we have size info
	if size > 0 {
		buf := make([]byte, size)
		n, err := io.ReadFull(f, buf)
		if err == io.ErrUnexpectedEOF {
			// File was shorter than expected, return what we got
			return buf[:n], nil
		}
		if err != nil {
			return nil, err
		}
		return buf, nil
	}

	// Unknown size, read all
	return io.ReadAll(f)
}

// Sub returns a fs.FS corresponding to the subtree rooted at dir.
func (cfs *FS) Sub(dir string) (fs.FS, error) {
	// Verify the directory exists
	info, err := cfs.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, &os.PathError{Op: "sub", Path: dir, Err: errors.New("not a directory")}
	}

	// Get base sub filesystem
	baseSub, err := cfs.base.Sub(dir)
	if err != nil {
		return nil, err
	}

	// Wrap it with compression
	compressSubFS, err := New(baseSub, cfs.config)
	if err != nil {
		return nil, err
	}

	return absfs.FilerToFS(compressSubFS, dir)
}

// renamedDirEntry wraps a DirEntry with a different name
type renamedDirEntry struct {
	fs.DirEntry
	name string
}

func (e *renamedDirEntry) Name() string {
	return e.name
}

// incrementStat atomically increments a stat counter
func (cfs *FS) incrementStat(counter *int64) {
	atomic.AddInt64(counter, 1)
}

// addBytes atomically adds to a byte counter
func (cfs *FS) addBytes(counter *int64, n int64) {
	atomic.AddInt64(counter, n)
}
