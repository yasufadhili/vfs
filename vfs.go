package vfs

import (
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/spf13/afero"
)

// FileSystem interface defines the core VFS operations
type FileSystem interface {
	// File operations
	ReadFile(filename string) ([]byte, error)
	ReadFileString(filename string) (string, error)
	WriteFile(filename string, data []byte, perm fs.FileMode) error

	// Directory operations
	MkdirAll(path string, perm fs.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error

	// File system queries
	Exists(path string) bool
	IsDir(path string) bool
	Stat(path string) (fs.FileInfo, error)

	// Directory listing
	ListFiles(dir string) ([]string, error)
	ListDirs(dir string) ([]string, error)
	Walk(root string, walkFn filepath.WalkFunc) error

	// File operations
	Open(path string) (afero.File, error)
	Create(path string) (afero.File, error)

	// Utility functions
	FindFiles(root, pattern string) ([]string, error)
	Copy(src, dst string) error
	Move(src, dst string) error

	// Disk integration
	LoadFromDisk(srcPath, destPath string) error
	SaveToDisk(srcPath, destPath string) error

	// Advanced operations
	Clone() FileSystem
	Merge(other FileSystem, destPath string) error
}

// WatchableFileSystem extends FileSystem with watching capabilities
type WatchableFileSystem interface {
	FileSystem

	// Watch operations
	Watch(path string, action WatchAction) error
	StopWatch(path string) error
	StopAllWatches() error
	IsWatching(path string) bool
}

// WatchAction defines the callback for file system events
type WatchAction func(event WatchEvent)

// WatchEvent represents a file system event
type WatchEvent struct {
	Path  string
	Op    WatchOp
	IsDir bool
	Error error
}

// WatchOp represents the type of file system operation
type WatchOp int

const (
	WatchOpCreate WatchOp = iota
	WatchOpWrite
	WatchOpRemove
	WatchOpRename
	WatchOpChmod
)

func (op WatchOp) String() string {
	switch op {
	case WatchOpCreate:
		return "CREATE"
	case WatchOpWrite:
		return "WRITE"
	case WatchOpRemove:
		return "REMOVE"
	case WatchOpRename:
		return "RENAME"
	case WatchOpChmod:
		return "CHMOD"
	default:
		return "UNKNOWN"
	}
}

// VFSType represents the type of VFS implementation
type VFSType int

const (
	VFSTypeMemory VFSType = iota
	VFSTypeDisk
	VFSTypeHybrid
)

// Logger interface for optional logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// NullLogger is a no-op logger
type NullLogger struct{}

func (n NullLogger) Debug(msg string, args ...interface{}) {}
func (n NullLogger) Info(msg string, args ...interface{})  {}
func (n NullLogger) Error(msg string, args ...interface{}) {}

// Option configures VFS creation
type Option func(*VFS)

// WithLogger sets a custom logger
func WithLogger(logger Logger) Option {
	return func(v *VFS) {
		v.logger = logger
	}
}

// WithRoot sets a custom root path
func WithRoot(root string) Option {
	return func(v *VFS) {
		v.root = filepath.Clean(root)
	}
}

// WithType sets the VFS type
func WithType(vfsType VFSType) Option {
	return func(v *VFS) {
		v.vfsType = vfsType
	}
}

// Factory functions for different VFS types

// NewMemoryVFS creates a pure in-memory VFS
func NewMemoryVFS(opts ...Option) *VFS {
	opts = append(opts, WithType(VFSTypeMemory))
	return New(opts...)
}

// NewDiskVFS creates a disk-based VFS with optional watching
func NewDiskVFS(rootPath string, opts ...Option) *VFS {
	opts = append(opts, WithType(VFSTypeDisk), WithRoot(rootPath))
	return New(opts...)
}

// NewHybridVFS creates a hybrid VFS (memory + bundled resources)
func NewHybridVFS(opts ...Option) *VFS {
	opts = append(opts, WithType(VFSTypeHybrid))
	return New(opts...)
}

// RegisterBundled registers an embedded filesystem with a given prefix
func (v *VFS) RegisterBundled(prefix string, embedFS embed.FS, subdir string) error {
	return v.bundledManager.Register(prefix, embedFS, subdir)
}
