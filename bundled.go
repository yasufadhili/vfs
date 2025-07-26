package vfs

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BundledManager manages embedded filesystems with different prefixes
type BundledManager struct {
	bundled map[string]*BundledFS
	mu      sync.RWMutex
}

// NewBundledManager creates a new bundled filesystem manager
func NewBundledManager() *BundledManager {
	return &BundledManager{
		bundled: make(map[string]*BundledFS),
	}
}

// Register registers an embedded filesystem with a given prefix
func (bm *BundledManager) Register(prefix string, embedFS embed.FS, subdir string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if !strings.HasSuffix(prefix, "://") {
		prefix += "://"
	}

	bundled := &BundledFS{
		embedFS: embedFS,
		prefix:  strings.TrimSuffix(prefix, "://"),
		subdir:  subdir,
	}

	bm.bundled[prefix] = bundled
	return nil
}

// GetBundledFS returns the appropriate bundled filesystem for a path
func (bm *BundledManager) GetBundledFS(path string) (*BundledFS, string, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for prefix, bundled := range bm.bundled {
		if strings.HasPrefix(path, prefix) {
			return bundled, strings.TrimPrefix(path, prefix), true
		}
	}
	return nil, "", false
}

// IsBundledPath checks if a path is a bundled path
func (bm *BundledManager) IsBundledPath(path string) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	for prefix := range bm.bundled {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// ListRegistered returns all registered prefixes
func (bm *BundledManager) ListRegistered() []string {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	prefixes := make([]string, 0, len(bm.bundled))
	for prefix := range bm.bundled {
		prefixes = append(prefixes, strings.TrimSuffix(prefix, "://"))
	}
	return prefixes
}

// BundledFS handles embedded filesystem access
type BundledFS struct {
	embedFS embed.FS
	prefix  string
	subdir  string
}

// ReadFile reads from the embedded filesystem
func (b *BundledFS) ReadFile(path string) ([]byte, error) {
	fullPath := b.getFullPath(path)
	return fs.ReadFile(b.embedFS, fullPath)
}

// Exists checks if a file exists in the embedded filesystem
func (b *BundledFS) Exists(path string) bool {
	fullPath := b.getFullPath(path)
	_, err := fs.Stat(b.embedFS, fullPath)
	return err == nil
}

// IsDir checks if a path is a directory in the embedded filesystem
func (b *BundledFS) IsDir(path string) bool {
	fullPath := b.getFullPath(path)
	stat, err := fs.Stat(b.embedFS, fullPath)
	return err == nil && stat.IsDir()
}

// Stat returns file info for embedded files
func (b *BundledFS) Stat(path string) (fs.FileInfo, error) {
	fullPath := b.getFullPath(path)
	return fs.Stat(b.embedFS, fullPath)
}

// ListFiles lists files in an embedded directory
func (b *BundledFS) ListFiles(path string) ([]string, error) {
	fullPath := b.getFullPath(path)
	entries, err := fs.ReadDir(b.embedFS, fullPath)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// ListDirs lists directories in an embedded directory
func (b *BundledFS) ListDirs(path string) ([]string, error) {
	fullPath := b.getFullPath(path)
	entries, err := fs.ReadDir(b.embedFS, fullPath)
	if err != nil {
		return nil, err
	}

	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	return dirs, nil
}

// Walk traverses the embedded filesystem
func (b *BundledFS) Walk(root string, walkFn filepath.WalkFunc) error {
	fullRoot := b.getFullPath(root)

	return fs.WalkDir(b.embedFS, fullRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return walkFn(path, nil, err)
		}

		info, err := d.Info()
		if err != nil {
			return walkFn(path, nil, err)
		}

		// Convert back to the original path format
		originalPath := b.getOriginalPath(path)
		bundledURL := fmt.Sprintf("%s://%s", b.prefix, originalPath)

		return walkFn(bundledURL, info, nil)
	})
}

// getFullPath constructs the full path within the embedded filesystem
func (b *BundledFS) getFullPath(path string) string {
	if b.subdir == "" {
		return path
	}
	return filepath.Join(b.subdir, path)
}

// getOriginalPath converts a full embedded path back to the original format
func (b *BundledFS) getOriginalPath(fullPath string) string {
	if b.subdir == "" {
		return fullPath
	}
	return strings.TrimPrefix(fullPath, b.subdir+"/")
}

// FileInfo implements fs.FileInfo for bundled files
type FileInfo struct {
	name    string
	size    int64
	mode    fs.FileMode
	modTime time.Time
	isDir   bool
}

func (fi FileInfo) Name() string       { return fi.name }
func (fi FileInfo) Size() int64        { return fi.size }
func (fi FileInfo) Mode() fs.FileMode  { return fi.mode }
func (fi FileInfo) ModTime() time.Time { return fi.modTime }
func (fi FileInfo) IsDir() bool        { return fi.isDir }
func (fi FileInfo) Sys() interface{}   { return nil }
