package history

import (
	"github.com/james-nesbitt/alloy/api"
	"os"
	"testing"
)

func TestStoreDurability(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "history-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	s, err := NewStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Append some events
	messages := []api.Message{
		{ID: "1", Method: "test:1", Sender: "kernel", Target: "events"},
		{ID: "2", Method: "test:2", Sender: "kernel", Target: "events"},
	}

	for _, m := range messages {
		if _, err := s.Append(m); err != nil {
			t.Fatal(err)
		}
	}

	s.Close()

	// Re-open and verify rebuildIndex
	s2, err := NewStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if len(s2.indexOffsets) != 2 {
		t.Errorf("expected 2 index offsets, got %d", len(s2.indexOffsets))
	}

	rangeEvents, err := s2.GetRange(0, 2)
	if err != nil {
		t.Fatal(err)
	}

	if len(rangeEvents) != 2 {
		t.Errorf("expected 2 events in range, got %d", len(rangeEvents))
	}
}

func TestStoreTruncate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "history-truncate-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	s, err := NewStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		s.Append(api.Message{ID: "test"})
	}

	if err := s.TruncateTo(2); err != nil {
		t.Fatal(err)
	}

	if len(s.indexOffsets) != 2 { // 0, 1 remains, 2 is truncated out
		t.Errorf("expected 2 index offsets after truncate to 2, got %d", len(s.indexOffsets))
	}

	s.Close()

	s2, err := NewStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if len(s2.indexOffsets) != 2 { // 0, 1 remains
		t.Errorf("expected 2 index offsets after reload, got %d", len(s2.indexOffsets))
	}
}
