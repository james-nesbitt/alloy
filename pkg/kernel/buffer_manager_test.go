package kernel

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "alloy-buffer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	bm := NewBufferManager(logger, tempDir)
	defer bm.Close()

	// Test 1: Create a buffer
	id := "test-buffer-1"
	name := "Test Buffer"
	size := 1024
	b, err := bm.CreateBuffer(id, name, size)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	if b.GetID() != id {
		t.Errorf("expected ID %s, got %s", id, b.GetID())
	}

	if b.GetSize() < size {
		t.Errorf("expected size at least %d, got %d", size, b.GetSize())
	}

	// Test 2: Get the same buffer
	b2, ok := bm.GetBuffer(id)
	if !ok {
		t.Fatal("failed to get existing buffer")
	}
	if b2.GetID() != id {
		t.Errorf("expected ID %s, got %s", id, b2.GetID())
	}

	// Test 3: Write and Read back
	content := []byte("hello world")
	sharedB := b.(*SharedBuffer)
	err = sharedB.Write(0, content)
	if err != nil {
		t.Fatalf("failed to write to buffer: %v", err)
	}

	data := b.GetData()
	if string(data[:len(content)]) != string(content) {
		t.Errorf("expected data %s, got %s", string(content), string(data[:len(content)]))
	}

	// Test 4: Verify Persistence (Mmap sync is usually automatic but file should exist)
	filePath := filepath.Join(tempDir, "buffers", id+".data")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("buffer file should exist at %s", filePath)
	}

	// Test 5: Re-opening the same manager/buffer should pick up the file
	bm2 := NewBufferManager(logger, tempDir)
	defer bm2.Close()
	b3, err := bm2.CreateBuffer(id, name, size)
	if err != nil {
		t.Fatalf("failed to re-open/create buffer: %v", err)
	}
	if string(b3.GetData()[:len(content)]) != string(content) {
		t.Errorf("persistence failed: expected %s, got %s", string(content), string(b3.GetData()[:len(content)]))
	}
}

func TestBufferManagerWriteOffset(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "alloy-buffer-offset-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	bm := NewBufferManager(logger, tempDir)
	defer bm.Close()

	id := "offset-buffer"
	b, _ := bm.CreateBuffer(id, id, 1024)
	sharedB := b.(*SharedBuffer)

	// Write at offset
	offset := 10
	content := []byte("alloy")
	err = sharedB.Write(offset, content)
	if err != nil {
		t.Errorf("write at offset failed: %v", err)
	}

	data := b.GetData()
	if string(data[offset:offset+len(content)]) != string(content) {
		t.Errorf("expected %s at offset %d, got %s", string(content), offset, string(data[offset:offset+len(content)]))
	}

	// Test Resize
	err = sharedB.Resize(2048)
	if err != nil {
		t.Fatalf("resize failed: %v", err)
	}
	if sharedB.GetSize() != 2048 {
		t.Errorf("expected size 2048, got %d", sharedB.GetSize())
	}
	// Verify data is still there after re-mmap
	if string(sharedB.GetData()[offset:offset+len(content)]) != string(content) {
		t.Errorf("data lost after resize: expected %s, got %s", string(content), string(sharedB.GetData()[offset:offset+len(content)]))
	}

	// Test ApplyChange
	change := api.BufferChange{
		Offset:    100,
		Data:      []byte("conflict-free-edit"),
		Version:   1,
		Actor:     "user-1",
		Timestamp: 12345678,
	}
	err = sharedB.ApplyChange(change)
	if err != nil {
		t.Fatalf("ApplyChange failed: %v", err)
	}
	if string(sharedB.GetData()[100:100+len(change.Data)]) != string(change.Data) {
		t.Errorf("ApplyChange data mismatch: expected %s, got %s", string(change.Data), string(sharedB.GetData()[100:100+len(change.Data)]))
	}

	// Test auto-resize via ApplyChange
	hugeChange := api.BufferChange{
		Offset:    5000,
		Data:      []byte("way-after-current-end"),
		Version:   2,
		Actor:     "user-2",
		Timestamp: 12345679,
	}
	err = sharedB.ApplyChange(hugeChange)
	if err != nil {
		t.Fatalf("auto-resize via ApplyChange failed: %v", err)
	}
	if sharedB.GetSize() < 5000+len(hugeChange.Data) {
		t.Errorf("buffer didn't resize via ApplyChange: size %d", sharedB.GetSize())
	}
}
