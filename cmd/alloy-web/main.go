package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/james-nesbitt/alloy/pkg/frontend"
)

//go:embed static/* templates/*
var content embed.FS

type WebFrontend struct {
	client *frontend.Client
	port   int
}

func main() {
	port := flag.Int("port", 8080, "HTTP port for the web frontend")
	socket := flag.String("socket", "", "Path to the Alloy kernel socket")
	flag.Parse()

	if *socket == "" {
		user := os.Getenv("USER")
		if user == "" {
			user = "unknown"
		}
		*socket = filepath.Join("/tmp", fmt.Sprintf("alloy-%s", user), "kernel.sock")
	}

	client, err := frontend.NewClient(*socket, "", false)
	if err != nil {
		log.Fatalf("Failed to connect to kernel: %v", err)
	}
	defer client.Close()

	wf := &WebFrontend{
		client: client,
		port:   *port,
	}

	mux := http.NewServeMux()
	
	// API for the WASM/JS bridge
	mux.HandleFunc("/api/send", wf.handleSend)
	mux.HandleFunc("/api/events", wf.handleEvents)
	
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

func (wf *WebFrontend) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFS(content, "templates/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}

func (wf *WebFrontend) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// TODO: Forward to wf.client.Send
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

	// TODO: Subscribe to kernel events via wf.client and pipe to SSE
	
	notify := w.(http.CloseNotifier).CloseNotify()
	for {
		select {
		case <-notify:
			log.Println("SSE client disconnected")
			return
		case <-time.After(15 * time.Second):
			fmt.Fprintf(w, "data: {\"type\": \"keepalive\"}\n\n")
			flusher.Flush()
		}
	}
}
