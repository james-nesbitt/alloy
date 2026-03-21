package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

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

	capabilities []api.Capability
}

// HandleMessage passes an Alloy Message to the guest via the Guest ABI.
func (i *Instance) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	// Add a small delay if this is the very first message after loading to avoid 
	// potential wasip1/tinygo runtime initialization races
	i.mu.Lock()
	defer i.mu.Unlock()

	i.logger.Debug("wasm HandleMessage start", "id", i.id, "method", msg.Method)

	if i.status == StatusCrashed {
		return api.Message{}, fmt.Errorf("plugin %s is crashed: %s", i.id, i.lastError)
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return api.Message{}, err
	}

	// 1. Get input buffer pointer
	getInPtr := i.mod.ExportedFunction("alloy_get_in_ptr")
	if getInPtr == nil {
		return api.Message{}, fmt.Errorf("plugin missing 'alloy_get_in_ptr' export")
	}
	results, err := getInPtr.Call(ctx)
	if err != nil {
		return api.Message{}, fmt.Errorf("failed to get input pointer: %w", err)
	}
	inPtr := uint32(results[0])

	// 2. Write payload to guest input buffer
	if !i.mod.Memory().Write(inPtr, payload) {
		return api.Message{}, fmt.Errorf("failed to write payload to guest memory")
	}

	// 3. Call handler
	handler := i.mod.ExportedFunction("alloy_handle_message")
	if handler == nil {
		return api.Message{}, fmt.Errorf("plugin missing 'alloy_handle_message' export")
	}

	handleResults, err := handler.Call(ctx, uint64(len(payload)))
	if err != nil {
		i.status = StatusCrashed
		i.lastError = err.Error()
		i.logger.Error("wasm plugin crashed", "id", i.id, "error", err)
		return api.Message{}, fmt.Errorf("failed to call alloy_handle_message: %w", err)
	}

	respSize := uint32(handleResults[0])
	if respSize == 0 {
		return api.Message{}, nil
	}

	// 4. Get output buffer pointer
	getOutPtr := i.mod.ExportedFunction("alloy_get_out_ptr")
	if getOutPtr == nil {
		return api.Message{}, fmt.Errorf("plugin missing 'alloy_get_out_ptr' export")
	}
	results, err = getOutPtr.Call(ctx)
	if err != nil {
		return api.Message{}, fmt.Errorf("failed to get output pointer: %w", err)
	}
	outPtr := uint32(results[0])

	// 5. Read response
	respBuf, ok := i.mod.Memory().Read(outPtr, respSize)
	if !ok {
		return api.Message{}, fmt.Errorf("failed to read response from guest memory")
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

	side := uint32(results[0])
	if side == 0 {
		return nil
	}

	getOutPtr := i.mod.ExportedFunction("alloy_get_out_ptr")
	ptrResults, _ := getOutPtr.Call(context.Background())
	ptr := uint32(ptrResults[0])

	buf, ok := i.mod.Memory().Read(ptr, side)
	if !ok {
		return nil
	}

	var caps []api.Capability
	_ = json.Unmarshal(buf, &caps)
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
