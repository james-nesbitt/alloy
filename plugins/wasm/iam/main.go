package main

import (
	"encoding/json"
	"strings"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type Policy struct {
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"` // e.g., ["plugin-chat:*", "plugin-projects:create"]
}

var (
	policies = wasm.NewKVStore[Policy]("policies")
	identities = wasm.NewKVStore[string]("identities") // mapping of Actor -> Role
)

func main() {
	p := wasm.New("plugin-iam").
		WithCapability("check", "Check if an actor is authorized for an action", "").
		WithCapability("policy:set", "Set policy for a role", "i p s").
		WithCapability("identity:set", "Set role for an actor", "i i s").
		OnInit(func() error {
			// Initialize default admin policy and first user if not exists
			if _, err := policies.Get("admin"); err != nil {
				_ = policies.Set("admin", Policy{Role: "admin", Permissions: []string{"*"}})
			}
			return nil
		})

	p.Handle("check", func(msg wasm.Message) wasm.Message {
		var req struct {
			Actor  string `json:"actor"`
			Target string `json:"target"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return wasm.Reply(msg, map[string]bool{"allowed": false})
		}

		// Systems and internal kernel calls are self-authorized in this model
		if req.Actor == "kernel" || req.Actor == "system" {
			return wasm.Reply(msg, map[string]bool{"allowed": true})
		}

		role, err := identities.Get(req.Actor)
		if err != nil {
			// If no identity is found, check if actor is 'anonymous' or use a default role
			role = "guest"
		}

		policy, err := policies.Get(role)
		if err != nil {
			return wasm.Reply(msg, map[string]bool{"allowed": false})
		}

		action := req.Target + ":" + req.Method
		for _, perm := range policy.Permissions {
			if perm == "*" || perm == action || (strings.HasSuffix(perm, ":*") && strings.HasPrefix(action, perm[:len(perm)-1])) {
				return wasm.Reply(msg, map[string]bool{"allowed": true})
			}
		}

		return wasm.Reply(msg, map[string]bool{"allowed": false})
	})

	p.Handle("policy:set", func(msg wasm.Message) wasm.Message {
		var pol Policy
		if err := json.Unmarshal(msg.Payload, &pol); err != nil {
			return wasm.ErrorReply(msg, "invalid policy")
		}
		_ = policies.Set(pol.Role, pol)
		return wasm.Reply(msg, map[string]string{"status": "ok"})
	})

	p.Handle("identity:set", func(msg wasm.Message) wasm.Message {
		var req struct {
			Actor string `json:"actor"`
			Role  string `json:"role"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return wasm.ErrorReply(msg, "invalid identity")
		}
		_ = identities.Set(req.Actor, req.Role)
		return wasm.Reply(msg, map[string]string{"status": "ok"})
	})

	p.Run()
}
