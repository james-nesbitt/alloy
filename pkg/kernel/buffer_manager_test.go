package kernel

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func TestBufferConflictResolution(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	bm := NewBufferManager(logger, t.TempDir())
	b, err := bm.CreateBuffer("test", "Test Buffer", 1024)
	if err != nil {
		t.Fatal(err)
	}

	change1 := api.BufferChange{
		Offset:    0,
		Data:      []byte("hello"),
		Version:   0,
		Actor:     "actor1",
		Timestamp: time.Now().Unix(),
	}

	if err := b.ApplyChange(change1); err != nil {
		t.Fatalf("first change failed: %v", err)
	}

	// Change 2: Concurrent with Change 1 (same base version)
	change2 := api.BufferChange{
		Offset:    2,
		Data:      []byte("xxx"),
		Version:   0, // Stale version
		Actor:     "actor2",
		Timestamp: time.Now().Unix() + 1, // Newer timestamp
	}

	// Should succeed because it is newer (LWW)
	if err := b.ApplyChange(change2); err != nil {
		t.Fatalf("concurrent newer change failed: %v", err)
	}

	// Change 3: Concurrent but older than Change 2
	change3 := api.BufferChange{
		Offset:    3,
		Data:      []byte("yyy"),
		Version:   0, // Stale version
		Actor:     "actor3",
		Timestamp: time.Now().Unix() - 10, // Older timestamp
	}

	// Should fail because it overlaps and is older
	if err := b.ApplyChange(change3); err == nil {
		t.Fatal("expected conflict error for older stale change, but it succeeded")
	}
}
