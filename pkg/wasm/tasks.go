package wasm

import (
	"context"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// TaskRunner handles scheduled operations.
type TaskRunner struct{}

func NewTaskRunner() *TaskRunner {
	return &TaskRunner{}
}

func (t *TaskRunner) ID() string { return "plugin-tasks" }

func (t *TaskRunner) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "schedule", Description: "Schedule a task to run at an interval"},
		{Method: "cancel", Description: "Cancel a scheduled task"},
	}
}

func (t *TaskRunner) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "schedule":
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    t.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"task_id":"mock-task-1"}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (t *TaskRunner) Shutdown(ctx context.Context) error {
	return nil
}
