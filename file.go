package compressfs

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/absfs/absfs"
)

// compressedFile wraps a file with compression/decompression
type compressedFile struct {
	cfs  *FS
	base absfs.File
	flag int

	// Original and compressed names
	originalName   string
	compressedName string

	// Compression state (write mode)
	writeBuffer    *bytes.Buffer
	compressor     io.WriteCloser
	writeAlgo      Algorithm
	writeLevel     int
	shouldCompress bool

	// Decompression state (read mode)
	decompressor io.ReadCloser
	readAlgo     Algorithm

	// Metadata
	bytesRead    int64
	bytesWritten int64
	closed       bool
	mu           sync.Mutex
}

// newCompressedFile creates a new compressed file wrapper
func newCompressedFile(cfs *FS, base absfs.File, originalName, compressedName string, flag int, algo Algorithm) (*compressedFile, error) {
	cf := &compressedFile{
		cfs:            cfs,
		base:           base,
		flag:           flag,
		originalName:   originalName,
		compressedName: compressedName,
		writeAlgo:      algo,
		readAlgo:       algo,
	}

	var isCreate = (flag & os.O_CREATE) != 0
	var isWrite = (flag & (os.O_WRONLY | os.O_RDWR | os.O_CREATE)) != 0
	var isReadOnly = (flag & (os.O_WRONLY | os.O_RDWR)) == 0

	// Determine if we should compress
	cf.shouldCompress = !cfs.shouldSkip(originalName) && algo != ""

	// Setup for writing
	if isWrite && cf.shouldCompress {
		cf.writeBuffer = new(bytes.Buffer)

		// Select algorithm and level based on rules/auto-tuning
		// We'll determine the final algorithm and level at close time when we know the file size
		if algo == "" {
			// For now, use default - we'll re-evaluate at close time
			algo, level, _ := cfs.selectAlgorithm(originalName, 0)
			cf.writeAlgo = algo
			cf.writeLevel = level
		} else {
			cf.writeAlgo = algo
			cf.writeLevel = cfs.config.Level
		}
	}

	// Setup for reading (not on create operations)
	if isReadOnly && !isCreate {
		// Check if file is empty first
		info, err := cf.base.Stat()
		isEmpty := err == nil && info.Size() == 0

		if !isEmpty && algo != "" && cf.shouldCompress {
			// We have a known algorithm from the file extension
			// Check magic bytes to verify the file is actually compressed
			magicBuf := make([]byte, 10)
			n, magicErr := cf.base.Read(magicBuf)
			if magicErr != nil && magicErr != io.EOF {
				// Read error, treat as uncompressed
				cf.shouldCompress = false
			} else {
				// Check if the data is actually compressed
				detectedAlgo, isCompressed := IsCompressed(magicBuf[:n])

				// Seek back to start for reading
				if _, seekErr := cf.base.Seek(0, io.SeekStart); seekErr != nil {
					// Can't seek, treat as uncompressed
					cf.shouldCompress = false
				} else {
					// Determine which algorithm to use:
					// 1. If we detected compression via magic bytes, use that
					// 2. If we didn't detect via magic bytes:
					//    a. For brotli/snappy, trust the extension (no reliable magic bytes)
					//    b. For other formats, file might be uncompressed (MinSize skip)
					useAlgo := algo
					shouldDecompress := isCompressed

					if isCompressed && detectedAlgo != "" {
						// Magic bytes matched, use detected algorithm
						useAlgo = detectedAlgo
					} else if !isCompressed {
						// Magic bytes didn't match
						// For brotli and snappy, trust the extension (no reliable magic bytes)
						// For gzip, zstd, lz4 - they have magic bytes, so file is truly uncompressed
						switch algo {
						case AlgorithmBrotli, AlgorithmSnappy:
							shouldDecompress = true // Trust extension for these formats
						default:
							shouldDecompress = false // Has magic bytes but didn't match
						}
					}

					if shouldDecompress {
						var decompressor io.ReadCloser
						var err error

						// Use dictionary if available for zstd
						if useAlgo == AlgorithmZstd && len(cfs.config.ZstdDictionary) > 0 {
							decompressor, err = createDecompressorWithDict(useAlgo, cf.base, cfs.config.Level, cfs.config.ZstdDictionary)
						} else {
							decompressor, err = createDecompressor(useAlgo, cf.base, cfs.config.Level)
						}

						if err != nil {
							// Failed to create decompressor, read uncompressed
							cf.shouldCompress = false
						} else {
							cf.decompressor = decompressor
							cf.readAlgo = useAlgo
						}
					} else {
						cf.shouldCompress = false
					}
				}
			}
		} else if !isEmpty && cfs.config.AutoDetect {
			// Try to detect algorithm
			if err := cf.detectAndSetupDecompressor(); err != nil {
				// If detection fails, try to read uncompressed
				cf.shouldCompress = false
			}
		}
		// If file is empty, don't set up decompressor - just read as empty
	}

	return cf, nil
}

// detectAndSetupDecompressor detects compression algorithm and sets up decompressor
func (cf *compressedFile) detectAndSetupDecompressor() error {
	// Read magic bytes
	buf := make([]byte, 10)
	n, err := cf.base.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}

	if n == 0 {
		return nil // Empty file
	}

	// Detect algorithm
	algo, detected := IsCompressed(buf[:n])
	if !detected {
		// Not compressed, read as-is
		cf.shouldCompress = false
		// Seek back to start
		if _, err := cf.base.Seek(0, io.SeekStart); err != nil {
			return err
		}
		return nil
	}

	cf.readAlgo = algo

	// Seek back to start for decompressor
	if _, err := cf.base.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// Create decompressor with dictionary support
	var decompressor io.ReadCloser
	if algo == AlgorithmZstd && len(cf.cfs.config.ZstdDictionary) > 0 {
		decompressor, err = createDecompressorWithDict(algo, cf.base, cf.cfs.config.Level, cf.cfs.config.ZstdDictionary)
	} else {
		decompressor, err = createDecompressor(algo, cf.base, cf.cfs.config.Level)
	}

	if err != nil {
		return err
	}

	cf.decompressor = decompressor
	return nil
}

// Read reads from the file with decompression
func (cf *compressedFile) Read(p []byte) (n int, err error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return 0, fs.ErrClosed
	}

	// If decompressor is set up, read from it
	if cf.decompressor != nil {
		n, err = cf.decompressor.Read(p)
		if n > 0 {
			cf.bytesRead += int64(n)
			cf.cfs.addBytes(&cf.cfs.stats.BytesRead, int64(n))
			// If we read data, don't return EOF on the same call
			// This matches typical file reading behavior where EOF is returned
			// on the subsequent read after all data has been consumed
			if err == io.EOF {
				err = nil
			}
		}
		return n, err
	}

	// Otherwise read directly from base
	n, err = cf.base.Read(p)
	if n > 0 {
		cf.bytesRead += int64(n)
		cf.cfs.addBytes(&cf.cfs.stats.BytesRead, int64(n))
	}
	return n, err
}

// Write writes to the file with compression
func (cf *compressedFile) Write(p []byte) (n int, err error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return 0, fs.ErrClosed
	}

	// If we should compress, write to buffer
	if cf.shouldCompress && cf.writeBuffer != nil {
		n, err = cf.writeBuffer.Write(p)
		if n > 0 {
			cf.bytesWritten += int64(n)
		}
		return n, err
	}

	// Otherwise write directly to base
	n, err = cf.base.Write(p)
	if n > 0 {
		cf.bytesWritten += int64(n)
		cf.cfs.addBytes(&cf.cfs.stats.BytesWritten, int64(n))
	}
	return n, err
}

// Close closes the file and flushes compression if needed
func (cf *compressedFile) Close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return nil
	}
	cf.closed = true

	var err error

	// Flush compression on write
	if cf.shouldCompress && cf.writeBuffer != nil {
		bufLen := int64(cf.writeBuffer.Len())

		// Check minimum size and that buffer is not empty
		if bufLen > 0 && bufLen >= cf.cfs.config.MinSize {
			// Re-evaluate algorithm and level based on actual file size (auto-tuning)
			finalAlgo, finalLevel, _ := cf.cfs.selectAlgorithm(cf.originalName, bufLen)

			// Use the selected algorithm/level, or stick with what was determined earlier
			// if rules were used (rules take precedence over auto-tuning)
			if cf.writeLevel != 0 {
				// Use the level that was set (either from rules or initial selection)
				finalLevel = cf.writeLevel
			}

			// Create compressor with dictionary support
			var compressor io.WriteCloser
			var cerr error

			// Check if we should use dictionary (only for zstd)
			if finalAlgo == AlgorithmZstd && len(cf.cfs.config.ZstdDictionary) > 0 {
				compressor, cerr = createCompressorWithDict(finalAlgo, cf.base, finalLevel, cf.cfs.config.ZstdDictionary)
			} else {
				compressor, cerr = createCompressor(finalAlgo, cf.base, finalLevel)
			}

			if cerr != nil {
				cf.base.Close()
				return cerr
			}

			// Write buffered data through compressor
			_, cerr = io.Copy(compressor, cf.writeBuffer)
			if cerr != nil {
				compressor.Close()
				cf.base.Close()
				return cerr
			}

			// Close compressor
			if cerr = compressor.Close(); cerr != nil {
				cf.base.Close()
				return cerr
			}

			// Update stats
			cf.cfs.incrementStat(&cf.cfs.stats.FilesCompressed)
			cf.cfs.addBytes(&cf.cfs.stats.BytesWritten, cf.bytesWritten)
			cf.cfs.addBytes(&cf.cfs.stats.BytesCompressed, cf.bytesWritten)
			cf.cfs.stats.IncrementAlgorithmCount(finalAlgo)
		} else if bufLen > 0 {
			// File too small, write uncompressed
			_, err = io.Copy(cf.base, cf.writeBuffer)
			cf.cfs.incrementStat(&cf.cfs.stats.FilesSkipped)

			// If we have a compression extension but didn't compress,
			// rename the file to remove the extension to avoid confusion on read
			if cf.compressedName != cf.originalName && HasCompressionExtension(cf.compressedName) {
				// Close the base file before renaming
				if cerr := cf.base.Close(); cerr != nil && err == nil {
					err = cerr
				}

				// Rename from compressed name to original name
				if renameErr := cf.cfs.base.Rename(cf.compressedName, cf.originalName); renameErr != nil {
					// If rename fails, it's not critical - we can still read the file
					// It just might try to decompress it unnecessarily
				}

				return err // Already closed the base file
			}
		}
		// If bufLen == 0, it's an empty file - just close without writing anything
	}

	// Close decompressor if present
	if cf.decompressor != nil {
		if cerr := cf.decompressor.Close(); cerr != nil && err == nil {
			err = cerr
		}
		cf.cfs.incrementStat(&cf.cfs.stats.FilesDecompressed)
		cf.cfs.addBytes(&cf.cfs.stats.BytesDecompressed, cf.bytesRead)
		cf.cfs.stats.IncrementAlgorithmCount(cf.readAlgo)
	}

	// Close base file
	if cerr := cf.base.Close(); cerr != nil && err == nil {
		err = cerr
	}

	return err
}

// Seek seeks in the file (limited support for compressed files)
func (cf *compressedFile) Seek(offset int64, whence int) (int64, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return 0, fs.ErrClosed
	}

	// Seeking is not supported in compressed mode
	if cf.decompressor != nil || cf.compressor != nil {
		return 0, ErrSeekNotSupported
	}

	return cf.base.Seek(offset, whence)
}

// Stat returns file information
func (cf *compressedFile) Stat() (fs.FileInfo, error) {
	return cf.base.Stat()
}

// Sync syncs the file to disk
func (cf *compressedFile) Sync() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return fs.ErrClosed
	}

	return cf.base.Sync()
}

// Algorithm returns the compression algorithm being used
func (cf *compressedFile) Algorithm() Algorithm {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.decompressor != nil {
		return cf.readAlgo
	}
	if cf.compressor != nil || cf.writeBuffer != nil {
		return cf.writeAlgo
	}
	return ""
}

// CompressionRatio returns the compression ratio (0-1, lower is better)
func (cf *compressedFile) CompressionRatio() float64 {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.bytesWritten == 0 {
		return 0
	}

	// This is approximate, actual compressed size is not known until close
	return 0.5 // Placeholder
}

// OriginalSize returns the original uncompressed size
func (cf *compressedFile) OriginalSize() int64 {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.decompressor != nil {
		return cf.bytesRead
	}
	return cf.bytesWritten
}

// CompressedSize returns the compressed size (approximate)
func (cf *compressedFile) CompressedSize() int64 {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	// This is approximate until file is closed
	info, err := cf.base.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// ============================================================================
// absfs.File interface methods
// ============================================================================

// Name returns the name of the file
func (cf *compressedFile) Name() string {
	return cf.originalName
}

// ReadAt reads len(b) bytes from the File starting at byte offset off
func (cf *compressedFile) ReadAt(b []byte, off int64) (n int, err error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return 0, fs.ErrClosed
	}

	// ReadAt not supported for compressed files
	if cf.decompressor != nil {
		return 0, ErrSeekNotSupported
	}

	// Delegate to base file if it supports ReadAt
	if ra, ok := cf.base.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}

	return 0, os.ErrInvalid
}

// WriteAt writes len(b) bytes to the File starting at byte offset off
func (cf *compressedFile) WriteAt(b []byte, off int64) (n int, err error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return 0, fs.ErrClosed
	}

	// WriteAt not supported for compressed files
	if cf.compressor != nil || cf.writeBuffer != nil {
		return 0, ErrSeekNotSupported
	}

	// Delegate to base file if it supports WriteAt
	if wa, ok := cf.base.(io.WriterAt); ok {
		return wa.WriteAt(b, off)
	}

	return 0, os.ErrInvalid
}

// WriteString writes a string to the file
func (cf *compressedFile) WriteString(s string) (n int, err error) {
	return cf.Write([]byte(s))
}

// Truncate changes the size of the file
func (cf *compressedFile) Truncate(size int64) error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return fs.ErrClosed
	}

	// Truncate not supported for compressed files
	if cf.compressor != nil || cf.decompressor != nil {
		return ErrSeekNotSupported
	}

	// Delegate to base file if it supports Truncate
	type truncater interface {
		Truncate(int64) error
	}
	if t, ok := cf.base.(truncater); ok {
		return t.Truncate(size)
	}

	return os.ErrInvalid
}

// Readdir reads the contents of the directory associated with the file
func (cf *compressedFile) Readdir(n int) ([]os.FileInfo, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return nil, fs.ErrClosed
	}

	// Delegate to base file if it supports Readdir
	type readdirer interface {
		Readdir(int) ([]os.FileInfo, error)
	}
	if rd, ok := cf.base.(readdirer); ok {
		return rd.Readdir(n)
	}

	return nil, os.ErrInvalid
}

// Readdirnames reads the contents of the directory and returns the names
func (cf *compressedFile) Readdirnames(n int) (names []string, err error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return nil, fs.ErrClosed
	}

	// Delegate to base file if it supports Readdirnames
	type readdirnamer interface {
		Readdirnames(int) ([]string, error)
	}
	if rdn, ok := cf.base.(readdirnamer); ok {
		return rdn.Readdirnames(n)
	}

	return nil, os.ErrInvalid
}

// ReadDir reads the contents of the directory and returns a slice of DirEntry values.
func (cf *compressedFile) ReadDir(n int) ([]fs.DirEntry, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	if cf.closed {
		return nil, fs.ErrClosed
	}

	// Delegate to base file if it supports ReadDir
	if rd, ok := cf.base.(interface{ ReadDir(int) ([]fs.DirEntry, error) }); ok {
		return rd.ReadDir(n)
	}

	return nil, os.ErrInvalid
}
