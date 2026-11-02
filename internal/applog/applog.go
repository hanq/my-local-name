package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxMemoryEntries = 200
	maxFileSize      = 1 << 20 // 1 MiB
)

// Entry is a single log line exposed to the UI.
type Entry struct {
	ID      int64  `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Logger keeps the most recent entries in memory and appends to a rotating file.
type Logger struct {
	mu       sync.Mutex
	entries  []Entry
	nextID   int64
	file     *os.File
	fileSize int64
	path     string
	onLine   func(Entry)
}

// New creates a logger in dir. A nil onLine callback is allowed.
func New(dir string, onLine func(Entry)) *Logger {
	l := &Logger{path: filepath.Join(dir, "my-local-name.log"), onLine: onLine}
	if err := os.MkdirAll(dir, 0o755); err == nil {
		l.openLocked()
	}
	return l
}

// Path returns the log file path.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Log appends one entry. It is safe for concurrent use.
func (l *Logger) Log(level, message string) {
	if l == nil {
		return
	}
	now := time.Now()
	entry := Entry{
		Time:    now.Format("15:04:05"),
		Level:   level,
		Message: message,
	}

	l.mu.Lock()
	l.nextID++
	entry.ID = l.nextID
	l.entries = append(l.entries, entry)
	if len(l.entries) > maxMemoryEntries {
		l.entries = append([]Entry(nil), l.entries[len(l.entries)-maxMemoryEntries:]...)
	}
	l.writeLocked(now, level, message)
	callback := l.onLine
	l.mu.Unlock()

	if callback != nil {
		callback(entry)
	}
}

// Snapshot returns a copy of the in-memory entries.
func (l *Logger) Snapshot() []Entry {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Entry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Close flushes and closes the log file.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

func (l *Logger) openLocked() {
	path := l.path
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return
	}
	l.file = f
	l.fileSize = info.Size()
}

func (l *Logger) writeLocked(t time.Time, level, message string) {
	line := fmt.Sprintf("%s %-5s %s\n", t.Format(time.RFC3339), level, message)
	if l.file != nil && l.fileSize+int64(len(line)) > maxFileSize {
		_ = l.file.Close()
		l.file = nil
		_ = os.Rename(l.path, l.path+".1")
		l.openLocked()
	}
	if l.file == nil {
		l.openLocked()
	}
	if l.file == nil {
		return
	}
	n, err := l.file.WriteString(line)
	if err == nil {
		l.fileSize += int64(n)
	}
}
