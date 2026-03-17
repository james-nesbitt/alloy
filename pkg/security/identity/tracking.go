package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// InstanceInfo represents a running core instance's metadata.
type InstanceInfo struct {
	Name      string    `json:"name"`
	PID       int       `json:"pid"`
	Socket    string    `json:"socket"`
	StartTime time.Time `json:"start_time"`
}

// WriteInstanceInfo records the instance metadata to the runtime directory.
func (s *Store) WriteInstanceInfo(info InstanceInfo, runtimeDir string) error {
	dir := filepath.Join(runtimeDir, "instances", info.Name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "info.json"), data, 0644)
}

// ClearInstanceInfo removes the instance metadata.
func (s *Store) ClearInstanceInfo(name string, runtimeDir string) error {
	path := filepath.Join(runtimeDir, "instances", name, "info.json")
	return os.Remove(path)
}

// ListInstances returns a list of all discovered instance metadata.
func (s *Store) ListInstances(runtimeDir string) ([]InstanceInfo, error) {
	instancesDir := filepath.Join(runtimeDir, "instances")
	entries, err := os.ReadDir(instancesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var infos []InstanceInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		path := filepath.Join(instancesDir, entry.Name(), "info.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var info InstanceInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}

		// Optional: check if PID is still alive
		if process, err := os.FindProcess(info.PID); err == nil {
			// On Unix, FindProcess always succeeds. Signal 0 checks if it's actually alive.
			if err := process.Signal(syscall.Signal(0)); err != nil {
				// Process is dead, cleanup? Maybe not automatically here.
				continue
			}
		} else {
			continue
		}

		infos = append(infos, info)
	}
	return infos, nil
}
