package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jnesbitt/alloy-go/pkg/wasm/sdk-go"
)

type Buffer struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	Version   int    `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

func main() {
	wasm.SetHandler(handleMessage)
}

//go:export malloc
func malloc(size uint32) uintptr {
	return wasm.Malloc(size)
}

func handleMessage(msg wasm.Message) wasm.Message {
	switch msg.Method {
	case "open":
		var req struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(msg.Payload, &req); err != nil {
			return errorResponse(msg, "invalid request")
		}

		data := wasm.KVGet("buf:" + req.ID)
		var buf Buffer
		if data != nil {
			json.Unmarshal(data, &buf)
		} else {
			buf = Buffer{
				ID:        req.ID,
				Type:      req.Type,
				Version:   0,
				UpdatedAt: time.Now().Unix(),
			}
			data, _ = json.Marshal(buf)
			wasm.KVSet("buf:"+req.ID, data)
		}

		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-buffer-manager",
			Target:  msg.Sender,
			Payload: data,
		}

	case "update":
		var req struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Version int    `json:"version"`
		}
		json.Unmarshal(msg.Payload, &req)

		data := wasm.KVGet("buf:" + req.ID)
		if data == nil {
			return errorResponse(msg, "buffer not found")
		}
		var buf Buffer
		json.Unmarshal(data, &buf)

		if req.Version < buf.Version {
			return errorResponse(msg, "conflict")
		}

		buf.Content = req.Content
		buf.Version = req.Version + 1
		buf.UpdatedAt = time.Now().Unix()

		data, _ = json.Marshal(buf)
		wasm.KVSet("buf:"+req.ID, data)

		return wasm.Message{
			ID:      msg.ID + "-resp",
			Type:    "response",
			Sender:  "plugin-buffer-manager",
			Target:  msg.Sender,
			Payload: data,
		}

	case "ping":
		return wasm.Message{
			ID:        msg.ID + "-resp",
			Type:      "response",
			Sender:    "plugin-buffer-manager",
			Target:    msg.Sender,
			Method:    "ping",
			Payload:   []byte(`{"status":"pong"}`),
			Timestamp: time.Now().Unix(),
		}

	default:
		return wasm.Message{}
	}
}

func errorResponse(msg wasm.Message, err string) wasm.Message {
	return wasm.Message{
		ID:      msg.ID + "-resp",
		Type:    "response",
		Sender:  "plugin-buffer-manager",
		Target:  msg.Sender,
		Payload: []byte(fmt.Sprintf(`{"error":"%s"}`, err)),
	}
}
