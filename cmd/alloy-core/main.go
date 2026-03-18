package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	"github.com/jnesbitt/alloy-go/pkg/storage"
	"github.com/jnesbitt/alloy-go/pkg/plugins/native"
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

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	debug := flag.Bool("debug", false, "Enable debug logging")
	insecure := flag.Bool("insecure", false, "Disable mTLS")
	alloyHome := flag.String("home", getAlloyHome(), "Directory for alloy config and identities")
	instanceName := flag.String("name", "default", "Instance name")

	defaultSocket := filepath.Join(getAlloyRuntimeDir(), "default.sock")
	socket := flag.String("socket", defaultSocket, "Socket address to listen for IPC connections (supports tcp://, unix://, or local path)")

	var wasmPluginsDirs stringSlice
	flag.Var(&wasmPluginsDirs, "wasm-plugins", "Directory to scan for WASM plugins to load at startup (can be specified multiple times)")

	var wasmPlugins stringSlice
	flag.Var(&wasmPlugins, "wasm-plugin", "WASM plugin file to load at startup (can be specified multiple times)")

	provisionManifest := flag.String("provision", "", "Path to provisioning manifest (JSON)")

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
	stateStore, err := storage.NewFileStateStore(filepath.Join(getAlloyDataDir(), "state"))
	if err != nil {
		logger.Error("failed to initialize state store", "error", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k := kernel.New(logger)

	// WASM Runtime Setup
	wasmRuntime, err := wasm.NewRuntime(context.Background(), logger, stateStore)
	if err != nil {
		logger.Error("failed to initialize wasm runtime", "error", err)
		os.Exit(1)
	}

	// Instantiate the host module (alloy) so plugins can import log, kv, etc.
	if _, err := wasmRuntime.InstantiateAlloyHost(context.Background()); err != nil {
		logger.Error("failed to instantiate alloy host module", "error", err)
		os.Exit(1)
	}

	// Registry Manager (Orchestrator)
	rm := native.NewRegistryManager(logger, k, stateStore, wasmRuntime)
	k.RegisterPlugin(rm)

	// Plugin Discovery (if --wasm-plugins flag(s) set)
	for _, dir := range wasmPluginsDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			logger.Error("failed to scan WASM plugins directory", "path", dir, "error", err)
			continue
		}
		for _, file := range files {
			if !file.IsDir() && filepath.Ext(file.Name()) == ".wasm" {
				pluginID := "plugin-" + file.Name()[:len(file.Name())-5]
				pluginPath := filepath.Join(dir, file.Name())
				logger.Info("discovered WASM plugin", "id", pluginID, "path", pluginPath)

				pluginDef := map[string]any{
					"id":   pluginID,
					"type": "wasm",
					"path": pluginPath,
				}
				payload, _ := json.Marshal(pluginDef)

				k.RouteMessage(context.Background(), api.Message{
					ID:      "auto-load-" + pluginID,
					Type:    api.TypeRequest,
					Sender:  "system",
					Target:  rm.ID(),
					Method:  "load",
					Payload: payload,
				})
			}
		}
	}

	// Manual Loading (if --wasm-plugin flag(s) set)
	for _, pluginPath := range wasmPlugins {
		pluginID := "plugin-" + filepath.Base(pluginPath)
		if len(pluginID) > 5 && pluginID[len(pluginID)-5:] == ".wasm" {
			pluginID = pluginID[:len(pluginID)-5]
		}
		logger.Info("loading manual WASM plugin", "id", pluginID, "path", pluginPath)

		pluginDef := map[string]any{
			"id":   pluginID,
			"type": "wasm",
			"path": pluginPath,
		}
		payload, _ := json.Marshal(pluginDef)

		k.RouteMessage(context.Background(), api.Message{
			ID:      "manual-load-" + pluginID,
			Type:    api.TypeRequest,
			Sender:  "system",
			Target:  rm.ID(),
			Method:  "load",
			Payload: payload,
		})
	}

	// Provisioning (if manifest path provided)
	if *provisionManifest != "" {
		logger.Info("loading provisioning manifest", "path", *provisionManifest)
		data, err := os.ReadFile(*provisionManifest)
		if err != nil {
			logger.Error("failed to read provisioning manifest", "path", *provisionManifest, "error", err)
		} else {
			k.RouteMessage(context.Background(), api.Message{
				ID:      "bootstrap-provision",
				Type:    api.TypeRequest,
				Sender:  "system",
				Target:  rm.ID(),
				Method:  "provision",
				Payload: data,
			})
		}
	} else {
		logger.Info("no provisioning manifest provided, starting in minimal mode")
	}

	if err := k.Start(ctx); err != nil {
		logger.Error("failed to start kernel", "error", err)
		os.Exit(1)
	}

	// Start IPC Server
	ipcServer := ipc.NewServer(logger, k, tlsConfig)
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

	select {
	case <-ctx.Done():
	case <-k.StopCh():
		logger.Info("received shutdown signal from kernel")
	}

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
