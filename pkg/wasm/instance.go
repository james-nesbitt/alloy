package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

type InstanceStatus string

const (
	StatusRunning InstanceStatus = "running"
	StatusCrashed InstanceStatus = "crashed"
	StatusStopped InstanceStatus = "stopped"
)

// Instance represents a single loaded WASM plugin.
type Instance struct {
	id          string
	ctx         context.Context
	cancel      context.CancelFunc
	mod         wazeroapi.Module
	logger      *slog.Logger
	defaultFuel uint64
	mu          sync.Mutex

	inPtr  uint32 // Pre-provided pointer to guest inBuffer
	outPtr uint32 // Pre-provided pointer to guest outBuffer

	// Pull model support
	msgChan  chan api.Message
	respChan chan api.Message

	status    InstanceStatus
	lastError string

	capabilities []api.Capability
}

// HandleMessage sends a message to the WASM plugin and waits for a response.
func (i *Instance) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	// If the message is system:capabilities, return them directly if we have them cached
	if msg.Method == "system:capabilities" && len(i.capabilities) > 0 {
		payload, _ := json.Marshal(i.capabilities)
		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  i.id,
			Target:  msg.Sender,
			Method:  msg.Method,
			Payload: payload,
		}, nil
	}

	if i.status == StatusCrashed {
		return api.Message{}, fmt.Errorf("plugin %s is crashed: %s", i.id, i.lastError)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	// 1. Send message to guest via the pull channel
	select {
	case i.msgChan <- msg:
		// success
	case <-ctx.Done():
		return api.Message{}, ctx.Err()
	}

	// 2. Wait for response from guest via the response channel
	// Note: The guest calls alloy_send_response which populates this channel.
	select {
	case resp := <-i.respChan:
		return resp, nil
	case <-time.After(1 * time.Minute):
		return api.Message{}, fmt.Errorf("timed out waiting for guest response (1m)")
	case <-ctx.Done():
		return api.Message{}, fmt.Errorf("waiting for guest response failed: %w", ctx.Err())
	}
}

func (i *Instance) ID() string {
	return i.id
}

func (i *Instance) Capabilities() []api.Capability {
	i.mu.Lock()
	defer i.mu.Unlock()

    if len(i.capabilities) > 0 {
        return i.capabilities
    }

	fn := i.mod.ExportedFunction("alloy_capabilities")
	if fn == nil {
		return nil
	}

	results, err := fn.Call(context.Background())
	if err != nil || len(results) == 0 {
		return nil
	}
	size := uint32(results[0])
	if size == 0 {
		return nil
	}

	getOutPtr := i.mod.ExportedFunction("alloy_get_out_ptr")
	ptrResults, _ := getOutPtr.Call(context.Background())
	ptr := uint32(ptrResults[0])

	buf, ok := i.mod.Memory().Read(ptr, size)
	if !ok {
		return nil
	}

	var caps []api.Capability
	if err := json.Unmarshal(buf, &caps); err != nil {
		return nil
	}

	i.capabilities = caps
	return caps
}

func (i *Instance) Status() (InstanceStatus, string) {
	return i.status, i.lastError
}

func (i *Instance) Shutdown(ctx context.Context) error {
	i.status = StatusStopped
	return i.mod.Close(ctx)
}
