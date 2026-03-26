package kernel

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/james-nesbitt/alloy/api"
)

// SharedBuffer represents a memory-mapped buffer accessible by multiple plugins.
type SharedBuffer struct {
	ID           string
	Name         string
	Path         string
	Size         int
	Data         []byte
	File         *os.File
	LastModified int64
	Version      int
	History      []api.BufferChange
	mu           sync.RWMutex
}

// Satisfy SharedBuffer interface for api.SharedBuffer
func (b *SharedBuffer) GetID() string          { return b.ID }
func (b *SharedBuffer) GetName() string        { return b.Name }
func (b *SharedBuffer) GetData() []byte        { return b.Data }
func (b *SharedBuffer) GetSize() int           { return b.Size }
func (b *SharedBuffer) Lock()                  { b.mu.Lock() }
func (b *SharedBuffer) Unlock()                { b.mu.Unlock() }
func (b *SharedBuffer) GetVersion() int        { return b.Version }
func (b *SharedBuffer) GetLastModified() int64 { return b.LastModified }

// Resize increases the size of the shared buffer.
func (b *SharedBuffer) Resize(newSize int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.resize(newSize)
}

func (b *SharedBuffer) resize(newSize int) error {
	if newSize <= b.Size {
		return nil
	}

	// Unmap old data
	if b.Data != nil {
		if err := syscall.Munmap(b.Data); err != nil {
			return fmt.Errorf("failed to munmap old data: %w", err)
		}
	}

	// Truncate file
	if err := b.File.Truncate(int64(newSize)); err != nil {
		return fmt.Errorf("failed to truncate file: %w", err)
	}

	// Mmap new data
	data, err := syscall.Mmap(int(b.File.Fd()), 0, newSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("failed to mmap resized file: %w", err)
	}

	b.Data = data
	b.Size = newSize
	return nil
}

// ApplyChange updates the buffer data and tracks the history for conflict resolution.
func (b *SharedBuffer) ApplyChange(change api.BufferChange) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if change.Offset+len(change.Data) > b.Size {
		// Auto-resize for incoming changes
		if err := b.resize(change.Offset + len(change.Data) + 1024); err != nil {
			return err
		}
	}

	copy(b.Data[change.Offset:], change.Data)
	b.Version++
	b.LastModified = change.Timestamp

	// Track in history (Limited to last 100 changes for "lite" model)
	b.History = append(b.History, change)
	if len(b.History) > 100 {
		b.History = b.History[1:]
	}

	return nil
}

// BufferManager handles the lifecycle of shared memory-mapped buffers.
type BufferManager struct {
	logger  *slog.Logger
	dataDir string
	buffers map[string]*SharedBuffer
	mu      sync.RWMutex
}

// NewBufferManager creates a new BufferManager.
func NewBufferManager(logger *slog.Logger, dataDir string) *BufferManager {
	return &BufferManager{
		logger:  logger,
		dataDir: dataDir,
		buffers: make(map[string]*SharedBuffer),
	}
}

// CreateBuffer creates or opens a memory-mapped buffer.
func (bm *BufferManager) CreateBuffer(id, name string, initialSize int) (api.SharedBuffer, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if b, ok := bm.buffers[id]; ok {
		return b, nil
	}

	path := filepath.Join(bm.dataDir, "buffers", id+".data")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create buffers dir: %w", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open buffer file: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat buffer file: %w", err)
	}

	if initialSize <= 0 {
		initialSize = 1024 * 64 // Default to 64KB
	}

	if info.Size() < int64(initialSize) {
		if err := file.Truncate(int64(initialSize)); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to truncate buffer file: %w", err)
		}
	} else {
		initialSize = int(info.Size())
	}

	data, err := syscall.Mmap(int(file.Fd()), 0, initialSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to mmap buffer file: %w", err)
	}

	b := &SharedBuffer{
		ID:   id,
		Name: name,
		Path: path,
		Size: initialSize,
		Data: data,
		File: file,
	}

	bm.buffers[id] = b
	bm.logger.Info("created shared mmap buffer", "id", id, "size", initialSize, "path", path)
	return b, nil
}

// GetBuffer retrieves a shared buffer by ID.
func (bm *BufferManager) GetBuffer(id string) (api.SharedBuffer, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	b, ok := bm.buffers[id]
	if !ok {
		return nil, false
	}
	return b, ok
}

// Write (Appends or Overwrites data)
func (b *SharedBuffer) Write(offset int, content []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if offset+len(content) > b.Size {
		return fmt.Errorf("buffer overflow: resize required")
	}

	copy(b.Data[offset:], content)
	b.Version++
	return nil
}

// Close closes all buffers.
func (bm *BufferManager) Close() error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, b := range bm.buffers {
		_ = syscall.Munmap(b.Data)
		_ = b.File.Close()
	}
	return nil
}
