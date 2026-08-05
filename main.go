// go-sync: watch a directory and sync changes to a target directory.
// Usage: go-sync <source> <target> [flags]
//
// Flags:
//   --stats        Print sync statistics on exit
//   --dry-run      Show what would be synced without copying

package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	stats := flag.Bool("stats", false, "Print sync statistics on exit")
	dryRun := flag.Bool("dry-run", false, "Show what would be synced without copying")
	flag.Parse()

	if flag.NArg() < 2 {
		fmt.Fprintf(os.Stderr, "Usage: go-sync [flags] <source-dir> <target-dir>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	src := flag.Arg(0)
	dst := flag.Arg(1)

	cfg, err := loadConfig(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	session := NewWatchSession(src, dst, cfg)

	if *dryRun {
		fmt.Printf("[dry-run] Would sync %s -> %s\n", src, dst)
		fmt.Printf("[dry-run] Ignoring: %v\n", cfg.IgnorePatterns)
		return
	}

	session.OnSync = func(rel string, size int64) {
		fmt.Printf("  -> %s\n", rel)
	}
	session.OnError = func(err error) {
		fmt.Fprintf(os.Stderr, "  ! %v\n", err)
	}

	fmt.Printf("go-sync: %s -> %s\n", src, dst)
	fmt.Printf("  ignore: %v\n", cfg.IgnorePatterns)

	if err := session.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		if *stats {
			fmt.Printf("\n%s\n", session.Stats.Summary())
		}
		session.Stop()
		os.Exit(0)
	}()

	fmt.Println("Watching for changes... (Ctrl+C to stop)")
	select {}
}
