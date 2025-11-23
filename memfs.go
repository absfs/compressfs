package compressfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// memFS is a simple in-memory filesystem for testing
type memFS struct {
	files map[string]*memFile
	mu    sync.RWMutex
}

// NewMemFS creates a new in-memory filesystem
func NewMemFS() FileSystem {
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

func (mfs *memFS) Open(name string) (File, error) {
	return mfs.OpenFile(name, os.O_RDONLY, 0)
}

func (mfs *memFS) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = filepath.Clean(name)

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

func (mfs *memFS) Create(name string) (File, error) {
	return mfs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

func (mfs *memFS) Mkdir(name string, perm fs.FileMode) error {
	// Simple implementation - just track that dir exists
	return nil
}

func (mfs *memFS) Remove(name string) error {
	mfs.mu.Lock()
	defer mfs.mu.Unlock()

	name = filepath.Clean(name)
	if _, exists := mfs.files[name]; !exists {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}

	delete(mfs.files, name)
	return nil
}

func (mfs *memFS) Stat(name string) (fs.FileInfo, error) {
	mfs.mu.RLock()
	defer mfs.mu.RUnlock()

	name = filepath.Clean(name)
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

	name = filepath.Clean(name)
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
