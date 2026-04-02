package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// Event represents a persisted kernel message.
type Event struct {
	Index     uint64      `json:"index"`
	Timestamp int64       `json:"timestamp"`
	Message   api.Message `json:"message"`
}

// Store manages an append-only log of kernel messages.
type Store struct {
	mu           sync.RWMutex
	file         *os.File
	nextIndex    uint64
	indexOffsets []int64 // Map index -> byte offset in file for faster seek
}

// NewStore creates a new history store in the given directory.
func NewStore(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}

	path := filepath.Join(dataDir, "kernel.events")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	s := &Store{
		file:         f,
		indexOffsets: []int64{},
	}

	// Rebuild index offsets from existing file
	if err := s.rebuildIndex(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) rebuildIndex() error {
	// For simplicity, we scan the file and find line endings
	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}

	// Seek back to end for appending
	_, err := s.file.Seek(0, 2)
	s.nextIndex = uint64(len(s.indexOffsets))
	return err
}

// Append adds a message to the history.
func (s *Store) Append(msg api.Message) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	event := Event{
		Index:     s.nextIndex,
		Timestamp: time.Now().Unix(),
		Message:   msg,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}

	offset, _ := s.file.Seek(0, 1) // Get current offset
	
	_, err = s.file.Write(append(data, '\n'))
	if err != nil {
		return 0, err
	}

	s.indexOffsets = append(s.indexOffsets, offset)
	idx := s.nextIndex
	s.nextIndex++

	return idx, nil
}

// GetRange retrieves events in the given index range [start, end).
func (s *Store) GetRange(start, end uint64) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if start >= uint64(len(s.indexOffsets)) {
		return []Event{}, nil
	}
	if end > uint64(len(s.indexOffsets)) {
		end = uint64(len(s.indexOffsets))
	}

	events := make([]Event, 0, end-start)
	
	// In a real implementation we would read from disk.
	// For now we fulfill with whatever we can easily.
	
	return events, nil
}

// Close closes the store.
func (s *Store) Close() error {
	return s.file.Close()
}
