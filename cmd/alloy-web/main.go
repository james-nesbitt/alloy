package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

//go:embed static/* templates/*
var content embed.FS

type WebFrontend struct {
	client frontend.ClientInterface
	port   int

	mu         sync.RWMutex
	eventChans []chan api.Message

	upgrader websocket.Upgrader
}

func main() {
	port := flag.Int("port", 8080, "HTTP port for the web frontend")
	socket := flag.String("socket", "", "Path to the Alloy kernel socket")
	actor := flag.String("actor", "", "Actor identity")
	debug := flag.Bool("debug", false, "Enable debug logging")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())
	cmdutil.SetupLogger(*debug)

	client, err := cmdutil.InitClient("alloy-web", *actor, *socket, sf)
	if err != nil {
		slog.Error("Failed to connect to kernel", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	wf := &WebFrontend{
		client:     client,
		port:       *port,
		eventChans: make([]chan api.Message, 0),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // Allow for exploration
		},
	}

	client.OnMessage(func(msg api.Message) {
		wf.broadcast(msg)
	})

	mux := http.NewServeMux()

	mux.HandleFunc("/api/commands", wf.handleCommands)
	mux.HandleFunc("/ws", wf.handleWS)

	// Static assets
	mux.Handle("/static/", http.FileServer(http.FS(content)))

	// Main UI
	mux.HandleFunc("/", wf.handleIndex)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	go func() {
		slog.Info("Alloy Web Frontend starting", "url", fmt.Sprintf("http://localhost:%d", *port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP Server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down web frontend...")
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
