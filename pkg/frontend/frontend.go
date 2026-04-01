package frontend

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/ipc"
	"github.com/james-nesbitt/alloy/pkg/security/identity"
)

// Common paths
func GetAlloyHome() string {
	if home := os.Getenv("XDG_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "alloy")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "alloy")
}

func GetAlloyRuntimeDir() string {
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "alloy")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	return filepath.Join(os.TempDir(), "alloy-"+user)
}

// ClientInterface represents the public API of the Alloy frontend client.
type ClientInterface interface {
	Send(ctx context.Context, target, method string, payload []byte) (api.Message, error)
	OnMessage(h func(api.Message))
	Close() error
	Name() string
	Actor() string
	Messages() []api.Message
	DispatchIntent(ctx context.Context, intent api.Intent) (api.Message, error)
}

// Client represents a standardized frontend client for Alloy.
type Client struct {
	ipc      *ipc.Client
	messages []api.Message
	mu       sync.RWMutex
	onMsg    []func(api.Message)
	name     string
	actor    string // The authenticated actor identity
	context  string // The current active context ID
}

func NewClient(name, socket string, insecure bool) (*Client, error) {
	return NewClientWithActor(name, name, socket, insecure)
}

func NewClientWithActor(name, actor, socket string, insecure bool) (*Client, error) {
	return NewClientWithActorAndSecurity(name, actor, socket, insecure, "")
}

func NewClientWithActorAndSecurity(name, actor, socket string, insecure bool, securityDir string) (*Client, error) {
	var tlsConfig *tls.Config
	if !insecure {
		if securityDir == "" {
			securityDir = GetAlloyHome()
		}
		store := identity.NewStore(securityDir)
		ca, err := store.InitializeMachine()
		if err != nil {
			return nil, err
		}
		tlsConfig, err = store.GetClientTLSConfig(ca, name)
		if err != nil {
			return nil, err
		}
	}

	ipcClient, err := ipc.Dial(socket, tlsConfig)
	if err != nil {
		return nil, err
	}

	c := &Client{
		ipc:   ipcClient,
		name:  name,
		actor: actor,
	}

	go c.readLoop()

	return c, nil
}

func (c *Client) readLoop() {
	async := c.ipc.Async()
	for {
		msg := <-async
		c.mu.RLock()
		c.messages = append(c.messages, msg)
		handlers := make([]func(api.Message), len(c.onMsg))
		copy(handlers, c.onMsg)
		c.mu.RUnlock()

		for _, handler := range handlers {
			handler(msg)
		}
	}
}

func (c *Client) OnMessage(h func(api.Message)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onMsg = append(c.onMsg, h)
}

func (c *Client) Send(ctx context.Context, target, method string, payload []byte) (api.Message, error) {
	c.mu.RLock()
	actor := c.actor
	contextID := c.context
	c.mu.RUnlock()

	msg := api.Message{
		ID:        fmt.Sprintf("frontend-%d", time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    c.name,
		Actor:     actor,
		Target:    target,
		Method:    method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
		Metadata:  make(map[string]any),
	}

	if contextID != "" {
		msg.Metadata["context"] = contextID
	}

	return c.ipc.Call(ctx, msg)
}

func (c *Client) DispatchIntent(ctx context.Context, intent api.Intent) (api.Message, error) {
	c.mu.RLock()
	actor := c.actor
	contextID := c.context
	c.mu.RUnlock()

	if intent.ID == "" {
		intent.ID = fmt.Sprintf("intent-%d", time.Now().UnixNano())
	}
	if intent.Sender == "" {
		intent.Sender = c.name
	}

	payload, _ := json.Marshal(intent)

	msg := api.Message{
		ID:        intent.ID,
		Type:      api.TypeRequest,
		Sender:    c.name,
		Actor:     actor,
		Target:    "kernel",
		Method:    "intent:dispatch",
		Payload:   payload,
		Timestamp: time.Now().Unix(),
		Metadata:  make(map[string]any),
	}

	if intent.ContextID != "" {
		msg.Metadata["context"] = intent.ContextID
	} else if contextID != "" {
		msg.Metadata["context"] = contextID
	}

	return c.ipc.Call(ctx, msg)
}

func (c *Client) SetContext(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.context = id
}

func (c *Client) Messages() []api.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]api.Message, len(c.messages))
	copy(res, c.messages)
	return res
}

func (c *Client) Name() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.name
}

func (c *Client) Actor() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.actor
}

func (c *Client) Close() error {
	return c.ipc.Close()
}
