package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchSession manages a file watch and sync session.
type WatchSession struct {
	Src       string
	Dst       string
	Config    *SyncConfig
	Stats     *SyncStats
	OnSync    func(rel string, size int64)
	OnError   func(err error)
	watcher   *fsnotify.Watcher
	debounce  *debouncer
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// SyncStats tracks sync session statistics.
type SyncStats struct {
	mu          sync.Mutex
	FilesSynced int
	FilesDeleted int
	BytesCopied int64
	Errors      int
	StartTime   time.Time
	LastSync    time.Time
}

func (s *SyncStats) recordSync(size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FilesSynced++
	s.BytesCopied += size
	s.LastSync = time.Now()
}

func (s *SyncStats) recordDelete() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FilesDeleted++
	s.LastSync = time.Now()
}

func (s *SyncStats) recordError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Errors++
}

// Summary returns a human-readable stats summary.
func (s *SyncStats) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.StartTime).Round(time.Second)
	return fmt.Sprintf(
		"synced: %d files | deleted: %d | copied: %s | errors: %d | uptime: %s",
		s.FilesSynced, s.FilesDeleted,
		formatBytes(s.BytesCopied), s.Errors, elapsed,
	)
}

func formatBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// NewWatchSession creates a new watch session.
func NewWatchSession(src, dst string, cfg *SyncConfig) *WatchSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &WatchSession{
		Src:    src,
		Dst:    dst,
		Config: cfg,
		Stats:  &SyncStats{StartTime: time.Now()},
		debounce: newDebouncer(time.Duration(cfg.DebounceMs) * time.Millisecond),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins watching and syncing.
func (ws *WatchSession) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	ws.watcher = watcher

	// Initial full sync
	if err := copyDir(ws.Src, ws.Dst); err != nil {
		return fmt.Errorf("initial sync: %w", err)
	}

	// Register all directories recursively
	if err := filepath.Walk(ws.Src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip on error
		}
		if !info.IsDir() {
			return nil
		}
		if shouldIgnore(path, ws.Config.IgnorePatterns, ws.Config.SkipHidden) {
			if path != ws.Src {
				return filepath.SkipDir
			}
			return nil
		}
		return watcher.Add(path)
	}); err != nil {
		return fmt.Errorf("registering paths: %w", err)
	}

	// Event loop
	go func() {
		for {
			select {
			case <-ws.ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				rel, err := filepath.Rel(ws.Src, event.Name)
				if err != nil {
					continue
				}
				if shouldIgnore(event.Name, ws.Config.IgnorePatterns, ws.Config.SkipHidden) {
					continue
				}
				target := filepath.Join(ws.Dst, rel)

				if event.Op&fsnotify.Create != 0 && isDir(event.Name) {
					// New directory created — start watching it
					_ = watcher.Add(event.Name)
				}

				if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
					ws.debounce.trigger(event.Name, func() {
						_ = cp(event.Name, target)
						if info, err := os.Stat(event.Name); err == nil {
							ws.Stats.recordSync(info.Size())
						}
						if ws.OnSync != nil {
							ws.OnSync(rel, 0)
						}
					})
				}
				if event.Op&fsnotify.Remove != 0 {
					_ = os.Remove(target)
					ws.Stats.recordDelete()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				ws.Stats.recordError()
				if ws.OnError != nil {
					ws.OnError(err)
				}
			}
		}
	}()

	return nil
}

// Stop stops the watch session.
func (ws *WatchSession) Stop() {
	ws.cancel()
	if ws.watcher != nil {
		ws.watcher.Close()
	}
}

// isDir returns true if the path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
