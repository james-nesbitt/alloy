package kernel

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/james-nesbitt/alloy/api"
)

func TestIntentBroker_Delegation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Capture responses and routed messages
	var capturedMsgs []api.Message
	router := func(ctx context.Context, msg api.Message) {
		capturedMsgs = append(capturedMsgs, msg)
	}

	broker := NewIntentBroker(logger, router, nil)

	// One delegating to Another Actor
	del := api.Delegation{
		ID:       "task-123",
		Owner:    "user",
		Assignee: "actor:claudine",
		Task:     "Analyze code",
		Status:   "pending",
	}
	delPayload, _ := json.Marshal(del)

	intent := api.Intent{
		ID:      "intent-1",
		Name:    "intent:delegate",
		Sender:  "user",
		Payload: delPayload,
	}

	err := broker.Dispatch(context.Background(), intent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify the message was routed to the assignee
	found := false
	for _, m := range capturedMsgs {
		if m.Target == "actor:claudine" && m.Method == "intent:delegate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Message was not routed to actor:claudine")
	}

	// Verify delegation was recorded and status is pending (as passed)
	broker.delegationsLock.RLock()
	recorded, ok := broker.delegations["task-123"]
	broker.delegationsLock.RUnlock()
	if !ok {
		t.Fatal("Delegation was not recorded")
	}
	if recorded.Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", recorded.Status)
	}

	// Child task delegation
	childDel := api.Delegation{
		ID:       "task-123-sub",
		ParentID: "task-123",
		Owner:    "actor:claudine",
		Assignee: "actor:auditor",
		Task:     "Audit changes",
	}
	childPayload, _ := json.Marshal(childDel)

	childIntent := api.Intent{
		ID:      "intent-2",
		Name:    "intent:delegate",
		Sender:  "actor:claudine",
		Payload: childPayload,
	}

	err = broker.Dispatch(context.Background(), childIntent)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Verify child recorded and parent chain updated
	broker.delegationsLock.RLock()
	parent, _ := broker.delegations["task-123"]
	child, ok_child := broker.delegations["task-123-sub"]
	broker.delegationsLock.RUnlock()

	if !ok_child {
		t.Fatal("Child delegation not recorded")
	}
	if len(parent.Chain) != 1 || parent.Chain[0] != "task-123-sub" {
		t.Fatalf("Parent chain not updated. Chain: %v", parent.Chain)
	}

	// Complete the child task
	updateBody, _ := json.Marshal(map[string]string{"id": "task-123-sub", "status": "complete"})
	updateIntent := api.Intent{
		ID:      "intent-3",
		Name:    "intent:delegate:complete",
		Sender:  "actor:auditor",
		Payload: updateBody,
	}

	err = broker.Dispatch(context.Background(), updateIntent)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	broker.delegationsLock.RLock()
	if child.Status != "complete" {
		t.Errorf("Child status not updated, got '%s'", child.Status)
	}
	broker.delegationsLock.RUnlock()

	// Third actor in the chain (Auditor requesting help from a Reviewer)
	reviewerDel := api.Delegation{
		ID:       "task-review",
		ParentID: "task-123-sub",
		Owner:    "actor:auditor",
		Assignee: "actor:reviewer",
		Task:     "Verify audit",
	}
	reviewerPayload, _ := json.Marshal(reviewerDel)

	reviewerIntent := api.Intent{
		ID:      "intent-5",
		Name:    "intent:delegate",
		Sender:  "actor:auditor",
		Payload: reviewerPayload,
	}

	err = broker.Dispatch(context.Background(), reviewerIntent)
	if err != nil {
		t.Fatalf("Third-level delegation failed: %v", err)
	}

	// Verify the third-level delegation was recorded and tracked
	broker.delegationsLock.RLock()
	child_audit, _ := broker.delegations["task-123-sub"]
	if len(child_audit.Chain) != 1 || child_audit.Chain[0] != "task-review" {
		t.Errorf("Child audit chain not updated: %v", child_audit.Chain)
	}
	broker.delegationsLock.RUnlock()

	// Check status via intent
	statusReq, _ := json.Marshal(map[string]string{"id": "task-123"})
	statusIntent := api.Intent{
		ID:      "intent-4",
		Name:    "intent:delegate:status",
		Sender:  "user",
		Payload: statusReq,
	}

	err = broker.Dispatch(context.Background(), statusIntent)
	if err != nil {
		t.Fatalf("Status intent failed: %v", err)
	}

	// Verify status response was delivered via router
	found_resp := false
	for _, m := range capturedMsgs {
		if m.Target == "user" && m.Type == api.TypeResponse && m.Sender == "intent-broker" {
			found_resp = true
			var resp api.Delegation
			json.Unmarshal(m.Payload, &resp)
			if resp.ID != "task-123" {
				t.Errorf("Response has wrong ID: %s", resp.ID)
			}
			if len(resp.Chain) != 1 || resp.Chain[0] != "task-123-sub" {
				t.Errorf("Response chain missing child: %v", resp.Chain)
			}
		}
	}
	if !found_resp {
		t.Error("Status response not found in captured messages")
	}

	// Test deep status
	capturedMsgs = nil
	deepStatusReq, _ := json.Marshal(map[string]any{"id": "task-123", "deep": true})
	deepStatusIntent := api.Intent{
		ID:      "intent-deep",
		Name:    "intent:delegate:status",
		Sender:  "user",
		Payload: deepStatusReq,
	}

	err = broker.Dispatch(context.Background(), deepStatusIntent)
	if err != nil {
		t.Fatalf("Deep status intent failed: %v", err)
	}

	found_deep := false
	for _, m := range capturedMsgs {
		if m.Target == "user" && m.Type == api.TypeResponse {
			found_deep = true
			var resp api.Delegation
			json.Unmarshal(m.Payload, &resp)
			if len(resp.SubTasks) != 1 {
				t.Errorf("Deep response missing subtasks, got: %d", len(resp.SubTasks))
			} else {
				sub := resp.SubTasks[0]
				if sub.ID != "task-123-sub" {
					t.Errorf("Subtask ID mismatch: %s", sub.ID)
				}
				if len(sub.SubTasks) != 1 || sub.SubTasks[0].ID != "task-review" {
					t.Errorf("Deep nesting missing third-level: %v", sub.SubTasks)
				}
			}
		}
	}
	if !found_deep {
		t.Error("Deep status response not found")
	}
}
