package vfs

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// WatchManager handles file system watching operations
type WatchManager struct {
	watcher  *fsnotify.Watcher
	watches  map[string]WatchAction
	rootPath string
	logger   Logger
	mu       sync.RWMutex
	closed   bool
}

// NewWatchManager creates a new watch manager
func NewWatchManager(rootPath string, logger Logger) *WatchManager {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("Failed to create file watcher: %v", err)
		return nil
	}

	wm := &WatchManager{
		watcher:  watcher,
		watches:  make(map[string]WatchAction),
		rootPath: rootPath,
		logger:   logger,
	}

	// Start the event processing goroutine
	go wm.processEvents()

	return wm
}

// processEvents processes file system events in a separate goroutine
func (wm *WatchManager) processEvents() {
	for {
		select {
		case event, ok := <-wm.watcher.Events:
			if !ok {
				return // Channel closed
			}
			wm.handleEvent(event)

		case err, ok := <-wm.watcher.Errors:
			if !ok {
				return // Channel closed
			}
			wm.logger.Error("File watcher error: %v", err)

			// Notify all watches about the error
			wm.mu.RLock()
			for path, action := range wm.watches {
				action(WatchEvent{
					Path:  path,
					Error: err,
				})
			}
			wm.mu.RUnlock()
		}
	}
}

// handleEvent processes a single file system event
func (wm *WatchManager) handleEvent(event fsnotify.Event) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	// Convert absolute path back to relative path for VFS
	relPath, err := filepath.Rel(wm.rootPath, event.Name)
	if err != nil {
		wm.logger.Error("Failed to get relative path for %s: %v", event.Name, err)
		return
	}

	// Convert to VFS path format
	vfsPath := "/" + filepath.ToSlash(relPath)

	// Find matching watch patterns
	for watchPath, action := range wm.watches {
		if wm.pathMatches(vfsPath, watchPath) {
			watchEvent := WatchEvent{
				Path:  vfsPath,
				Op:    convertFsnotifyOp(event.Op),
				IsDir: wm.isDir(event.Name),
			}

			wm.logger.Debug("File event: %s %s", watchEvent.Op, watchEvent.Path)

			// Execute the watch action in a separate goroutine to avoid blocking
			go func(action WatchAction, event WatchEvent) {
				defer func() {
					if r := recover(); r != nil {
						wm.logger.Error("Watch action panicked: %v", r)
					}
				}()
				action(event)
			}(action, watchEvent)
		}
	}
}

// pathMatches checks if a file path matches a watch pattern
func (wm *WatchManager) pathMatches(filePath, watchPath string) bool {
	// Exact match
	if filePath == watchPath {
		return true
	}

	// Directory match (file is under the watched directory)
	if watchPath == "/" || filepath.HasPrefix(filePath, watchPath+"/") {
		return true
	}

	// Pattern matching could be added here
	matched, err := filepath.Match(watchPath, filePath)
	if err != nil {
		return false
	}

	return matched
}

// isDir checks if a path is a directory
func (wm *WatchManager) isDir(path string) bool {
	// This is a simple heuristic - in practice, you might want to stat the file
	// but fsnotify events might fire after deletion, so stat could fail
	return filepath.Ext(path) == ""
}

// convertFsnotifyOp converts fsnotify operations to our WatchOp type
func convertFsnotifyOp(op fsnotify.Op) WatchOp {
	switch {
	case op&fsnotify.Create == fsnotify.Create:
		return WatchOpCreate
	case op&fsnotify.Write == fsnotify.Write:
		return WatchOpWrite
	case op&fsnotify.Remove == fsnotify.Remove:
		return WatchOpRemove
	case op&fsnotify.Rename == fsnotify.Rename:
		return WatchOpRename
	case op&fsnotify.Chmod == fsnotify.Chmod:
		return WatchOpChmod
	default:
		return WatchOpWrite // Default fallback
	}
}

// Watch starts watching a path for changes
func (wm *WatchManager) Watch(path string, action WatchAction) error {
	if wm == nil || wm.closed {
		return fmt.Errorf("watch manager is not available")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Convert VFS path to absolute disk path
	var diskPath string
	if path == "/" {
		diskPath = wm.rootPath
	} else {
		diskPath = filepath.Join(wm.rootPath, strings.TrimPrefix(path, "/"))
	}

	// Add to fsnotify watcher
	if err := wm.watcher.Add(diskPath); err != nil {
		return fmt.Errorf("failed to watch path %s: %w", path, err)
	}

	// Store the action
	wm.watches[path] = action
	wm.logger.Debug("Started watching path: %s", path)

	return nil
}

// StopWatch stops watching a specific path
func (wm *WatchManager) StopWatch(path string) error {
	if wm == nil || wm.closed {
		return nil
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Convert VFS path to absolute disk path
	var diskPath string
	if path == "/" {
		diskPath = wm.rootPath
	} else {
		diskPath = filepath.Join(wm.rootPath, strings.TrimPrefix(path, "/"))
	}

	// Remove from fsnotify watcher
	if err := wm.watcher.Remove(diskPath); err != nil {
		wm.logger.Error("Failed to stop watching path %s: %v", path, err)
	}

	// Remove the action
	delete(wm.watches, path)
	wm.logger.Debug("Stopped watching path: %s", path)

	return nil
}

// StopAllWatches stops all active watches
func (wm *WatchManager) StopAllWatches() error {
	if wm == nil || wm.closed {
		return nil
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	for path := range wm.watches {
		diskPath := filepath.Join(wm.rootPath, strings.TrimPrefix(path, "/"))
		if err := wm.watcher.Remove(diskPath); err != nil {
			wm.logger.Error("Failed to stop watching path %s: %v", path, err)
		}
	}

	wm.watches = make(map[string]WatchAction)
	wm.logger.Debug("Stopped all watches")

	return nil
}

// IsWatching checks if a path is being watched
func (wm *WatchManager) IsWatching(path string) bool {
	if wm == nil || wm.closed {
		return false
	}

	wm.mu.RLock()
	defer wm.mu.RUnlock()

	_, exists := wm.watches[path]
	return exists
}

// Close closes the watch manager and stops all watches
func (wm *WatchManager) Close() error {
	if wm == nil || wm.closed {
		return nil
	}

	wm.mu.Lock()
	wm.closed = true
	wm.mu.Unlock()

	wm.StopAllWatches()
	return wm.watcher.Close()
}

// Watch operations for VFS - these delegate to the watch manager if available

// Watch starts watching a path for changes (only available for disk-based VFS)
func (v *VFS) Watch(path string, action WatchAction) error {
	if v.watchManager == nil {
		return fmt.Errorf("watching is only available for disk-based VFS")
	}

	return v.watchManager.Watch(path, action)
}

// StopWatch stops watching a specific path
func (v *VFS) StopWatch(path string) error {
	if v.watchManager == nil {
		return fmt.Errorf("watching is only available for disk-based VFS")
	}

	return v.watchManager.StopWatch(path)
}

// StopAllWatches stops all active watches
func (v *VFS) StopAllWatches() error {
	if v.watchManager == nil {
		return fmt.Errorf("watching is only available for disk-based VFS")
	}

	return v.watchManager.StopAllWatches()
}

// IsWatching checks if a path is being watched
func (v *VFS) IsWatching(path string) bool {
	if v.watchManager == nil {
		return false
	}

	return v.watchManager.IsWatching(path)
}

// Close closes the VFS and stops all watches
func (v *VFS) Close() error {
	if v.watchManager != nil {
		return v.watchManager.Close()
	}
	return nil
}
