package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

//go:embed static/* templates/*
var content embed.FS

type WebFrontend struct {
	client *frontend.Client
	port   int

	mu         sync.RWMutex
	eventChans []chan api.Message
}

func main() {
	port := flag.Int("port", 8080, "HTTP port for the web frontend")
	socket := flag.String("socket", "", "Path to the Alloy kernel socket")
	insecure := flag.Bool("insecure", true, "Disable mTLS and use insecure connection")
	flag.Parse()

	if *socket == "" {
		*socket = filepath.Join(frontend.GetAlloyRuntimeDir(), "kernel.sock")
	}

	client, err := frontend.NewClient("alloy-web", *socket, *insecure)
	if err != nil {
		log.Fatalf("Failed to connect to kernel: %v", err)
	}
	defer client.Close()

	wf := &WebFrontend{
		client:     client,
		port:       *port,
		eventChans: make([]chan api.Message, 0),
	}

	client.OnMessage(func(msg api.Message) {
		wf.broadcast(msg)
	})

	mux := http.NewServeMux()
	
	// API for the WASM/JS bridge
	mux.HandleFunc("/api/send", wf.handleSend)
	mux.HandleFunc("/api/events", wf.handleEvents)
	mux.HandleFunc("/api/commands", wf.handleCommands)
	
	// Static assets
	mux.Handle("/static/", http.FileServer(http.FS(content)))
	
	// Main UI
	mux.HandleFunc("/", wf.handleIndex)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	go func() {
		log.Printf("Alloy Web Frontend starting on http://localhost:%d", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen: %s\n", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down web frontend...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
}

func (wf *WebFrontend) broadcast(msg api.Message) {
	wf.mu.RLock()
	defer wf.mu.RUnlock()
	for _, ch := range wf.eventChans {
		select {
		case ch <- msg:
		default: // Skip if slow client
		}
	}
}

func (wf *WebFrontend) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(content, "templates/index.html")
	if err != nil {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(resp.Payload)
}

type SendRequest struct {
	Target  string `json:"target"`
	Method  string `json:"method"`
	Payload string `json:"payload"`
}

func (wf *WebFrontend) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := wf.client.Send(ctx, req.Target, req.Method, []byte(req.Payload))
	if err != nil {
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

	log.Println("New SSE client connected")

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
		log.Println("SSE client disconnected")
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
