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
	mod         wazeroapi.Module
	logger      *slog.Logger
	defaultFuel uint64
	mu          sync.Mutex

	status    InstanceStatus
	lastError string
}

// HandleMessage passes an Alloy Message to the guest via the Guest ABI.
func (i *Instance) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Resource enforcement: Timeout based on "fuel"
	// For now, treat fuel as milliseconds for simplicity in sandboxing.
	if i.defaultFuel > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(i.defaultFuel)*time.Millisecond)
		defer cancel()
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return api.Message{}, err
	}

	size := uint64(len(payload))
	// 1. Allocate memory in guest for request
	malloc := i.mod.ExportedFunction("alloy_malloc")
	if malloc == nil {
		return api.Message{}, fmt.Errorf("plugin missing 'alloy_malloc' export")
	}

	results, err := malloc.Call(ctx, size)
	if err != nil {
		return api.Message{}, fmt.Errorf("failed to allocate memory in guest: %w", err)
	}
	ptr := uint32(results[0])

	// Ensure we free the allocated pointer on the way out
	defer func() {
		if free := i.mod.ExportedFunction("alloy_free"); free != nil {
			_, _ = free.Call(ctx, uint64(ptr))
		}
	}()

	// 2. Write payload to guest memory
	if !i.mod.Memory().Write(ptr, payload) {
		return api.Message{}, fmt.Errorf("failed to write payload to guest memory")
	}

	// 3. Invoke handler
	handler := i.mod.ExportedFunction("alloy_handle_message")
	if handler == nil {
		return api.Message{}, fmt.Errorf("plugin missing 'alloy_handle_message' export")
	}

	handleResults, err := handler.Call(ctx, uint64(ptr), size)
	if err != nil {
		i.status = StatusCrashed
		i.lastError = err.Error()
		i.logger.Error("wasm plugin crashed", "id", i.id, "error", err)
		return api.Message{}, fmt.Errorf("failed to call alloy_handle_message: %w", err)
	}

	packed := handleResults[0]
	respPtr := uint32(packed >> 32)
	respSize := uint32(packed)

	if respSize == 0 {
		return api.Message{}, nil
	}

	// 4. Read response from guest memory
	respBuf, ok := i.mod.Memory().Read(respPtr, respSize)
	if !ok {
		return api.Message{}, fmt.Errorf("failed to read response from guest memory")
	}

	// Cleanup response buffer in guest now that we've read it
	if free := i.mod.ExportedFunction("alloy_free"); free != nil {
		_, _ = free.Call(ctx, uint64(respPtr))
	}

	var resp api.Message
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return api.Message{}, fmt.Errorf("failed to unmarshal guest response: %w", err)
	}

	return resp, nil
}

func (i *Instance) ID() string {
	return i.id
}

func (i *Instance) Capabilities() []api.Capability {
	i.mu.Lock()
	defer i.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	fn := i.mod.ExportedFunction("alloy_capabilities")
	if fn == nil {
		return nil
	}

	results, err := fn.Call(ctx)
	if err != nil || len(results) == 0 {
		return nil
	}

	packed := results[0]
	ptr := uint32(packed >> 32)
	size := uint32(packed)

	if size == 0 {
		return nil
	}

	buf, ok := i.mod.Memory().Read(ptr, size)
	if !ok {
		return nil
	}

	var caps []api.Capability
	if err := json.Unmarshal(buf, &caps); err != nil {
		return nil
	}

	return caps
}

func (i *Instance) Shutdown(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.status = StatusStopped
	return i.mod.Close(ctx)
}

func (i *Instance) Status() (InstanceStatus, string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.status, i.lastError
}

func (i *Instance) IsCrashed() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.status == StatusCrashed
}
