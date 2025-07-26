package vfs

import (
	"fmt"
	"github.com/spf13/afero"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// VFS represents a virtual file system with support for bundled resources and watching
type VFS struct {
	fs             afero.Fs
	afero          *afero.Afero
	root           string
	vfsType        VFSType
	logger         Logger
	bundledManager *BundledManager
	watchManager   *WatchManager
	diskPath       string // For disk-based VFS
}

// New creates a new VFS instance
func New(opts ...Option) *VFS {
	vfs := &VFS{
		root:           "/",
		vfsType:        VFSTypeMemory,
		logger:         NullLogger{},
		bundledManager: NewBundledManager(),
	}

	// Apply options first to determine type
	for _, opt := range opts {
		opt(vfs)
	}

	// Initialize filesystem based on type
	switch vfs.vfsType {
	case VFSTypeMemory, VFSTypeHybrid:
		memFs := afero.NewMemMapFs()
		vfs.fs = memFs
		vfs.afero = &afero.Afero{Fs: memFs}
	case VFSTypeDisk:
		if vfs.root == "/" {
			vfs.root = "."
		}
		vfs.diskPath = vfs.root
		// Create a base directory filesystem rooted at diskPath
		baseFs := afero.NewBasePathFs(afero.NewOsFs(), vfs.diskPath)
		vfs.fs = baseFs
		vfs.afero = &afero.Afero{Fs: baseFs}
		vfs.watchManager = NewWatchManager(vfs.diskPath, vfs.logger)
	}

	vfs.logger.Debug("Created VFS with type: %v, root: %s", vfs.vfsType, vfs.root)
	return vfs
}

// Clone creates a deep copy of the VFS
func (v *VFS) Clone() FileSystem {
	clone := &VFS{
		root:           v.root,
		vfsType:        VFSTypeMemory, // Clones are always memory-based
		logger:         v.logger,
		bundledManager: v.bundledManager, // Share bundled resources
	}

	memFs := afero.NewMemMapFs()
	clone.fs = memFs
	clone.afero = &afero.Afero{Fs: memFs}

	// Copy all files from original to clone
	v.Walk("/", func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		// Skip bundled files as they're shared
		if v.bundledManager.IsBundledPath(path) {
			return nil
		}

		data, readErr := v.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		return clone.WriteFile(path, data, info.Mode())
	})

	clone.logger.Debug("Created clone of VFS")
	return clone
}

// Merge merges another filesystem into this one at the specified destination path
func (v *VFS) Merge(other FileSystem, destPath string) error {
	return other.Walk("/", func(path string, info fs.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		data, readErr := other.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		// Calculate destination path
		relPath := strings.TrimPrefix(path, "/")
		mergePath := filepath.Join(destPath, relPath)

		// Ensure directory exists
		if err := v.MkdirAll(filepath.Dir(mergePath), 0755); err != nil {
			return err
		}

		return v.WriteFile(mergePath, data, info.Mode())
	})
}

// normalizePath ensures path is absolute within the VFS
func (v *VFS) normalizePath(path string) string {
	if v.bundledManager.IsBundledPath(path) {
		return path // Return bundled paths as-is
	}

	if !filepath.IsAbs(path) {
		return filepath.Join("/", path)
	}
	return path
}

// ReadFile reads a file from either bundled, disk, or memory storage
func (v *VFS) ReadFile(filename string) ([]byte, error) {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(filename); ok {
		return bundled.ReadFile(bundledPath)
	}

	vfsPath := v.normalizePath(filename)
	data, err := v.afero.ReadFile(vfsPath)
	if err != nil {
		v.logger.Error("Failed to read file %s: %v", filename, err)
	}
	return data, err
}

// ReadFileString reads a file as a string
func (v *VFS) ReadFileString(filename string) (string, error) {
	data, err := v.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes data to a file
func (v *VFS) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	if v.bundledManager.IsBundledPath(filename) {
		return fmt.Errorf("cannot write to bundled URL: %s", filename)
	}

	vfsPath := v.normalizePath(filename)

	// Ensure directory exists
	if err := v.afero.MkdirAll(filepath.Dir(vfsPath), 0755); err != nil {
		return err
	}

	err := v.afero.WriteFile(vfsPath, data, perm)
	if err != nil {
		v.logger.Error("Failed to write file %s: %v", filename, err)
	} else {
		v.logger.Debug("Successfully wrote file: %s", filename)
	}
	return err
}

// Exists checks if a path exists
func (v *VFS) Exists(path string) bool {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(path); ok {
		return bundled.Exists(bundledPath)
	}

	vfsPath := v.normalizePath(path)
	exists, _ := v.afero.Exists(vfsPath)
	return exists
}

// IsDir checks if a path is a directory
func (v *VFS) IsDir(path string) bool {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(path); ok {
		return bundled.IsDir(bundledPath)
	}

	vfsPath := v.normalizePath(path)
	info, err := v.afero.Stat(vfsPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// MkdirAll creates directories recursively
func (v *VFS) MkdirAll(path string, perm fs.FileMode) error {
	if v.bundledManager.IsBundledPath(path) {
		return fmt.Errorf("cannot create directories in bundled URL: %s", path)
	}

	vfsPath := v.normalizePath(path)
	err := v.afero.MkdirAll(vfsPath, perm)
	if err != nil {
		v.logger.Error("Failed to create directory %s: %v", path, err)
	}
	return err
}

// Remove removes a file or directory
func (v *VFS) Remove(path string) error {
	if v.bundledManager.IsBundledPath(path) {
		return fmt.Errorf("cannot remove bundled URL: %s", path)
	}

	vfsPath := v.normalizePath(path)
	return v.afero.Remove(vfsPath)
}

// RemoveAll removes a path recursively
func (v *VFS) RemoveAll(path string) error {
	if v.bundledManager.IsBundledPath(path) {
		return fmt.Errorf("cannot remove bundled URL: %s", path)
	}

	vfsPath := v.normalizePath(path)
	return v.afero.RemoveAll(vfsPath)
}

// Stat returns file information
func (v *VFS) Stat(path string) (fs.FileInfo, error) {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(path); ok {
		return bundled.Stat(bundledPath)
	}

	vfsPath := v.normalizePath(path)
	return v.afero.Stat(vfsPath)
}

// Open opens a file for reading
func (v *VFS) Open(path string) (afero.File, error) {
	if v.bundledManager.IsBundledPath(path) {
		return nil, fmt.Errorf("open not implemented for bundled URLs")
	}

	vfsPath := v.normalizePath(path)
	return v.fs.Open(vfsPath)
}

// Create creates a file for writing
func (v *VFS) Create(path string) (afero.File, error) {
	if v.bundledManager.IsBundledPath(path) {
		return nil, fmt.Errorf("cannot create files with bundled URL: %s", path)
	}

	vfsPath := v.normalizePath(path)
	return v.fs.Create(vfsPath)
}

// Walk traverses the filesystem
func (v *VFS) Walk(root string, walkFn filepath.WalkFunc) error {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(root); ok {
		return bundled.Walk(bundledPath, walkFn)
	}

	vfsRoot := v.normalizePath(root)
	return afero.Walk(v.fs, vfsRoot, walkFn)
}

// ListFiles lists files in a directory
func (v *VFS) ListFiles(dir string) ([]string, error) {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(dir); ok {
		return bundled.ListFiles(bundledPath)
	}

	var files []string
	vfsDir := v.normalizePath(dir)

	entries, err := afero.ReadDir(v.fs, vfsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// ListDirs lists directories in a directory
func (v *VFS) ListDirs(dir string) ([]string, error) {
	if bundled, bundledPath, ok := v.bundledManager.GetBundledFS(dir); ok {
		return bundled.ListDirs(bundledPath)
	}

	var dirs []string
	vfsDir := v.normalizePath(dir)

	entries, err := afero.ReadDir(v.fs, vfsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}

	return dirs, nil
}

// FindFiles recursively finds files matching a pattern
func (v *VFS) FindFiles(root, pattern string) ([]string, error) {
	var matches []string

	err := v.Walk(root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			matched, err := filepath.Match(pattern, filepath.Base(path))
			if err != nil {
				return err
			}
			if matched {
				matches = append(matches, path)
			}
		}

		return nil
	})

	return matches, err
}

// Copy copies a file from src to dst
func (v *VFS) Copy(src, dst string) error {
	data, err := v.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file %s: %w", src, err)
	}

	info, err := v.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source file %s: %w", src, err)
	}

	return v.WriteFile(dst, data, info.Mode())
}

// Move moves a file from src to dst
func (v *VFS) Move(src, dst string) error {
	if err := v.Copy(src, dst); err != nil {
		return err
	}
	return v.Remove(src)
}

// LoadFromDisk loads files from the OS filesystem
func (v *VFS) LoadFromDisk(srcPath, destPath string) error {
	realFs := afero.NewOsFs()

	return afero.Walk(realFs, srcPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}

		vfsPath := filepath.Join(destPath, relPath)

		if info.IsDir() {
			return v.MkdirAll(vfsPath, info.Mode())
		}

		content, err := afero.ReadFile(realFs, path)
		if err != nil {
			return err
		}

		return v.WriteFile(vfsPath, content, info.Mode())
	})
}

// SaveToDisk saves VFS contents to disk
func (v *VFS) SaveToDisk(srcPath, destPath string) error {
	if v.bundledManager.IsBundledPath(srcPath) {
		return fmt.Errorf("cannot save bundled URLs to disk directly")
	}

	realFs := afero.NewOsFs()
	vfsSrcPath := v.normalizePath(srcPath)

	return v.Walk(vfsSrcPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(vfsSrcPath, path)
		if err != nil {
			return err
		}

		diskPath := filepath.Join(destPath, relPath)

		if info.IsDir() {
			return realFs.MkdirAll(diskPath, info.Mode())
		}

		content, err := v.ReadFile(path)
		if err != nil {
			return err
		}

		return afero.WriteFile(realFs, diskPath, content, info.Mode())
	})
}

func (v *VFS) Dump(writer io.Writer) error {
	// --- Dump the primary filesystem (memory or disk) ---
	io.WriteString(writer, "--- VFS Root ---\n")

	// Use a map to build a tree to sort it nicely
	tree := make(map[string][]string)
	err := v.Walk("/", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == "/" { // Skip the root itself
			return nil
		}

		parent := filepath.Dir(path)
		if parent == "." {
			parent = "/"
		}
		tree[parent] = append(tree[parent], path)
		return nil
	})

	if err != nil {
		// Handle cases where the root might not exist yet in an empty VFS
		if _, ok := err.(*fs.PathError); !ok {
			return err
		}
	}

	printTree(writer, tree, "/", "")

	// --- Dump each bundled filesystem ---
	registeredPrefixes := v.bundledManager.ListRegistered()
	if len(registeredPrefixes) > 0 {
		io.WriteString(writer, "\n--- Bundled Filesystems ---\n")

		for _, prefix := range registeredPrefixes {
			io.WriteString(writer, fmt.Sprintf("Bundle [%s://]:\n", prefix))

			bundleTree := make(map[string][]string)
			bundledFS, _, ok := v.bundledManager.GetBundledFS(prefix + "://")
			if !ok {
				continue
			}

			// Walk the bundled filesystem starting from root
			bundledFS.Walk("", func(path string, info fs.FileInfo, err error) error {
				if err != nil {
					return err
				}

				// Skip the prefix part and get the clean path
				cleanPath := strings.TrimPrefix(path, prefix+"://")
				if cleanPath == "" || cleanPath == "." {
					return nil
				}

				parent := filepath.Dir(cleanPath)
				if parent == "." {
					parent = ""
				}
				bundleTree[parent] = append(bundleTree[parent], cleanPath)
				return nil
			})

			printTree(writer, bundleTree, "", "  ")
		}
	}

	return nil
}

// printTree is a helper function to print the file tree structure recursively.
func printTree(w io.Writer, tree map[string][]string, root, indent string) {
	// Sort entries for a consistent order
	entries := tree[root]
	sort.Strings(entries)

	for i, path := range entries {
		isLast := i == len(entries)-1
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		baseName := filepath.Base(path)
		io.WriteString(w, fmt.Sprintf("%s%s%s\n", indent, connector, baseName))

		// If this path is a directory (i.e., it's a key in the tree), recurse
		if _, ok := tree[path]; ok {
			newIndent := indent + "│   "
			if isLast {
				newIndent = indent + "    "
			}
			printTree(w, tree, path, newIndent)
		}
	}
}
