package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

func (wf *WebFrontend) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(content, "templates/index.html")
	if err != nil {
		slog.Error("Failed to parse index template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func (wf *WebFrontend) handleCommands(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp, err := wf.client.Send(ctx, "system", "discovery:list", nil)
	if err != nil {
		slog.Warn("Failed to fetch command tree", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp.Payload)
}

func (wf *WebFrontend) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wf.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("Failed to upgrade websocket", "error", err)
		return
	}
	defer conn.Close()

	slog.Info("New WebSocket client connected")

	// Host-to-Client channel
	msgChan := make(chan api.Message, 100)
	wf.mu.Lock()
	wf.eventChans = append(wf.eventChans, msgChan)
	wf.mu.Unlock()

	defer func() {
		wf.mu.Lock()
		for i, ch := range wf.eventChans {
			if ch == msgChan {
				wf.eventChans = append(wf.eventChans[:i], wf.eventChans[i+1:]...)
				break
			}
		}
		wf.mu.Unlock()
		slog.Info("WebSocket client disconnected")
	}()

	// Write loop (from Kernel/Host to Client)
	go func() {
		for msg := range msgChan {
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}()

	// Read loop (from Client to Host/Kernel)
	for {
		var msg api.Message
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		// Ensure sender is set
		if msg.Sender == "" {
			msg.Sender = "web-client"
		}

		slog.Debug("WS Request from client", "target", msg.Target, "method", msg.Method)

		go func(m api.Message) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			resp, err := wf.client.Send(ctx, m.Target, m.Method, m.Payload)
			if err != nil {
				slog.Error("WS task failed", "error", err)
				_ = conn.WriteJSON(api.Message{
					ID:      m.ID + "-error",
					Type:    api.TypeResponse,
					Sender:  "kernel",
					Payload: []byte(fmt.Sprintf(`{"error": "%s"}`, err.Error())),
				})
			} else {
				// Send response back to the client
				_ = conn.WriteJSON(resp)
			}
		}(msg)
	}
}
