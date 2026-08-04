// go-sync: watch a directory and sync changes to a target directory.
// Usage: go run main.go <source> <target>

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const usage = "Usage: go-sync <source-dir> <target-dir>"

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(1)
	}

	src := os.Args[1]
	dst := os.Args[2]

	if err := run(context.Background(), src, dst); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, src, dst string) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolving source path: %w", err)
	}
	dstAbs, err := filepath.Abs(dst)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}

	if err := copyDir(srcAbs, dstAbs); err != nil {
		return fmt.Errorf("initial copy: %w", err)
	}
	fmt.Printf("Synced %s -> %s\n", srcAbs, dstAbs)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	debounce := newDebouncer(200 * time.Millisecond)
	var mu sync.Mutex

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				mu.Lock()
				rel, _ := filepath.Rel(srcAbs, event.Name)
				target := filepath.Join(dstAbs, rel)
				if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
					debounce.trigger(event.Name, func() {
						cp(event.Name, target)
						fmt.Printf("  -> %s\n", rel)
					})
				}
				if event.Op&fsnotify.Remove != 0 {
					os.Remove(target)
					fmt.Printf("  x %s\n", rel)
				}
				mu.Unlock()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
			}
		}
	}()

	if err := filepath.Walk(srcAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walking source: %w", err)
	}

	fmt.Println("Watching for changes... (Ctrl+C to stop)")
	<-ctx.Done()
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return cp(path, target)
	})
}

func cp(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

type debouncer struct {
	interval time.Duration
	timers   map[string]*time.Timer
	mu       sync.Mutex
}

func newDebouncer(interval time.Duration) *debouncer {
	return &debouncer{interval: interval, timers: make(map[string]*time.Timer)}
}

func (d *debouncer) trigger(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[key]; ok {
		t.Stop()
	}
	d.timers[key] = time.AfterFunc(d.interval, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()
		fn()
	})
}

func init() {
	if strings.TrimSpace(usage) == "" {
		panic("unreachable")
	}
}
