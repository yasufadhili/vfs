package main

import (
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/yasufadhili/vfs"
)

//go:embed stdlib/*
var stdlibFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// CompilerExample demonstrates a complete compiler workflow
func CompilerExample() {
	fmt.Println("=== Compiler Example ===")

	// 1. Create input VFS from disk with watching
	inputVFS := vfs.NewDiskVFS("./src", vfs.WithLogger(&SimpleLogger{}))
	defer inputVFS.Close()

	// 2. Create output VFS in memory for generated code
	outputVFS := vfs.NewMemoryVFS(vfs.WithLogger(&SimpleLogger{}))

	// 3. Create hybrid VFS for stdlib and templates
	compilerVFS := vfs.NewHybridVFS(vfs.WithLogger(&SimpleLogger{}))
	compilerVFS.RegisterBundled("stdlib", stdlibFS, "stdlib")
	compilerVFS.RegisterBundled("templates", templateFS, "templates")

	// 4. Set up watching for source changes
	inputVFS.Watch("/", func(event vfs.WatchEvent) {
		if event.Error != nil {
			log.Printf("Watch error: %v", event.Error)
			return
		}

		log.Printf("File %s %s", event.Op, event.Path)

		switch event.Op {
		case vfs.WatchOpWrite, vfs.WatchOpCreate:
			if filepath.Ext(event.Path) == ".jml" {
				// Recompile the changed file
				compileFile(inputVFS, outputVFS, compilerVFS, event.Path)
			}
		case vfs.WatchOpRemove:
			// Remove corresponding output files
			cleanupOutputs(outputVFS, event.Path)
		}
	})

	// 5. Initial compilation of all source files
	sourceFiles, _ := inputVFS.FindFiles("/", "*.jml")
	for _, file := range sourceFiles {
		compileFile(inputVFS, outputVFS, compilerVFS, file)
	}

	// 6. Eventually save outputs to disk
	outputVFS.SaveToDisk("/", "./build")

	fmt.Println("Compiler setup complete. Watching for changes...")
	time.Sleep(2 * time.Second) // Simulate watching
}

// NetworkToolExample demonstrates network tool usage
func NetworkToolExample() {
	fmt.Println("\n=== Network Tool Example ===")

	// Create tool with embedded payloads and config from disk
	toolVFS := vfs.NewHybridVFS(vfs.WithLogger(&SimpleLogger{}))

	// Register embedded payloads
	// toolVFS.RegisterBundled("payloads", payloadsFS, "")

	// Load configurations from disk
	toolVFS.LoadFromDisk("./configs", "/configs")

	// Use resources
	// payload, _ := toolVFS.ReadFile("payloads://reverse_shell.bin")
	config, _ := toolVFS.ReadFileString("/configs/target.json")

	fmt.Printf("Loaded config: %s\n", config[:min(50, len(config))])
}

// BuildSystemExample demonstrates build system usage
func BuildSystemExample() {
	fmt.Println("\n=== Build System Example ===")

	// Input: source project on disk with watching
	sourceVFS := vfs.NewDiskVFS("./project", vfs.WithLogger(&SimpleLogger{}))
	defer sourceVFS.Close()

	// Intermediate: memory VFS for temporary files
	tempVFS := vfs.NewMemoryVFS()

	// Templates: embedded templates
	templateVFS := vfs.NewHybridVFS()
	templateVFS.RegisterBundled("templates", templateFS, "templates")

	// Set up build pipeline
	sourceVFS.Watch("/", func(event vfs.WatchEvent) {
		if event.Op == vfs.WatchOpWrite && filepath.Ext(event.Path) == ".go" {
			// Process file through pipeline
			processFile(sourceVFS, tempVFS, templateVFS, event.Path)
		}
	})

	fmt.Println("Build system ready")
}

// PipelineExample demonstrates VFS pipeline usage
func PipelineExample() {
	fmt.Println("\n=== Pipeline Example ===")

	// Create pipeline stages
	stage1 := vfs.NewMemoryVFS(vfs.WithLogger(&SimpleLogger{}))
	stage2 := vfs.NewMemoryVFS(vfs.WithLogger(&SimpleLogger{}))
	stage3 := vfs.NewMemoryVFS(vfs.WithLogger(&SimpleLogger{}))

	// Populate stage 1
	stage1.WriteFile("/input.txt", []byte("Hello, VFS Pipeline!"), 0644)
	stage1.WriteFile("/data.json", []byte(`{"version": "1.0"}`), 0644)

	// Process stage 1 -> stage 2
	files, _ := stage1.ListFiles("/")
	for _, file := range files {
		data, _ := stage1.ReadFile("/" + file)
		processed := append([]byte("PROCESSED: "), data...)
		stage2.WriteFile("/processed_"+file, processed, 0644)
	}

	// Clone stage 2 to stage 3 for parallel processing
	stage3Clone := stage2.Clone()

	// Merge results
	stage3.Merge(stage3Clone, "/merged")

	// Output final results
	finalFiles, _ := stage3.ListFiles("/")
	fmt.Printf("Final files: %v\n", finalFiles)
}

// Helper functions for examples

func compileFile(inputVFS, outputVFS, compilerVFS vfs.FileSystem, filePath string) {
	// Read source file
	source, err := inputVFS.ReadFileString(filePath)
	if err != nil {
		log.Printf("Failed to read %s: %v", filePath, err)
		return
	}

	// Read stdlib if needed
	stdlibCode, _ := compilerVFS.ReadFileString("stdlib://core.jml")

	// Read code generation template
	template, _ := compilerVFS.ReadFileString("templates://output.tmpl")

	// Simulate compilation process
	compiled := fmt.Sprintf("// Generated from %s\n// Stdlib: %d bytes\n// Template: %d bytes\n%s",
		filePath, len(stdlibCode), len(template), source)

	// Write to output VFS
	outputPath := filepath.Join("/", filepath.Base(filePath)+".compiled")
	outputVFS.WriteFile(outputPath, []byte(compiled), 0644)

	log.Printf("Compiled %s -> %s", filePath, outputPath)
}

func cleanupOutputs(outputVFS vfs.FileSystem, sourcePath string) {
	outputPath := filepath.Join("/", filepath.Base(sourcePath)+".compiled")
	if outputVFS.Exists(outputPath) {
		outputVFS.Remove(outputPath)
		log.Printf("Cleaned up %s", outputPath)
	}
}

func processFile(sourceVFS, tempVFS, templateVFS vfs.FileSystem, filePath string) {
	// Read source
	source, _ := sourceVFS.ReadFileString(filePath)

	// Process with template
	template, _ := templateVFS.ReadFileString("templates://processor.tmpl")

	// Write to temp VFS
	processed := fmt.Sprintf("// Template: %d bytes\n%s", len(template), source)
	tempPath := "/temp_" + filepath.Base(filePath)
	tempVFS.WriteFile(tempPath, []byte(processed), 0644)

	log.Printf("Processed %s -> %s", filePath, tempPath)
}

// SimpleLogger implements the Logger interface
type SimpleLogger struct{}

func (l *SimpleLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] "+msg, args...)
}

func (l *SimpleLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *SimpleLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}

func main() {
	CompilerExample()
	NetworkToolExample()
	BuildSystemExample()
	PipelineExample()
}
