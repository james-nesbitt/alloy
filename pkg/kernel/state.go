package kernel

import (
	"os"
	"path/filepath"
	"sync"
)

// StateStore handles persistence of plugin-specific data.
type StateStore interface {
	Get(pluginID, key string) ([]byte, error)
	Set(pluginID, key string, value []byte) error
	Delete(pluginID, key string) error
}

// FileStateStore implements StateStore using the local filesystem.
type FileStateStore struct {
	mu      sync.RWMutex
	baseDir string
}

// NewFileStateStore creates a new filesystem-backed state store.
func NewFileStateStore(baseDir string) (*FileStateStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, err
	}
	return &FileStateStore{baseDir: baseDir}, nil
}

func (s *FileStateStore) pluginDir(pluginID string) string {
	return filepath.Join(s.baseDir, pluginID)
}

func (s *FileStateStore) Get(pluginID, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.pluginDir(pluginID), key+".bin")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func (s *FileStateStore) Set(pluginID, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.pluginDir(pluginID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	path := filepath.Join(dir, key+".bin")
	return os.WriteFile(path, value, 0600)
}

func (s *FileStateStore) Delete(pluginID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.pluginDir(pluginID), key+".bin")
	return os.Remove(path)
}

// MemoryStateStore implements StateStore in-memory for testing.
type MemoryStateStore struct {
	mu   sync.RWMutex
	data map[string]map[string][]byte
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{
		data: make(map[string]map[string][]byte),
	}
}

func (m *MemoryStateStore) Get(pluginID, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if plugin, ok := m.data[pluginID]; ok {
		return plugin[key], nil
	}
	return nil, nil
}

func (m *MemoryStateStore) Set(pluginID, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[pluginID]; !ok {
		m.data[pluginID] = make(map[string][]byte)
	}
	m.data[pluginID][key] = value
	return nil
}

func (m *MemoryStateStore) Delete(pluginID, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if plugin, ok := m.data[pluginID]; ok {
		delete(plugin, key)
	}
	return nil
}
