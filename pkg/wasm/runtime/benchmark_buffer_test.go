package runtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func BenchmarkSharedBufferComparison(b *testing.B) {
	size := 1024 * 1024 // 1MB buffer
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}

	b.Run("Path-A-Standard-JSON-Copy", func(b *testing.B) {
		type readResponse struct {
			ID      string `json:"id"`
			Content []byte `json:"content"`
			Version int    `json:"version"`
		}

		for i := 0; i < b.N; i++ {
			// Simulate the copy-heavy route
			respData, _ := json.Marshal(readResponse{
				ID:      "test",
				Content: payload,
				Version: i,
			})
			var decoded readResponse
			_ = json.Unmarshal(respData, &decoded)
			_ = decoded.Content[100] // simulated access
		}
	})

	b.Run("Path-B-Mmap-Zero-Copy-Direct", func(b *testing.B) {
		// In-memory simulation of how the SharedBuffer handles This
		// (once mapped, access is just reference access)
		sharedMemory := payload
		for i := 0; i < b.N; i++ {
			// Simulate direct access in guest code (once pointer is known)
			_ = sharedMemory[100] // simulated access
		}
	})

	b.Run("Path-C-Metadata-Overhead-Only", func(b *testing.B) {
		// Simulate the overhead of checking with the host for updates
		// but not copying the actual large payload
		for i := 0; i < b.N; i++ {
			msg := api.Message{
				ID:      "check-update",
				Type:    api.TypeRequest,
				Payload: []byte(`{"id":"test"}`),
			}
			_ = msg.Payload
			resp := api.Message{
				ID:      "check-update-resp",
				Type:    api.TypeResponse,
				Payload: []byte(`{"id":"test","version":1,"last_modified":12345}`),
			}
			_ = resp.Payload
		}
	})
}

func BenchmarkBufferChangePropagation(b *testing.B) {
	data := make([]byte, 1024*1024)

	b.Run("Standard-IPC-Push", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			change := api.BufferChange{
				Offset:    100,
				Data:      []byte("some-data"),
				Version:   i,
				Timestamp: time.Now().UnixNano(),
			}
			payload, _ := json.Marshal(change)
			_ = api.Message{
				Type:    api.TypeEvent,
				Target:  "events",
				Method:  "publish",
				Payload: payload,
			}
		}
	})

	b.Run("Mmap-Direct-Write", func(b *testing.B) {
		// Simulate direct write into shared memory
		for i := 0; i < b.N; i++ {
			copy(data[100:], []byte("some-data"))
		}
	})
}
