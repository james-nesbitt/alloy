package native

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/security/audit"
)

// LoggerManager consumes audit events and persists them to a tamper-evident log.
type LoggerManager struct {
	logger *slog.Logger
	audit  *audit.Logger
	route  func(context.Context, api.Message)
}

func NewLoggerManager(logger *slog.Logger, dataDir string) (*LoggerManager, error) {
	a, err := audit.NewLogger(dataDir)
	if err != nil {
		return nil, err
	}
	return &LoggerManager{
		logger: logger,
		audit:  a,
	}, nil
}

func (l *LoggerManager) SetRouter(r func(context.Context, api.Message)) {
	l.route = r
	// Subscribe to audit events
	go func() {
		time.Sleep(150 * time.Millisecond)
		l.route(context.Background(), api.Message{
			ID:        "sub-logger-audit",
			Type:      api.TypeRequest,
			Sender:    l.ID(),
			Target:    "plugin-events",
			Method:    "subscribe",
			Payload:   []byte(`{"topic":"system:audit"}`),
			Timestamp: time.Now().Unix(),
		})
	}()
}

func (l *LoggerManager) ID() string { return "plugin-logger" }

func (l *LoggerManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "system:audit", Description: "Event handler for audit logs"},
	}
}

func (l *LoggerManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	if msg.Method == "system:audit" {
		var entry audit.Entry
		if err := json.Unmarshal(msg.Payload, &entry); err != nil {
			l.logger.Error("failed to unmarshal audit entry", "error", err)
			return api.Message{}, nil
		}
		l.audit.Log(entry)
	}
	return api.Message{}, nil
}

func (l *LoggerManager) Shutdown(ctx context.Context) error {
	return l.audit.Close()
}
