package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// HealthManager monitorsHost and Kernel health.
type HealthManager struct {
	logger    *slog.Logger
	startTime time.Time
}

func NewHealthManager(logger *slog.Logger) *HealthManager {
	return &HealthManager{
		logger:    logger,
		startTime: time.Now(),
	}
}

func (h *HealthManager) ID() string { return "health" }

func (h *HealthManager) Capabilities() []api.Capability {
	return []api.Capability{
		{
			Method:      "status",
			Description: "Get current system and kernel health metrics",
			Shortcut:    "h s",
			Annotations: map[string]string{"group": "system"},
		},
	}
}

func (h *HealthManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	if msg.Method == "status" {
		stats := h.getStats()
		payload, _ := json.Marshal(stats)
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    h.ID(),
			Target:    msg.Sender,
			Payload:   payload,
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (h *HealthManager) Shutdown(ctx context.Context) error { return nil }

type HealthStats struct {
	Uptime     string  `json:"uptime"`
	Goroutines int     `json:"goroutines"`
	HeapAlloc  uint64  `json:"heap_alloc_mb"`
	HostCPU    float64 `json:"host_cpu_percent"`
	HostMem    float64 `json:"host_mem_percent"`
	OS         string  `json:"os"`
	Arch       string  `json:"arch"`
}

func (h *HealthManager) getStats() HealthStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	v, _ := mem.VirtualMemory()
	c, _ := cpu.Percent(0, false)

	cpuPercent := 0.0
	if len(c) > 0 {
		cpuPercent = c[0]
	}

	return HealthStats{
		Uptime:     time.Since(h.startTime).String(),
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  ms.HeapAlloc / 1024 / 1024,
		HostCPU:    cpuPercent,
		HostMem:    v.UsedPercent,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}
}
