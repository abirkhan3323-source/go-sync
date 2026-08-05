# go-sync

Watch a directory and sync file changes to a target directory in real time.

## Install

```bash
go install github.com/abirkhan3323-source/go-sync@latest
```

## Usage

```bash
go-sync ./source ./target
```

Changes in `source/` are immediately mirrored to `target/`. The watcher
debounces rapid writes so bulk saves produce a single sync per file.

## Features

- Recursive directory watching
- Debounced writes (200ms default)
- Handles create, write, and remove events
- Cross-platform (Windows, macOS, Linux)

## Error Codes
| Code | Description |
|------|-------------|
| 1 | Invalid args |
| 2 | Source not found |
