package compressfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/absfs/absfs"
)

// normalizePath normalizes a path for consistent storage/lookup
// It removes leading slashes and cleans the path
func normalizePath(name string) string {
	// Clean the path first
	name = filepath.Clean(name)
	// Remove leading slashes to normalize absolute and relative paths
	name = strings.TrimPrefix(name, "/")
	name = strings.TrimPrefix(name, string(filepath.Separator))
	// Handle empty path (root directory)
	if name == "" || name == "." {
		name = "."
	}
	return name
}

// memFS is a simple in-memory filesystem for testing
type memFS struct {
	files map[string]*memFile
	mu    sync.RWMutex
}

// NewMemFS creates a new in-memory filesystem
func NewMemFS() absfs.Filer {
	return &memFS{
		files: make(map[string]*memFile),
	}
}

type memFile struct {
	name    string
	data    *bytes.Buffer
	mode    fs.FileMode
	modTime time.Time
	pos     int64
	closed  bool
	mu      sync.Mutex
}

func (mfs *memFS) Open(name string) (absfs.File, error) {
	return mfs.OpenFile(name, os.O_RDONLY, 0)
}

func (mfs *memFS) OpenFile(name string, flag int, perm fs.FileMode) (absfs.File, error) {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = normalizePath(name)

	// Handle directory open (for ReadDir support)
	if name == "." || name == "" {
		return &memDir{mfs: mfs, name: "."}, nil
	}

	// Handle creation
	if flag&os.O_CREATE != 0 {
		if _, exists := mfs.files[name]; !exists {
			mfs.files[name] = &memFile{
				name:    name,
				data:    new(bytes.Buffer),
				mode:    perm,
				modTime: time.Now(),
			}
		}
	}

	// Check if file exists
	mf, exists := mfs.files[name]
	if !exists {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	// Handle truncate
	if flag&os.O_TRUNC != 0 {
		mf.data.Reset()
		mf.modTime = time.Now()
	}

	// Create a copy for this file handle
	handle := &memFile{
		name:    mf.name,
		data:    mf.data,
		mode:    mf.mode,
		modTime: mf.modTime,
		pos:     0,
	}

	// Set position to end if append mode
	if flag&os.O_APPEND != 0 {
		handle.pos = int64(mf.data.Len())
	}

	return handle, nil
}

func (mfs *memFS) Create(name string) (absfs.File, error) {
	return mfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

func (mfs *memFS) Mkdir(name string, perm fs.FileMode) error {
	// Simple implementation - just track that dir exists
	return nil
}

func (mfs *memFS) Remove(name string) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = normalizePath(name)
	if _, exists := mfs.files[name]; !exists {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}

	delete(mfs.files, name)
	return nil
}

func (mfs *memFS) Stat(name string) (fs.FileInfo, error) {
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()

	name = normalizePath(name)
	mf, exists := mfs.files[name]
	if !exists {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	return &memFileInfo{
		name:    filepath.Base(mf.name),
		size:    int64(mf.data.Len()),
		mode:    mf.mode,
		modTime: mf.modTime,
	}, nil
}

func (mfs *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()

	name = normalizePath(name)
	var entries []fs.DirEntry

	for path := range mfs.files {
		dir := filepath.Dir(path)
		if dir == name {
			info, _ := mfs.Stat(path)
			entries = append(entries, fs.FileInfoToDirEntry(info))
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

// Rename renames a file
func (mfs *memFS) Rename(oldpath, newpath string) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	oldpath = normalizePath(oldpath)
	newpath = normalizePath(newpath)

	mf, exists := mfs.files[oldpath]
	if !exists {
		return &fs.PathError{Op: "rename", Path: oldpath, Err: fs.ErrNotExist}
	}

	// Copy file to new location
	newFile := &memFile{
		name:    newpath,
		data:    mf.data,
		mode:    mf.mode,
		modTime: time.Now(),
	}
	mfs.files[newpath] = newFile
	delete(mfs.files, oldpath)

	return nil
}

// Chmod changes file permissions
func (mfs *memFS) Chmod(name string, mode os.FileMode) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = normalizePath(name)
	mf, exists := mfs.files[name]
	if !exists {
		return &fs.PathError{Op: "chmod", Path: name, Err: fs.ErrNotExist}
	}

	mf.mode = mode
	return nil
}

// Chtimes changes file access and modification times
func (mfs *memFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = normalizePath(name)
	mf, exists := mfs.files[name]
	if !exists {
		return &fs.PathError{Op: "chtimes", Path: name, Err: fs.ErrNotExist}
	}

	mf.modTime = mtime
	return nil
}

// Chown changes file owner (no-op for memFS)
func (mfs *memFS) Chown(name string, uid, gid int) error {
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()

	name = normalizePath(name)
	if _, exists := mfs.files[name]; !exists {
		return &fs.PathError{Op: "chown", Path: name, Err: fs.ErrNotExist}
	}

	// No-op for in-memory filesystem
	return nil
}

func (mf *memFile) Read(p []byte) (n int, err error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return 0, fs.ErrClosed
	}

	// Read from current position
	if mf.pos >= int64(mf.data.Len()) {
		return 0, io.EOF
	}

	data := mf.data.Bytes()[mf.pos:]
	n = copy(p, data)
	mf.pos += int64(n)
	return n, nil
}

func (mf *memFile) Write(p []byte) (n int, err error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return 0, fs.ErrClosed
	}

	// For simplicity, append to buffer
	n, err = mf.data.Write(p)
	mf.modTime = time.Now()
	return n, err
}

func (mf *memFile) Close() error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return nil
	}
	mf.closed = true
	return nil
}

func (mf *memFile) Seek(offset int64, whence int) (int64, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return 0, fs.ErrClosed
	}

	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = mf.pos + offset
	case io.SeekEnd:
		newPos = int64(mf.data.Len()) + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if newPos < 0 {
		return 0, errors.New("negative position")
	}

	mf.pos = newPos
	return newPos, nil
}

func (mf *memFile) Stat() (fs.FileInfo, error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	return &memFileInfo{
		name:    filepath.Base(mf.name),
		size:    int64(mf.data.Len()),
		mode:    mf.mode,
		modTime: mf.modTime,
	}, nil
}

func (mf *memFile) Sync() error {
	return nil
}

type memFileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
}

func (fi *memFileInfo) Name() string       { return fi.name }
func (fi *memFileInfo) Size() int64        { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi *memFileInfo) ModTime() time.Time { return fi.modTime }
func (fi *memFileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi *memFileInfo) Sys() interface{}   { return nil }

// ============================================================================
// absfs.File interface methods for memFile
// ============================================================================

// Name returns the name of the file
func (mf *memFile) Name() string {
	return mf.name
}

// ReadAt reads len(b) bytes from the File starting at byte offset off
func (mf *memFile) ReadAt(b []byte, off int64) (n int, err error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return 0, fs.ErrClosed
	}

	if off < 0 {
		return 0, errors.New("negative offset")
	}

	data := mf.data.Bytes()
	if off >= int64(len(data)) {
		return 0, io.EOF
	}

	n = copy(b, data[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// WriteAt writes len(b) bytes to the File starting at byte offset off
func (mf *memFile) WriteAt(b []byte, off int64) (n int, err error) {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return 0, fs.ErrClosed
	}

	if off < 0 {
		return 0, errors.New("negative offset")
	}

	// Expand buffer if needed
	data := mf.data.Bytes()
	needed := int(off) + len(b)
	if needed > len(data) {
		// Create new buffer with expanded size
		newData := make([]byte, needed)
		copy(newData, data)
		mf.data = bytes.NewBuffer(newData)
	}

	// Write at offset
	data = mf.data.Bytes()
	n = copy(data[off:], b)
	mf.modTime = time.Now()
	return n, nil
}

// WriteString writes a string to the file
func (mf *memFile) WriteString(s string) (n int, err error) {
	return mf.Write([]byte(s))
}

// Truncate changes the size of the file
func (mf *memFile) Truncate(size int64) error {
	mf.mu.Lock()
	defer mf.mu.Unlock()

	if mf.closed {
		return fs.ErrClosed
	}

	data := mf.data.Bytes()
	if size < int64(len(data)) {
		// Truncate to smaller size
		mf.data = bytes.NewBuffer(data[:size])
	} else if size > int64(len(data)) {
		// Expand with zeros
		newData := make([]byte, size)
		copy(newData, data)
		mf.data = bytes.NewBuffer(newData)
	}

	mf.modTime = time.Now()
	return nil
}

// Readdir reads directory contents
func (mf *memFile) Readdir(n int) ([]os.FileInfo, error) {
	// Not a directory
	return nil, os.ErrInvalid
}

// Readdirnames reads directory entry names
func (mf *memFile) Readdirnames(n int) ([]string, error) {
	// Not a directory
	return nil, os.ErrInvalid
}

// ============================================================================
// memDir - Virtual directory implementation for memFS
// ============================================================================

// memDir represents a virtual directory in memFS
type memDir struct {
	mfs  *memFS
	name string
}

func (md *memDir) Read(p []byte) (n int, err error) {
	return 0, os.ErrInvalid
}

func (md *memDir) Write(p []byte) (n int, err error) {
	return 0, os.ErrInvalid
}

func (md *memDir) Close() error {
	return nil
}

func (md *memDir) Seek(offset int64, whence int) (int64, error) {
	return 0, os.ErrInvalid
}

func (md *memDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{
		name:    md.name,
		size:    0,
		mode:    fs.ModeDir | 0755,
		modTime: time.Now(),
	}, nil
}

func (md *memDir) Sync() error {
	return nil
}

func (md *memDir) Name() string {
	return md.name
}

func (md *memDir) ReadAt(b []byte, off int64) (n int, err error) {
	return 0, os.ErrInvalid
}

func (md *memDir) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, os.ErrInvalid
}

func (md *memDir) WriteString(s string) (n int, err error) {
	return 0, os.ErrInvalid
}

func (md *memDir) Truncate(size int64) error {
	return os.ErrInvalid
}

func (md *memDir) Readdir(n int) ([]os.FileInfo, error) {
	md.mfs.mu.RLock()
	defer md.mfs.mu.RUnlock()

	var infos []os.FileInfo
	dirPath := normalizePath(md.name)

	for path, mf := range md.mfs.files {
		// Check if file is in this directory
		dir := filepath.Dir(path)
		if dir == dirPath || (dirPath == "." && dir == ".") {
			infos = append(infos, &memFileInfo{
				name:    filepath.Base(path),
				size:    int64(mf.data.Len()),
				mode:    mf.mode,
				modTime: mf.modTime,
			})
		}
	}

	if n > 0 && len(infos) > n {
		infos = infos[:n]
	}

	return infos, nil
}

func (md *memDir) Readdirnames(n int) ([]string, error) {
	md.mfs.mu.RLock()
	defer md.mfs.mu.RUnlock()

	var names []string
	dirPath := normalizePath(md.name)

	for path := range md.mfs.files {
		// Check if file is in this directory
		dir := filepath.Dir(path)
		if dir == dirPath || (dirPath == "." && dir == ".") {
			names = append(names, filepath.Base(path))
		}
	}

	if n > 0 && len(names) > n {
		names = names[:n]
	}

	return names, nil
}
