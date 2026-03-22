package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/james-nesbitt/alloy/pkg/ipc"
)

func getAlloyRuntimeDir() string {
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "alloy")
	}
	// Fallback to /tmp if XDG_RUNTIME_DIR is not available
	return filepath.Join(os.TempDir(), "alloy")
}

func main() {
	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Parse command line flags
	dataDir := flag.String("data-dir", "./data", "Directory for plugin data")
	listenAddr := flag.String("listen", "unix://" + filepath.Join(getAlloyRuntimeDir(), "default.sock"), "Address to listen on (unix:// or tcp://)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	provisionFile := flag.String("provision", "", "Provisioning file for initial plugins")
	flag.Parse()

	// Adjust logging level
	if *debug {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		logger.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	// Create storage
	storagePath := filepath.Join(*dataDir, "state")
	kv, err := storage.NewFileStateStore(storagePath)
	if err != nil {
		logger.Error("failed to create storage", "error", err)
		os.Exit(1)
	}

	// Create WIT-based kernel
	k, err := kernel.NewWITKernel(logger, kv, *dataDir)
	if err != nil {
		logger.Error("failed to create WIT kernel", "error", err)
		os.Exit(1)
	}

	// Create context for kernel shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load provisioned plugins if specified
	if *provisionFile != "" {
		if err := loadProvisionedPlugins(k, *provisionFile, logger); err != nil {
			logger.Error("failed to load provisioned plugins", "error", err)
			os.Exit(1)
		}
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create health monitor
	go healthMonitor(k, logger)

	// Create and start IPC server
	server := ipc.NewServer(logger, k, nil)

	go func() {
		logger.Info("starting IPC server", "addr", *listenAddr)
		if err := server.ListenAndServe(*listenAddr); err != nil {
			logger.Error("IPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("Alloy Core started", "data_dir", *dataDir)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("shutting down")

	server.Stop()
	if err := k.Shutdown(ctx); err != nil {
		logger.Error("kernel shutdown error", "error", err)
	}
}

// loadProvisionedPlugins loads plugins from a provisioning file.
func loadProvisionedPlugins(k *kernel.WITKernel, provisionFile string, logger *slog.Logger) error {
	logger.Info("loading provisioned plugins", "file", provisionFile)
	// Provisioning logic would go here
	return nil
}

// healthMonitor monitors the health of plugins.
func healthMonitor(k *kernel.WITKernel, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metadata := k.GetPluginMetadata()
			for id, md := range metadata {
				logger.Debug("plugin status", "id", id, "capabilities", len(md.Capabilities))
			}
		}
	}
}
