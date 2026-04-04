package kernel

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/james-nesbitt/alloy/pkg/storage"
)

func TestIdentityManagerActorPrefix(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	state := storage.NewMemoryStateStore()
	iam, err := NewIdentityManager(context.Background(), logger, state)
	if err != nil {
		t.Fatalf("failed to create IAM: %v", err)
	}

	// Test 1: Known actor (registered in bootstrap)
	if !iam.Authorize("actor:claudine", "buffer", "list") {
		t.Error("actor:claudine should be authorized for buffer:list (developer role)")
	}

	// Test 2: Unknown actor (should get default 'actor' role)
	if !iam.Authorize("actor:newbie", "chat", "send") {
		t.Error("actor:newbie should be authorized for chat:send via default 'actor' role")
	}

	// Test 3: Standard guest (no actor prefix)
	if iam.Authorize("guest-user", "intent", "propose") {
		t.Error("guest-user should NOT be authorized for intent:propose")
	}

	// Test 4: Actor with prefix but requesting restricted method
	if iam.Authorize("actor:newbie", "iam", "policy:set") {
		t.Error("actor:newbie should NOT be authorized for iam:policy:set")
	}
}
