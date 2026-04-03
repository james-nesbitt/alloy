package kernel

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage/history"
	"go.opentelemetry.io/otel"
)

func TestKernelHistoryReplay(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "kernel-history-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, _ := history.NewStore(tempDir)
	defer store.Close()

	// Append some events
	store.Append(api.Message{ID: "ev1", Method: "test:1", Payload: nil, Sender: "kernel", Target: "plugin-A", Type: api.TypeRequest})
	store.Append(api.Message{ID: "ev2", Method: "test:2", Payload: nil, Sender: "kernel", Target: "plugin-A", Type: api.TypeRequest})

	receivedMessages := make(chan api.Message, 10)

	// Create a dummy replayer context
	k := &Kernel{
		logger: logger,
		plugins: map[string]api.Plugin{
			"plugin-A": &dummyPlugin{received: receivedMessages},
		},
		history:   store,
		tracer:    otel.Tracer("test"),
		telemetry: nil,
	}

	// The actual replayer function to test
	replayer := func(ctx context.Context, start, end uint64) error {
		return k.ReplayEvents(ctx, start, end)
	}

	h := NewHistoryManager(logger, store, replayer)

	// Test history:replay message
	req := struct {
		Start uint64 `json:"start"`
		End   uint64 `json:"end"`
	}{Start: 0, End: 2} // End is exclusive in GetRange [start, end)
	payload, _ := json.Marshal(req)

	msg := api.Message{
		ID:      "replay-1",
		Method:  "history:replay",
		Sender:  "tester",
		Target:  "history",
		Payload: payload,
	}

	resp, err := h.HandleMessage(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}

	if string(resp.Payload) != `{"status":"ok"}` {
		t.Errorf("expected ok status, got %s", string(resp.Payload))
	}

	// Verify messages were delivered to the plugin (order can be async)
	timeout := time.After(2 * time.Second)
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case m := <-receivedMessages:
			seen[m.Method] = true
		case <-timeout:
			t.Fatalf("timed out waiting for replayed messages, seen: %v", seen)
		}
	}

	if !seen["test:1"] || !seen["test:2"] {
		t.Errorf("did not see all expected messages, seen: %v", seen)
	}
}

type dummyPlugin struct {
	received chan api.Message
}

func (d *dummyPlugin) ID() string                     { return "dummy" }
func (d *dummyPlugin) Capabilities() []api.Capability { return nil }
func (d *dummyPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	d.received <- msg
	return api.Message{}, nil
}
func (d *dummyPlugin) Shutdown(ctx context.Context) error { return nil }
