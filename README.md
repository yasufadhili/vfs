# VFS - Virtual File System Package

A flexible, reusable virtual file system implementation in Go that seamlessly integrates in-memory, disk-based, and embedded filesystems.
Built for my personal learning projects including compilers, networking tools, and cybersecurity utilities.

## Features

- **Multiple Filesystem Types**:
  - **Memory**: Fast, in-memory file operations.
  - **Disk**: A VFS that interacts directly with the local filesystem.
  - **Hybrid**: A combination of in-memory and embedded filesystems.
- **Unified Interface**: A single API for all VFS types.
- **File Watching**: Monitor disk-based VFS for changes.
- **Bundled Filesystems**: Register multiple embedded filesystems with custom prefixes.
- **Advanced Operations**: Clone and merge filesystems.
- **Flexible Configuration**: Options pattern for clean setup.
- **Comprehensive File Ops**: Full CRUD operations, directory traversal, and pattern matching.
- **Optional Logging**: Debug and error logging support.
- **Testing-friendly**: Interface-based design for easy mocking.

## Installation

```bash
go get github.com/yasufadhili/vfs
```

## Quick Start

```go
package main

import (
	"embed"
	"fmt"
	"log"
	"time"

	"github.com/yasufadhili/vfs"
)

//go:embed stdlib/*
var stdlibFS embed.FS

func main() {
	// 1. Create a hybrid VFS for in-memory and embedded files
	hybridVFS := vfs.NewHybridVFS()
	hybridVFS.RegisterBundled("stdlib", stdlibFS, "stdlib")

	// 2. Create a disk-based VFS to watch for source file changes
	diskVFS := vfs.NewDiskVFS("./src", vfs.WithLogger(&vfs.NullLogger{}))
	defer diskVFS.Close()

	// 3. Watch for changes
	diskVFS.Watch("/", func(event vfs.WatchEvent) {
		log.Printf("File %s changed: %s", event.Path, event.Op)
	})

	// 4. Write a file to the hybrid VFS
	hybridVFS.WriteFile("/main.go", []byte("package main"), 0644)

	// 5. Read from both memory and embedded files
	mainFile, _ := hybridVFS.ReadFileString("/main.go")
	stdlibFile, _ := hybridVFS.ReadFileString("stdlib://core.go")

	fmt.Printf("Main file: %s
", mainFile)
	fmt.Printf("Stdlib file: %s
", stdlibFile)

	// Keep the application running to receive watch events
	time.Sleep(10 * time.Second)
}
```

## Use Cases in My Projects

### Compiler Projects
```go
// A hybrid VFS for the standard library and a disk VFS for user code
compilerVFS := vfs.NewHybridVFS()
compilerVFS.RegisterBundled("stdlib", stdlibEmbed, "lib")
userCodeVFS := vfs.NewDiskVFS("./project")

// Compile with access to both user code and stdlib
sourceCode, _ := userCodeVFS.ReadFileString("/main.jml")
stdlibCode, _ := compilerVFS.ReadFileString("stdlib://fmt.jml")
```

### Network Tools
```go
// Load configuration from disk and embedded payloads
toolVFS := vfs.NewHybridVFS()
toolVFS.RegisterBundled("payloads", payloadsEmbed, "")
toolVFS.LoadFromDisk("./configs", "/configs")

// Access both
payload, _ := toolVFS.ReadFile("payloads://reverse_shell.bin")
config, _ := toolVFS.ReadFile("/configs/target.json")
```

### Build Systems
```go
// Use different VFS for different stages of the build
sourceVFS := vfs.NewDiskVFS("./project")
buildVFS := vfs.NewMemoryVFS()

// ... build process ...

// Save the build artifacts to disk
buildVFS.SaveToDisk("/", "./build")
```

## API Reference

### Core Operations

```go
// File I/O
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
```

### Directory Listing

```go
// List contents
ListFiles(dir string) ([]string, error)
ListDirs(dir string) ([]string, error)
Walk(root string, walkFn filepath.WalkFunc) error

// Pattern matching
FindFiles(root, pattern string) ([]string, error)
```

### Utility Operations

```go
// File operations
Copy(src, dst string) error
Move(src, dst string) error

// Disk integration
LoadFromDisk(srcPath, destPath string) error
SaveToDisk(srcPath, destPath string) error
```

### Advanced Operations

```go
// Clone a VFS
Clone() FileSystem

// Merge one VFS into another
Merge(other FileSystem, destPath string) error
```

### File Watching

```go
// Watch for file changes
Watch(path string, action WatchAction) error
StopWatch(path string) error
StopAllWatches() error
IsWatching(path string) bool
```

### Configuration

```go
// Factory functions
NewMemoryVFS(opts ...Option) *VFS
NewDiskVFS(rootPath string, opts ...Option) *VFS
NewHybridVFS(opts ...Option) *VFS

// Configuration options
WithLogger(logger Logger) Option
WithRoot(root string) Option

// Register embedded filesystems
RegisterBundled(prefix string, embedFS embed.FS, subdir string) error
```

## Path Conventions

- **Regular files**: `/path/to/file.ext` or `path/to/file.ext`
- **Bundled files**: `prefix://path/to/file.ext`
- **Examples**:
  - `stdlib://fmt.go` - Standard library file
  - `templates://config.tmpl` - Template file
  - `/src/main.go` - Regular VFS file

## Dependencies

- `github.com/spf13/afero` - Virtual filesystem abstraction
- `github.com/fsnotify/fsnotify` - File system notifications
- Go 1.16+ (for `embed.FS` support)

## Project Structure

```
vfs/
├── vfs.go          # Core interfaces and factory functions
├── main.go         # Main VFS implementation
├── bundled.go      # Embedded filesystem handling
├── watch.go        # File watching implementation
├── README.md       # This file
├── examples/       # Usage examples
└── vfs_test.go     # Test files
```

## Future Enhancements

- [ ] Compression support for bundled files
- [ ] Caching layer for frequently accessed files
- [ ] Plugin system for custom filesystem backends
- [ ] Metrics and performance monitoring

## Notes

This is a personal project designed for my specific use cases in mainly learning compiler development, networking tools, and cybersecurity utilities. The API prioritises simplicity and flexibility over backwards compatibility—breaking changes may occur as I evolve the design based on real-world usage in my projects.

The package draws inspiration from Go's `embed.FS`, `afero`, and various VFS implementations I've encountered in systems programming and compiler design.
