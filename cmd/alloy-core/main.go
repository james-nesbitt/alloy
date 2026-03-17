package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/ipc"
	"github.com/jnesbitt/alloy-go/pkg/kernel"
	"github.com/jnesbitt/alloy-go/pkg/security/audit"
	"github.com/jnesbitt/alloy-go/pkg/security/identity"
	"github.com/jnesbitt/alloy-go/pkg/wasm"
)

func getAlloyDataDir() string {
	if data := os.Getenv("XDG_DATA_HOME"); data != "" {
		return filepath.Join(data, "alloy")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "alloy")
}

func getAlloyHome() string {
	if home := os.Getenv("XDG_CONFIG_HOME"); home != "" {
		return filepath.Join(home, "alloy")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "alloy")
}

func getAlloyRuntimeDir() string {
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "alloy")
	}
	// Fallback to /tmp if XDG_RUNTIME_DIR is not available
	return filepath.Join(os.TempDir(), "alloy")
}

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	insecure := flag.Bool("insecure", false, "Disable mTLS")
	alloyHome := flag.String("home", getAlloyHome(), "Directory for alloy config and identities")
	instanceName := flag.String("name", "default", "Instance name")

	defaultSocket := filepath.Join(getAlloyRuntimeDir(), "default.sock")
	socket := flag.String("socket", defaultSocket, "Socket address to listen for IPC connections (supports tcp://, unix://, or local path)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	// Audit Setup
	auditLogger, err := audit.NewLogger(getAlloyDataDir())
	if err != nil {
		logger.Error("failed to initialize audit logger", "error", err)
		os.Exit(1)
	}
	defer auditLogger.Close()
	auditLogger.Log(audit.Entry{Action: "startup", Actor: "system", Status: "success", Details: map[string]any{"pid": os.Getpid()}})

	// PKI / Identity Setup
	store := identity.NewStore(*alloyHome)
	var tlsConfig *tls.Config
	if !*insecure {
		ca, err := store.InitializeMachine()
		if err != nil {
			logger.Error("failed to initialize machine pki", "error", err)
			os.Exit(1)
		}

		pair, err := store.CreateInstanceIdentity(ca, *instanceName)
		if err != nil {
			logger.Error("failed to create instance identity", "error", err)
			os.Exit(1)
		}

		tlsConfig, err = store.GetServerTLSConfig(ca, pair)
		if err != nil {
			logger.Error("failed to create tls config", "error", err)
			os.Exit(1)
		}
	}

	// State Setup
	stateStore, err := kernel.NewFileStateStore(filepath.Join(getAlloyDataDir(), "state"))
	if err != nil {
		logger.Error("failed to initialize state store", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k := kernel.New(logger, auditLogger, stateStore)

	// Register Core Plugins
	em := wasm.NewEventManager(logger)
	em.SetRouter(k.RouteMessage)
	k.RegisterPlugin(em)

	// Subscribe Command Manager to registration events
	cm := wasm.NewCommandManager()
	k.RegisterPlugin(cm)

	// Subscribe Command Manager to registration events early
	k.RouteMessage(context.Background(), api.Message{
		ID:      "sub-reg",
		Type:    api.TypeRequest,
		Sender:  cm.ID(),
		Target:  "plugin-events",
		Method:  "subscribe",
		Payload: []byte(`{"topic":"component:registered"}`),
	})

	k.RegisterPlugin(wasm.NewIAMManager())
	k.RegisterPlugin(wasm.NewSecretManager())
	k.RegisterPlugin(wasm.NewHealthManager())
	k.RegisterPlugin(wasm.NewKVManager(stateStore))
	k.RegisterPlugin(wasm.NewTaskRunner())
	k.RegisterPlugin(wasm.NewCacheManager())
	k.RegisterPlugin(wasm.NewDocStore())
	k.RegisterPlugin(wasm.NewNetworkManager())
	k.RegisterPlugin(wasm.NewStorageManager())

	if err := k.Start(ctx); err != nil {
		logger.Error("failed to start kernel", "error", err)
		os.Exit(1)
	}

	// Start IPC Server
	ipcServer := ipc.NewServer(logger, auditLogger, k, tlsConfig)
	go func() {
		if err := ipcServer.ListenAndServe(*socket); err != nil {
			logger.Error("IPC server stopped", "error", err)
		}
	}()

	// Write Instance Tracking Info
	info := identity.InstanceInfo{
		Name:      *instanceName,
		PID:       os.Getpid(),
		Socket:    *socket,
		StartTime: time.Now(),
	}
	if err := store.WriteInstanceInfo(info, getAlloyRuntimeDir()); err != nil {
		logger.Warn("failed to write instance tracking info", "error", err)
	}

	logger.Info("alloy-core started", "instance", *instanceName, "socket", *socket, "mtls", tlsConfig != nil)

	<-ctx.Done()
	logger.Info("shutting down alloy-core")

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopCancel()

	_ = store.ClearInstanceInfo(*instanceName, getAlloyRuntimeDir())
	ipcServer.Stop()
	if err := k.Stop(stopCtx); err != nil {
		logger.Error("failed to stop kernel gracefully", "error", err)
		os.Exit(1)
	}

	logger.Info("alloy-core stopped")
}
