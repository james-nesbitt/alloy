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

	"github.com/jnesbitt/alloy-go/pkg/kernel"
	"github.com/jnesbitt/alloy-go/pkg/storage"
)

func main() {
	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Parse command line flags
	dataDir := flag.String("data-dir", "./data", "Directory for plugin data")
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

	// Start health monitor
	go healthMonitor(k, logger)

	logger.Info("Alloy Core started", "data_dir", *dataDir)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("shutting down")
	
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
