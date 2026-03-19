package tests

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// targetPlugin is a utility mock for testing routing and interception.
type targetPlugin struct {
	received chan api.Message
}

func (t *targetPlugin) ID() string                      { return "target-plugin" }
func (t *targetPlugin) Capabilities() []api.Capability { return nil }
func (t *targetPlugin) Shutdown(ctx context.Context) error { return nil }
func (t *targetPlugin) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	t.received <- msg
	return api.Message{}, nil
}

// sendMsg is a helper to encode and send a message over a network connection in tests.
func sendMsg(t *testing.T, conn net.Conn, msg api.Message) {
	t.Helper()
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		t.Fatalf("failed to send message: %v", err)
	}
}

// awaitResponse waits for a message with a specific ID from the decoder.
func awaitResponse(t *testing.T, decoder *json.Decoder, id string) api.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var resp api.Message
		if err := decoder.Decode(&resp); err != nil {
			if err == io.EOF {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.ID == id || resp.ID == id+"-resp" {
			return resp
		}
	}
	t.Fatalf("timed out waiting for response ID %s", id)
	return api.Message{}
}
