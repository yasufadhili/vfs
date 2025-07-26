package vfs

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

//go:embed testdata/*
var testdataFS embed.FS

// TestMemoryVFS tests basic memory VFS operations
func TestMemoryVFS(t *testing.T) {
	vfs := NewMemoryVFS()

	// Test file operations
	testContent := []byte("Hello, VFS!")
	err := vfs.WriteFile("/test.txt", testContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if !vfs.Exists("/test.txt") {
		t.Error("File should exist")
	}

	content, err := vfs.ReadFile("/test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(content) != string(testContent) {
		t.Errorf("Content mismatch: got %s, want %s", content, testContent)
	}

	// Test directory operations
	err = vfs.MkdirAll("/path/to/dir", 0755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if !vfs.IsDir("/path/to/dir") {
		t.Error("Directory should exist")
	}
}

// TestDiskVFS tests disk-based VFS operations
func TestDiskVFS(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("Hello, Disk VFS!")
	err := os.WriteFile(testFile, testContent, 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create disk VFS
	vfs := NewDiskVFS(tempDir)
	defer vfs.Close()

	// Test reading existing file
	content, err := vfs.ReadFile("/test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(content) != string(testContent) {
		t.Errorf("Content mismatch: got %s, want %s", content, testContent)
	}

	// Test writing new file
	newContent := []byte("New file content")
	err = vfs.WriteFile("/new.txt", newContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify file was created on disk
	diskPath := filepath.Join(tempDir, "new.txt")
	diskContent, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("Failed to read file from disk: %v", err)
	}

	if string(diskContent) != string(newContent) {
		t.Errorf("Disk content mismatch: got %s, want %s", diskContent, newContent)
	}
}

// TestHybridVFS tests hybrid VFS with bundled resources
func TestHybridVFS(t *testing.T) {
	vfs := NewHybridVFS()

	// Register bundled filesystem
	err := vfs.RegisterBundled("test", testdataFS, "testdata")
	if err != nil {
		t.Fatalf("RegisterBundled failed: %v", err)
	}

	// Test writing to memory
	memContent := []byte("Memory content")
	err = vfs.WriteFile("/memory.txt", memContent, 0644)
	if err != nil {
		t.Fatalf("WriteFile to memory failed: %v", err)
	}

	// Test reading from memory
	content, err := vfs.ReadFile("/memory.txt")
	if err != nil {
		t.Fatalf("ReadFile from memory failed: %v", err)
	}

	if string(content) != string(memContent) {
		t.Errorf("Memory content mismatch: got %s, want %s", content, memContent)
	}

	// Test reading from bundled
	bundledContent, err := vfs.ReadFile("test://test.txt")
	if err != nil {
		t.Fatalf("ReadFile from bundled failed: %v", err)
	}

	if len(bundledContent) == 0 {
		t.Error("Bundled content should not be empty")
	}
}

// TestVFSClone tests VFS cloning functionality
func TestVFSClone(t *testing.T) {
	original := NewMemoryVFS()

	// Add some files to original
	original.WriteFile("/file1.txt", []byte("Content 1"), 0644)
	original.WriteFile("/file2.txt", []byte("Content 2"), 0644)
	original.MkdirAll("/subdir", 0755)
	original.WriteFile("/subdir/file3.txt", []byte("Content 3"), 0644)

	// Clone the VFS
	clone := original.Clone()

	// Verify clone has all files
	files := []string{"/file1.txt", "/file2.txt", "/subdir/file3.txt"}
	for _, file := range files {
		if !clone.Exists(file) {
			t.Errorf("Clone missing file: %s", file)
		}

		originalContent, _ := original.ReadFile(file)
		cloneContent, _ := clone.ReadFile(file)

		if string(originalContent) != string(cloneContent) {
			t.Errorf("Clone content mismatch for %s", file)
		}
	}

	// Modify clone and verify original is unchanged
	clone.WriteFile("/file1.txt", []byte("Modified Content"), 0644)

	originalContent, _ := original.ReadFile("/file1.txt")
	if string(originalContent) != "Content 1" {
		t.Error("Original VFS was modified when clone was changed")
	}
}

// TestVFSMerge tests VFS merging functionality
func TestVFSMerge(t *testing.T) {
	vfs1 := NewMemoryVFS()
	vfs2 := NewMemoryVFS()

	// Add files to both VFS
	vfs1.WriteFile("/file1.txt", []byte("VFS1 Content"), 0644)
	vfs1.WriteFile("/shared.txt", []byte("VFS1 Shared"), 0644)

	vfs2.WriteFile("/file2.txt", []byte("VFS2 Content"), 0644)
	vfs2.WriteFile("/shared.txt", []byte("VFS2 Shared"), 0644)

	// Merge vfs2 into vfs1 at /merged
	err := vfs1.Merge(vfs2, "/merged")
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}

	// Verify merged files exist
	if !vfs1.Exists("/merged/file2.txt") {
		t.Error("Merged file2.txt should exist")
	}

	if !vfs1.Exists("/merged/shared.txt") {
		t.Error("Merged shared.txt should exist")
	}

	// Verify original files still exist
	if !vfs1.Exists("/file1.txt") {
		t.Error("Original file1.txt should still exist")
	}

	// Verify content
	content, _ := vfs1.ReadFileString("/merged/file2.txt")
	if content != "VFS2 Content" {
		t.Errorf("Merged content mismatch: got %s, want %s", content, "VFS2 Content")
	}
}

// TestFileWatch tests file watching functionality
func TestFileWatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping watch test in short mode")
	}

	// Create temporary directory
	tempDir := t.TempDir()

	// Create disk VFS
	vfs := NewDiskVFS(tempDir)
	defer vfs.Close()

	// Set up watch
	var events []WatchEvent
	var mu sync.Mutex

	watchAction := func(event WatchEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}

	err := vfs.Watch("/", watchAction)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Create a file (should trigger CREATE event)
	err = vfs.WriteFile("/watch_test.txt", []byte("Hello"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Modify the file (should trigger WRITE event)
	err = vfs.WriteFile("/watch_test.txt", []byte("Hello, World!"), 0644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Wait for events to be processed
	time.Sleep(500 * time.Millisecond)

	// Check events
	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Error("No watch events received")
	}

	// Verify we got relevant events
	hasCreate := false
	hasWrite := false

	for _, event := range events {
		t.Logf("Watch event: %s %s", event.Op, event.Path)
		if strings.Contains(event.Path, "watch_test.txt") {
			switch event.Op {
			case WatchOpCreate:
				hasCreate = true
			case WatchOpWrite:
				hasWrite = true
			}
		}
	}

	if !hasCreate && !hasWrite {
		t.Error("Expected CREATE or WRITE events for watch_test.txt")
	}
}

// TestLoadFromDisk tests loading files from disk
func TestLoadFromDisk(t *testing.T) {
	// Create temporary source directory
	srcDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"file1.txt":         "Content 1",
		"subdir/file2.txt":  "Content 2",
		"subdir/file3.json": `{"key": "value"}`,
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(srcDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create test file %s: %v", path, err)
		}
	}

	// Create memory VFS and load from disk
	vfs := NewMemoryVFS()
	err := vfs.LoadFromDisk(srcDir, "/loaded")
	if err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Verify loaded files
	for path, expectedContent := range testFiles {
		vfsPath := "/loaded/" + path
		if !vfs.Exists(vfsPath) {
			t.Errorf("Loaded file should exist: %s", vfsPath)
			continue
		}

		content, err := vfs.ReadFileString(vfsPath)
		if err != nil {
			t.Errorf("Failed to read loaded file %s: %v", vfsPath, err)
			continue
		}

		if content != expectedContent {
			t.Errorf("Content mismatch for %s: got %s, want %s", vfsPath, content, expectedContent)
		}
	}
}

// TestSaveToDisk tests saving VFS contents to disk
func TestSaveToDisk(t *testing.T) {
	// Create memory VFS with test files
	vfs := NewMemoryVFS()
	vfs.WriteFile("/file1.txt", []byte("Content 1"), 0644)
	vfs.WriteFile("/subdir/file2.txt", []byte("Content 2"), 0644)
	vfs.WriteFile("/subdir/file3.json", []byte(`{"key": "value"}`), 0644)

	// Create temporary destination directory
	destDir := t.TempDir()

	// Save to disk
	err := vfs.SaveToDisk("/", destDir)
	if err != nil {
		t.Fatalf("SaveToDisk failed: %v", err)
	}

	// Verify saved files
	expectedFiles := []string{
		"file1.txt",
		"subdir/file2.txt",
		"subdir/file3.json",
	}

	for _, file := range expectedFiles {
		diskPath := filepath.Join(destDir, file)
		if _, err := os.Stat(diskPath); os.IsNotExist(err) {
			t.Errorf("Saved file should exist on disk: %s", file)
			continue
		}

		// Verify content
		diskContent, err := os.ReadFile(diskPath)
		if err != nil {
			t.Errorf("Failed to read saved file %s: %v", file, err)
			continue
		}

		vfsContent, err := vfs.ReadFile("/" + file)
		if err != nil {
			t.Errorf("Failed to read VFS file %s: %v", file, err)
			continue
		}

		if string(diskContent) != string(vfsContent) {
			t.Errorf("Content mismatch for %s", file)
		}
	}
}

// TestFindFiles tests pattern-based file finding
func TestFindFiles(t *testing.T) {
	vfs := NewMemoryVFS()

	// Create test files
	testFiles := []string{
		"/file1.txt",
		"/file2.go",
		"/subdir/file3.txt",
		"/subdir/file4.go",
		"/subdir/nested/file5.json",
	}

	for _, file := range testFiles {
		vfs.WriteFile(file, []byte("content"), 0644)
	}

	// Test pattern matching
	tests := []struct {
		pattern  string
		expected int
	}{
		{"*.txt", 2},
		{"*.go", 2},
		{"*.json", 1},
		{"file*", 5},
		{"*", 5},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			matches, err := vfs.FindFiles("/", tt.pattern)
			if err != nil {
				t.Fatalf("FindFiles failed: %v", err)
			}

			if len(matches) != tt.expected {
				t.Errorf("Expected %d matches, got %d", tt.expected, len(matches))
			}
		})
	}
}

//go:embed testdata/*
var testEmbed embed.FS

func TestVFS_Dump(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *VFS
		expected []string
	}{
		{
			name: "empty memory VFS",
			setup: func() *VFS {
				return NewMemoryVFS()
			},
			expected: []string{
				"--- VFS Root ---",
			},
		},
		{
			name: "memory VFS with files",
			setup: func() *VFS {
				vfs := NewMemoryVFS()
				vfs.WriteFile("/test.txt", []byte("content"), 0644)
				vfs.WriteFile("/dir/nested.txt", []byte("nested"), 0644)
				vfs.WriteFile("/dir/another.txt", []byte("another"), 0644)
				return vfs
			},
			expected: []string{
				"--- VFS Root ---",
				"├── dir",
				"│   ├── another.txt",
				"│   └── nested.txt",
				"└── test.txt",
			},
		},
		{
			name: "VFS with bundled filesystem",
			setup: func() *VFS {
				vfs := NewMemoryVFS()
				// Add some regular files
				vfs.WriteFile("/regular.txt", []byte("regular content"), 0644)

				// Register a bundled filesystem
				vfs.RegisterBundled("assets", testEmbed, "testdata")
				return vfs
			},
			expected: []string{
				"--- VFS Root ---",
				"└── regular.txt",
				"--- Bundled Filesystems ---",
				"Bundle [assets://]:",
			},
		},
		{
			name: "VFS with multiple bundled filesystems",
			setup: func() *VFS {
				vfs := NewMemoryVFS()
				vfs.WriteFile("/main.txt", []byte("main"), 0644)

				// Register multiple bundled filesystems
				vfs.RegisterBundled("assets", testEmbed, "testdata")
				vfs.RegisterBundled("themes", testEmbed, "testdata")
				return vfs
			},
			expected: []string{
				"--- VFS Root ---",
				"└── main.txt",
				"--- Bundled Filesystems ---",
				"Bundle [assets://]:",
				"Bundle [themes://]:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vfs := tt.setup()

			var buf bytes.Buffer
			err := vfs.Dump(&buf)
			if err != nil {
				t.Fatalf("Dump() error = %v", err)
			}

			output := buf.String()
			t.Logf("Dump output:\n%s", output)

			// Check that all expected strings are present
			for _, expected := range tt.expected {
				if !strings.Contains(output, expected) {
					t.Errorf("Expected output to contain %q, but it didn't.\nFull output:\n%s", expected, output)
				}
			}

			// Basic structure checks
			if !strings.Contains(output, "--- VFS Root ---") {
				t.Error("Output should contain VFS Root section")
			}

			// If we have bundled filesystems, check for bundled section
			registeredPrefixes := vfs.bundledManager.ListRegistered()
			if len(registeredPrefixes) > 0 {
				if !strings.Contains(output, "--- Bundled Filesystems ---") {
					t.Error("Output should contain Bundled Filesystems section when bundled filesystems are registered")
				}
			}
		})
	}
}

func TestVFS_Dump_TreeFormatting(t *testing.T) {
	vfs := NewMemoryVFS()

	// Create a more complex directory structure
	files := []string{
		"/a.txt",
		"/b.txt",
		"/dir1/file1.txt",
		"/dir1/file2.txt",
		"/dir1/subdir/deep.txt",
		"/dir2/file3.txt",
		"/dir2/another/nested.txt",
	}

	for _, file := range files {
		err := vfs.WriteFile(file, []byte("content"), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", file, err)
		}
	}

	var buf bytes.Buffer
	err := vfs.Dump(&buf)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}

	output := buf.String()
	t.Logf("Tree output:\n%s", output)

	// Check for proper tree formatting characters
	expectedPatterns := []string{
		"├──", // Branch connector
		"└──", // Last item connector
		"│",   // Vertical line
	}

	for _, pattern := range expectedPatterns {
		if !strings.Contains(output, pattern) {
			t.Errorf("Expected tree formatting pattern %q not found in output", pattern)
		}
	}

	// Check that directories appear before their contents
	lines := strings.Split(output, "\n")
	var dir1Index, file1Index int
	for i, line := range lines {
		if strings.Contains(line, "dir1") && !strings.Contains(line, "/") {
			dir1Index = i
		}
		if strings.Contains(line, "file1.txt") {
			file1Index = i
		}
	}

	if dir1Index == 0 || file1Index == 0 {
		t.Error("Could not find expected directory and file entries")
	}

	if dir1Index >= file1Index {
		t.Error("Directory should appear before its contents in the tree")
	}
}

func TestVFS_Dump_ErrorHandling(t *testing.T) {
	vfs := NewMemoryVFS()

	// Test with a nil writer should not panic
	err := vfs.Dump(nil)
	if err == nil {
		t.Error("Expected error when dumping to nil writer")
	}

	// Test with a valid but empty VFS
	var buf bytes.Buffer
	err = vfs.Dump(&buf)
	if err != nil {
		t.Errorf("Dump() should not error on empty VFS: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "--- VFS Root ---") {
		t.Error("Empty VFS should still show root section")
	}
}

func BenchmarkVFS_Dump(b *testing.B) {
	vfs := NewMemoryVFS()

	// Create a filesystem with some depth and breadth
	for i := 0; i < 50; i++ {
		vfs.WriteFile(fmt.Sprintf("/file%d.txt", i), []byte("content"), 0644)
		vfs.WriteFile(fmt.Sprintf("/dir%d/nested%d.txt", i%5, i), []byte("nested"), 0644)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		err := vfs.Dump(&buf)
		if err != nil {
			b.Fatalf("Dump error: %v", err)
		}
	}
}
