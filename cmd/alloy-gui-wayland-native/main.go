package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/frontend"
	"github.com/rajveermalviya/go-wayland/wayland/client"
)

type appState struct {
	display    *client.Display
	registry   *client.Registry
	compositor *client.Compositor
	shm        *client.Shm
	surface    *client.Surface
	logger     *slog.Logger
}

func main() {
	socketAddr := flag.String("socket", "tcp://127.0.0.1:4242", "Alloy core IPC address")
	insecure := flag.Bool("insecure", true, "Disable mTLS for local testing")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Info("Starting Pure Go Wayland Native GUI", "addr", *socketAddr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Initialize shared frontend client (IPC part)
	ipcClient, err := frontend.NewClient("alloy-gui-wayland", *socketAddr, *insecure)
	if err != nil {
		logger.Error("failed to create frontend client", "error", err)
		os.Exit(1)
	}
	defer ipcClient.Close()

	// 2. Pure Go Wayland connection
	// The library uses environment variables to find the socket if we don't provide a path.
	display, err := client.Connect("")
	if err != nil {
		logger.Error("failed to connect to Wayland compositor (is WAYLAND_DISPLAY set?)", "error", err)
		// We proceed anyway to show the IPC still works, but in a real app we'd exit.
	} else {
		defer display.Context().Close()
		logger.Info("Connected to Wayland compositor")

		// Display Event Loop
		go func() {
			for {
				if err := display.Context().Dispatch(); err != nil {
					logger.Error("Wayland dispatch error", "error", err)
					return
				}
			}
		}()
	}

	// Send an initial ping request to Alloy core via IPC
	go func() {
		resp, err := ipcClient.Send(ctx, "kernel", "ping", nil)
		if err == nil {
			logger.Info("Alloy core reachable", "resp_id", resp.ID)
		} else {
			logger.Error("failed to reach Alloy core", "error", err)
		}
	}()

	// Listener for Alloy Core events
	ipcClient.OnMessage(func(msg api.Message) {
		logger.Info("Kernel Event received", "method", msg.Method, "id", msg.ID)
	})

	<-ctx.Done()
	logger.Info("GUI shutting down")
}
