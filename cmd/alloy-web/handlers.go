package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/james-nesbitt/alloy/api"
)

type SendRequest struct {
	Target  string `json:"target"`
	Method  string `json:"method"`
	Payload string `json:"payload"`
}

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

func (wf *WebFrontend) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Warn("Failed to decode send request", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	slog.Debug("Sending request to kernel", "target", req.Target, "method", req.Method)
	_, err := wf.client.Send(ctx, req.Target, req.Method, []byte(req.Payload))
	if err != nil {
		slog.Error("Kernel call failed", "error", err, "target", req.Target, "method", req.Method)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK")
}

func (wf *WebFrontend) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	slog.Info("New SSE client connected")

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
		slog.Info("SSE client disconnected")
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-msgChan:
			data, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-time.After(15 * time.Second):
			fmt.Fprintf(w, "data: {\"type\": \"keepalive\"}\n\n")
			flusher.Flush()
		}
	}
}
