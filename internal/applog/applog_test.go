package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerRingBufferAndFile(t *testing.T) {
	dir := t.TempDir()
	var seen []Entry
	logger := New(dir, func(entry Entry) { seen = append(seen, entry) })
	defer logger.Close()

	for i := 0; i < maxMemoryEntries+50; i++ {
		logger.Log("info", fmt.Sprintf("line %d", i))
	}

	if len(seen) != maxMemoryEntries+50 {
		t.Fatalf("callback count = %d", len(seen))
	}
	snapshot := logger.Snapshot()
	if len(snapshot) != maxMemoryEntries {
		t.Fatalf("snapshot length = %d", len(snapshot))
	}
	if snapshot[0].Message != "line 50" {
		t.Fatalf("first retained entry = %q", snapshot[0].Message)
	}
	if snapshot[len(snapshot)-1].Message != "line 249" {
		t.Fatalf("last entry = %q", snapshot[len(snapshot)-1].Message)
	}

	data, err := os.ReadFile(filepath.Join(dir, "my-local-name.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "line 249") {
		t.Fatal("file log does not contain the latest entry")
	}
}

func TestLoggerRotatesLargeFile(t *testing.T) {
	dir := t.TempDir()
	logger := New(dir, nil)
	defer logger.Close()

	logger.Log("info", strings.Repeat("x", maxFileSize+32))
	if _, err := os.Stat(filepath.Join(dir, "my-local-name.log.1")); err != nil {
		t.Fatalf("rotated file missing: %v", err)
	}
}
