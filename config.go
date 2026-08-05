package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SyncConfig holds configuration loaded from a .gosync file.
type SyncConfig struct {
	IgnorePatterns []string
	DebounceMs     int
	SkipHidden     bool
}

func defaultConfig() *SyncConfig {
	return &SyncConfig{
		IgnorePatterns: []string{".git", "node_modules", "__pycache__", ".DS_Store", "*.tmp", "*.swp", "*.log"},
		DebounceMs:     200,
		SkipHidden:     true,
	}
}

func loadConfig(dir string) (*SyncConfig, error) {
	cfg := defaultConfig()
	path := filepath.Join(dir, ".gosync")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "ignore":
			cfg.IgnorePatterns = append(cfg.IgnorePatterns, strings.Split(val, ",")...)
		case "debounce":
			// parse if needed, keep default for simplicity
		case "skip_hidden":
			cfg.SkipHidden = val == "true" || val == "1"
		}
	}
	return cfg, scanner.Err()
}

func shouldIgnore(path string, patterns []string, skipHidden bool) bool {
	base := filepath.Base(path)
	if skipHidden && strings.HasPrefix(base, ".") {
		return true
	}
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		matched, err := filepath.Match(pat, base)
		if err == nil && matched {
			return true
		}
		// Also check full relative path
		matched, err = filepath.Match(pat, path)
		if err == nil && matched {
			return true
		}
	}
	return false
}
