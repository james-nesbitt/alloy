package wasm

import (
	"context"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

// IAMManager handles authorization and roles.
type IAMManager struct{}

func NewIAMManager() *IAMManager {
	return &IAMManager{}
}

func (i *IAMManager) ID() string { return "plugin-iam" }

func (i *IAMManager) Capabilities() []api.Capability {
	return []api.Capability{
		{Method: "authorize", Description: "Check if an actor can perform an action"},
	}
}

func (i *IAMManager) HandleMessage(ctx context.Context, msg api.Message) (api.Message, error) {
	switch msg.Method {
	case "authorize":
		// logic for RBAC
		return api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    i.ID(),
			Target:    msg.Sender,
			Payload:   []byte(`{"allowed":true}`),
			Timestamp: time.Now().Unix(),
		}, nil
	}
	return api.Message{}, nil
}

func (i *IAMManager) Shutdown(ctx context.Context) error {
	return nil
}
