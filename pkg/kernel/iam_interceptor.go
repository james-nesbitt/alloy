package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

// IAMInterceptor implements authorization enforcement for the kernel.
type IAMInterceptor struct {
	kernel *WITKernel
}

// NewIAMInterceptor creates a new IAM interceptor.
func NewIAMInterceptor(kernel *WITKernel) *IAMInterceptor {
	return &IAMInterceptor{kernel: kernel}
}

// PreRoute checks if the actor (or sender) is authorized to perform the requested action.
func (i *IAMInterceptor) PreRoute(ctx context.Context, msg api.Message) (api.Message, bool, error) {
	i.kernel.logger.Debug("IAM PreRoute", "id", msg.ID, "sender", fmt.Sprintf("[%s]", msg.Sender), "target", msg.Target)

	sender := strings.TrimSpace(msg.Sender)
	// 1. Bypass internal and bootstrap messages from trusted system components
	if sender == "kernel" || sender == "system" || sender == "iam" || sender == "events" || sender == "command-manager" {
		return msg, true, nil
	}

	// 2. Bypass messages targeting IAM itself (to avoid deadlock)
	if msg.Target == "iam" {
		return msg, true, nil
	}

	// 3. Bypass core event reporting and discovery (open access)
	if msg.Target == "events" || msg.Target == "command-manager" {
		return msg, true, nil
	}

	// 4. Only enforce on requests (methods that perform actions)
	// Response messages are allowed through to their targets
	if msg.Type == api.TypeResponse {
		return msg, true, nil
	}

	// Heuristic fallback for responses (backwards compatibility with older plugins or missing type)
	if strings.HasSuffix(msg.ID, "-resp") || strings.HasSuffix(msg.ID, "-response") {
		i.kernel.logger.Debug("bypassing security check for response heuristic", "id", msg.ID)
		return msg, true, nil
	}

	// Simple events targeting the bus are generally allowed (permissions checked at subscription)
	if msg.Type == api.TypeEvent && (msg.Target == "events" || msg.Target == "") {
		return msg, true, nil
	}

	// 5. Determine the identity (Actor)
	actor := msg.Actor
	if actor == "" {
		actor = msg.Sender // fallback to sender if actor is not set (e.g. from an unauthenticated frontend)
	}

	// 6. Call IAM check
	// Verify if IAM plugin is actually registered/available
	i.kernel.mu.RLock()
	_, isPlugin := i.kernel.plugins["iam"]
	_, isLazy := i.kernel.metadata["iam"]
	i.kernel.mu.RUnlock()

	if !isPlugin && !isLazy {
		// If IAM is not part of the system, we bypass (useful for minimal test environments)
		i.kernel.logger.Warn("IAM plugin not found, bypassing security check")
		return msg, true, nil
	}

	authReq := struct {
		Actor  string `json:"actor"`
		Target string `json:"target"`
		Method string `json:"method"`
	}{
		Actor:  actor,
		Target: msg.Target,
		Method: msg.Method,
	}

	payload, _ := json.Marshal(authReq)
	authMsg := api.Message{
		ID:        "auth-check-" + fmt.Sprint(time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    "kernel",
		Target:    "iam",
		Method:    "check",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}

	i.kernel.logger.Debug("performing security check", "id", msg.ID, "type", string(msg.Type), "sender", fmt.Sprintf("[%s]", msg.Sender), "actor", actor, "target", msg.Target, "method", msg.Method)

	resp, err := i.kernel.handleMessageSync(ctx, authMsg)
	if err != nil {
		i.kernel.logger.Error("IAM check failed to execute", "error", err)
		// On error, fail closed (secure by default)
		return msg, false, fmt.Errorf("security system error: %w", err)
	}

	var authResp struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(resp.Payload, &authResp); err != nil {
		i.kernel.logger.Error("failed to unmarshal IAM response", "error", err)
		return msg, false, fmt.Errorf("security response invalid")
	}

	if !authResp.Allowed {
		i.kernel.logger.Warn("DENIED unauthorized access", "actor", actor, "target", msg.Target, "method", msg.Method)

		// Return an error message back to the sender if it was a request
		errorResp := api.Message{
			ID:        msg.ID + "-resp",
			Type:      api.TypeResponse,
			Sender:    "system",
			Target:    msg.Sender,
			Payload:   []byte(fmt.Sprintf(`{"error":"denied: unauthorized to call %s on %s"}`, msg.Method, msg.Target)),
			Timestamp: time.Now().Unix(),
		}
		i.kernel.RouteMessage(ctx, errorResp)

		return msg, false, nil // Stop routing
	}

	// Authorized!
	return msg, true, nil
}
