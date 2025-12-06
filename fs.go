package compressfs

import (
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

	// Open directory and read entries
	f, err := cfs.base.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	// Convert names to DirEntry
	entries := make([]fs.DirEntry, 0, len(names))
	for _, entryName := range names {
		fullPath := name + string(cfs.Separator()) + entryName
		info, err := cfs.base.Stat(fullPath)
		if err != nil {
			continue // Skip entries we can't stat
		}
		entries = append(entries, fs.FileInfoToDirEntry(info))
	}

	// If StripExtension is enabled, remove compression extensions from names
	if config.StripExtension {
		result := make([]fs.DirEntry, 0, len(entries))
		seen := make(map[string]bool)

		for _, entry := range entries {
			name := entry.Name()
			stripped, _, hasCompExt := StripExtension(name)

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
				if !seen[name] {
					result = append(result, entry)
					seen[name] = true
				}
			}
		}
		return result, nil
	}

	return entries, nil
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
