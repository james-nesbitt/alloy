package ipc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jnesbitt/alloy-go/api"
)

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	dec  *json.Decoder

	mu      sync.Mutex
	resps   map[string]chan api.Message
	asyncCh chan api.Message
}

func Dial(rawAddr string, tlsConfig *tls.Config) (*Client, error) {
	network, addr := ParseAddress(rawAddr)

	var conn net.Conn
	var err error
	if tlsConfig != nil {
		conn, err = tls.Dial(network, addr, tlsConfig)
	} else {
		conn, err = net.Dial(network, addr)
	}

	if err != nil {
		return nil, err
	}

	client := &Client{
		conn:    conn,
		enc:     json.NewEncoder(conn),
		dec:     json.NewDecoder(conn),
		resps:   make(map[string]chan api.Message),
		asyncCh: make(chan api.Message, 100),
	}

	go client.readLoop()

	return client, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) readLoop() {
	for {
		var msg api.Message
		if err := c.dec.Decode(&msg); err != nil {
			return
		}

		c.mu.Lock()
		ch, ok := c.resps[msg.ID]
		if ok {
			ch <- msg
			delete(c.resps, msg.ID)
		} else {
			// No one is waiting for a response, put it in the async channel
			select {
			case c.asyncCh <- msg:
			default:
				// ignore if full
			}
		}
		c.mu.Unlock()
	}
}

// Async returns a channel that receives unsolicited messages or events.
func (c *Client) Async() <-chan api.Message {
	return c.asyncCh
}

func (c *Client) Send(msg api.Message) error {
	return c.enc.Encode(msg)
}

func (c *Client) Call(ctx context.Context, msg api.Message) (api.Message, error) {
	ch := make(chan api.Message, 1)

	c.mu.Lock()
	c.resps[msg.ID+"-resp"] = ch
	c.mu.Unlock()

	if err := c.Send(msg); err != nil {
		c.mu.Lock()
		delete(c.resps, msg.ID+"-resp")
		c.mu.Unlock()
		return api.Message{}, err
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.resps, msg.ID+"-resp")
		c.mu.Unlock()
		return api.Message{}, ctx.Err()
	}
}
