package history

import (
	"bufio"
	"encoding/json"
	"io"
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}

	s.indexOffsets = []int64{}
	s.nextIndex = 0

	reader := bufio.NewReader(s.file)
	offset := int64(0)

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			s.indexOffsets = append(s.indexOffsets, offset)
			offset += int64(len(line))
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	s.nextIndex = uint64(len(s.indexOffsets))
	_, err := s.file.Seek(0, 2)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	total := uint64(len(s.indexOffsets))
	if start >= total {
		return []Event{}, nil
	}
	if end == 0 || end > total {
		end = total
	}

	events := make([]Event, 0, end-start)

	// Read entire file for simplicity for now, or seek for each?
	// Given this is a staff engineer job, let's seek correctly if we can.
	// But actually, we don't store the offsets persistently yet (they are in memory).

	// Open a separate read-only file handle for concurrent access or just use the existing one with proper locking?
	// Since we already have the RLock, we can seek the existing file.

	for i := start; i < end; i++ {
		offset := s.indexOffsets[i]
		if _, err := s.file.Seek(offset, 0); err != nil {
			return events, err
		}

		// Read one line (event)
		// We could use a scanner, but we just need the next JSON object.
		var ev Event
		decoder := json.NewDecoder(s.file)
		if err := decoder.Decode(&ev); err != nil {
			return events, err
		}
		events = append(events, ev)
	}

	return events, nil
}

// TruncateTo removes events from the history starting from the given index (inclusive).
func (s *Store) TruncateTo(index uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := uint64(len(s.indexOffsets))
	if index >= total {
		return nil
	}

	offset := s.indexOffsets[index]
	// Also seek back to the head of the file for the truncate to take effect in O_APPEND mode?
	// Actually Truncate works regardless of Seek.
	if err := s.file.Truncate(offset); err != nil {
		return err
	}

	if _, err := s.file.Seek(offset, 0); err != nil {
		return err
	}

	s.indexOffsets = s.indexOffsets[:index]
	s.nextIndex = index
	return nil
}

// Clear truncates the history store. Use with caution.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.file.Truncate(0); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}
	s.indexOffsets = []int64{}
	s.nextIndex = 0
	return nil
}

// Restore clears the history and imports the given events.
func (s *Store) Restore(events []Event) error {
	if err := s.Clear(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}

		offset, _ := s.file.Seek(0, 1) // Get current offset
		_, err = s.file.Write(append(data, '\n'))
		if err != nil {
			return err
		}

		s.indexOffsets = append(s.indexOffsets, offset)
	}

	s.nextIndex = uint64(len(s.indexOffsets))
	return nil
}

// Close closes the store.
func (s *Store) Close() error {
	return s.file.Close()
}
