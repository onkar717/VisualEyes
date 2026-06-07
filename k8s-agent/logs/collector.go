// Package logs implements an offset-tracked log tailer for Kubernetes container logs.
// Unlike Claritty (which re-reads full files every tick), this collector tracks the
// last byte offset per file and only reads new lines, dramatically reducing I/O.
package logs

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// MaxLinesPerFile caps lines collected per file per tick to prevent flooding.
	MaxLinesPerFile = 300
	// MaxFileSize skips files larger than 500 MB that have no tracked offset.
	MaxFileSize = 500 * 1024 * 1024
)

// LogLine is a single parsed log line from a container.
type LogLine struct {
	Pod       string
	Namespace string
	Container string
	Node      string
	Stream    string // "stdout" | "stderr"
	Line      string
	Timestamp time.Time
}

// Collector tails container log files under logDir, tracking byte offsets so
// repeated calls only return new lines since the last collection.
type Collector struct {
	logDir  string
	offsets map[string]int64 // file path → last read byte offset
	sizes   map[string]int64 // file path → size at last read (rotation detection)
	mu      sync.Mutex
	node    string
}

// NewCollector creates a Collector targeting logDir (typically /var/log/containers).
func NewCollector(logDir, nodeName string) *Collector {
	return &Collector{
		logDir:  logDir,
		offsets: make(map[string]int64),
		sizes:   make(map[string]int64),
		node:    nodeName,
	}
}

// Collect scans for new log files and reads newly written lines from tracked files.
// Returns at most MaxLinesPerFile lines per file. Safe to call from multiple goroutines.
func (c *Collector) Collect() ([]LogLine, error) {
	pattern := filepath.Join(c.logDir, "*.log")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var result []LogLine

	for _, path := range paths {
		lines, err := c.tailFile(path)
		if err != nil {
			slog.Warn("log tail error", "path", path, "error", err)
			continue
		}
		result = append(result, lines...)
	}

	return result, nil
}

// tailFile reads only new bytes from path since the last call.
func (c *Collector) tailFile(path string) ([]LogLine, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	currentSize := info.Size()

	c.mu.Lock()
	offset := c.offsets[path]
	prevSize := c.sizes[path]
	c.mu.Unlock()

	// Detect log rotation: file shrank → reset offset.
	if currentSize < prevSize {
		slog.Debug("log rotation detected", "path", path)
		offset = 0
	}

	// Nothing new to read.
	if currentSize == offset {
		return nil, nil
	}

	// Skip large files we've never started reading (avoid OOM on first scan).
	if offset == 0 && currentSize > MaxFileSize {
		slog.Warn("skipping large log file", "path", path, "size_mb", currentSize/1024/1024)
		c.mu.Lock()
		c.offsets[path] = currentSize
		c.sizes[path] = currentSize
		c.mu.Unlock()
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if _, err := f.Seek(offset, 0); err != nil {
		return nil, err
	}

	pod, namespace, container, ok := parseLogFilename(filepath.Base(path))
	if !ok {
		// Not a standard k8s log filename — skip silently.
		return nil, nil
	}

	var lines []LogLine
	count := 0
	scanner := bufio.NewScanner(f)

	for scanner.Scan() && count < MaxLinesPerFile {
		raw := scanner.Text()
		if raw == "" {
			continue
		}
		ts, stream, message := parseCRILine(raw)
		lines = append(lines, LogLine{
			Pod:       pod,
			Namespace: namespace,
			Container: container,
			Node:      c.node,
			Stream:    stream,
			Line:      message,
			Timestamp: ts,
		})
		count++
	}

	if err := scanner.Err(); err != nil {
		slog.Warn("scanner error reading log", "path", path, "error", err)
	}

	// Update offset to current position.
	newOffset, _ := f.Seek(0, 1) // SEEK_CUR
	c.mu.Lock()
	c.offsets[path] = newOffset
	c.sizes[path] = currentSize
	c.mu.Unlock()

	return lines, nil
}

// parseLogFilename extracts pod, namespace, and container from the standard
// Kubernetes container log filename format:
//
//	<pod-name>_<namespace>_<container-name>-<containerID>.log
func parseLogFilename(filename string) (pod, namespace, container string, ok bool) {
	// Strip .log suffix
	name := strings.TrimSuffix(filename, ".log")
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return "", "", "", false
	}
	pod = parts[0]
	namespace = parts[1]
	// Container name may have a hash appended after the last "-"
	containerPart := strings.Join(parts[2:], "_")
	if idx := strings.LastIndex(containerPart, "-"); idx >= 0 {
		container = containerPart[:idx]
	} else {
		container = containerPart
	}
	return pod, namespace, container, true
}

// parseCRILine parses a CRI-O / containerd log line:
//
//	<RFC3339Nano> <stream> <flags> <message>
//
// Falls back gracefully if the format does not match.
func parseCRILine(raw string) (ts time.Time, stream, message string) {
	parts := strings.SplitN(raw, " ", 4)
	if len(parts) < 4 {
		return time.Now(), "stdout", raw
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		t = time.Now()
	}
	return t, parts[1], parts[3]
}
