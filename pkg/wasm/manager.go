package wasm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// KernelInternal interface to avoid circular dependency.
type KernelInternal interface {
	RegisterPlugin(p api.Plugin)
	RouteMessage(ctx context.Context, msg api.Message)
}

// Manager is an Alloy native plugin that manages WASM plugin lifecycles.
type Manager struct {
	logger  *slog.Logger
	runtime *Runtime
	kernel  KernelInternal
}

func NewManager(logger *slog.Logger, runtime *Runtime, k KernelInternal) *Manager {
	return &Manager{
		logger:  logger,
		runtime: runtime,
		kernel:  k,
	}
}

func (m *Manager) ID() string { return "plugin-wasm-manager" }

func (m *Manager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "load", Description: "Load a WASM plugin"},
	}
}

func (m *Manager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "load":
		var def struct {
			ID          string `json:"id"`
			Path        string `json:"path"`
			MemoryLimit uint64 `json:"memory_limit_mb,omitempty"`
			FuelLimit   uint64 `json:"fuel_limit,omitempty"`
		}
		if err := json.Unmarshal(msg.Payload, &def); err != nil {
			return api.Message{}, err
		}

		// Asynchronously load to avoid blocking the kernel
		go m.loadAndRegister(def.ID, def.Path, def.MemoryLimit, def.FuelLimit)

		return api.Message{
			ID:      msg.ID + "-resp",
			Type:    api.TypeResponse,
			Sender:  m.ID(),
			Target:  msg.Sender,
			Payload: []byte(`{"status":"loading"}`),
		}, nil
	}
	return api.Message{}, nil
}

func (m *Manager) loadAndRegister(id, path string, mem, fuel uint64) {
	m.logger.Info("loading wasm plugin", "id", id, "path", path)
	content, err := os.ReadFile(path)
	if err != nil {
		m.logger.Error("failed to read wasm file", "id", id, "error", err)
		return
	}

	p, err := m.runtime.LoadPlugin(context.Background(), id, content, mem, fuel)
	if err != nil {
		m.logger.Error("failed to instantiate wasm plugin", "id", id, "error", err)
		return
	}

	m.kernel.RegisterPlugin(p)
	m.logger.Info("wasm plugin registered", "id", id)

	// Register with Command Manager
	caps := p.Capabilities()
	if caps == nil { caps = []api.Capability{} }
	capsData, _ := json.Marshal(caps)
	m.kernel.RouteMessage(context.Background(), api.Message{
		ID:        "reg-cm-" + id,
		Type:      api.TypeRequest,
		Sender:    m.ID(),
		Target:    "plugin-command-manager",
		Method:    "register",
		Payload:   []byte(`{"id":"` + id + `","type":"wasm","capabilities":` + string(capsData) + `}`),
		Timestamp: time.Now().Unix(),
	})
}

func (m *Manager) Shutdown(ctx context.Context) error {
	return m.runtime.Close(ctx)
}
