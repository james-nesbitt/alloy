package frontend

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/ipc"
	"github.com/jnesbitt/alloy-go/pkg/security/identity"
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
	return filepath.Join(os.TempDir(), "alloy")
}

// Client represents a standardized frontend client for Alloy.
type Client struct {
	ipc      *ipc.Client
	messages []api.Message
	mu       sync.RWMutex
	onMsg    []func(api.Message)
	name     string
}

func NewClient(name, socket string, insecure bool) (*Client, error) {
	var tlsConfig *tls.Config
	if !insecure {
		store := identity.NewStore(GetAlloyHome())
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
		ipc:  ipcClient,
		name: name,
	}

	go c.readLoop()

	return c, nil
}

func (c *Client) readLoop() {
	async := c.ipc.Async()
	for {
		msg := <-async
		c.mu.Lock()
		c.messages = append(c.messages, msg)
		c.mu.Unlock()

		for _, handler := range c.onMsg {
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
	msg := api.Message{
		ID:        fmt.Sprintf("frontend-%d", time.Now().UnixNano()),
		Type:      api.TypeRequest,
		Sender:    c.name,
		Target:    target,
		Method:    method,
		Payload:   payload,
		Timestamp: time.Now().Unix(),
	}

	return c.ipc.Call(ctx, msg)
}

func (c *Client) Messages() []api.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]api.Message, len(c.messages))
	copy(res, c.messages)
	return res
}

func (c *Client) Close() error {
	return c.ipc.Close()
}
