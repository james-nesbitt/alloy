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

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/ipc"
	"github.com/james-nesbitt/alloy/pkg/kernel"
	"github.com/james-nesbitt/alloy/pkg/plugins/native"
	"github.com/james-nesbitt/alloy/pkg/security/identity"
	"github.com/james-nesbitt/alloy/pkg/storage"
)

func getAlloyRuntimeDir() string {
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "alloy")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	// Fallback to /tmp if XDG_RUNTIME_DIR is not available
	return filepath.Join(os.TempDir(), "alloy-"+user)
}

func main() {
	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Parse command line flags
	dataDir := flag.String("data-dir", "./data", "Directory for plugin data")
	listenAddr := flag.String("listen", "unix://"+filepath.Join(getAlloyRuntimeDir(), "default.sock"), "Address to listen on (unix:// or tcp://)")
	metricsAddr := flag.String("metrics-addr", ":9090", "Address for Prometheus metrics")
	debug := flag.Bool("debug", false, "Enable debug logging")
	provisionFile := flag.String("provision", "", "Provisioning file for initial plugins")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())

	// Adjust logging level
	if *debug {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}

	// Disable metrics for core tests if requested via env
	if os.Getenv("ALLOY_TEST_MODE") == "true" {
		*metricsAddr = ""
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
	k, err := kernel.NewWITKernel(logger, kv, *dataDir, *metricsAddr)
	if err != nil {
		logger.Error("failed to create WIT kernel", "error", err)
		os.Exit(1)
	}

	// Create context for kernel shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up security/identity
	var tlsConfig *tls.Config
	if !*sf.Insecure {
		secDir := *sf.SecurityDir
		if secDir == "" {
			if home := os.Getenv("XDG_CONFIG_HOME"); home != "" {
				secDir = filepath.Join(home, "alloy")
			} else {
				secDir = filepath.Join(os.Getenv("HOME"), ".config", "alloy")
			}
		}
		store := identity.NewStore(secDir)
		ca, err := store.InitializeMachine()
		if err == nil {
			id, err := store.CreateInstanceIdentity(ca, "core")
			if err == nil {
				tlsConfig, _ = store.GetServerTLSConfig(ca, id)
			}
		}
		if tlsConfig != nil {
			logger.Info("mTLS security enabled", "dir", secDir)
		} else {
			logger.Warn("mTLS initialization failed, falling back to insecure", "dir", secDir)
		}
	} else {
		logger.Warn("running in INSECURE mode")
	}

	// Create and start IPC server BEFORE loading plugins so tests can connect while loading
	server := ipc.NewServer(logger, k, tlsConfig)

	go func() {
		logger.Info("starting IPC server", "addr", *listenAddr)
		if err := server.ListenAndServe(*listenAddr); err != nil {
			logger.Error("IPC server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Resolve provisioning file
	currentProvisionFile := *provisionFile
	if currentProvisionFile == "" {
		// 1. Try relative to the executable (FHS relative: from /usr/libexec/alloy to /etc/alloy)
		if exe, err := os.Executable(); err == nil {
			fhsEtc := filepath.Join(filepath.Dir(exe), "..", "..", "etc", "alloy", "provision.json")
			if _, err := os.Stat(fhsEtc); err == nil {
				currentProvisionFile = fhsEtc
			}
		}
		// 2. Try global absolute FHS
		if currentProvisionFile == "" {
			if _, err := os.Stat("/etc/alloy/provision.json"); err == nil {
				currentProvisionFile = "/etc/alloy/provision.json"
			}
		}
		// 3. Try local CWD (development fallback)
		if currentProvisionFile == "" {
			if _, err := os.Stat("provision.json"); err == nil {
				currentProvisionFile = "provision.json"
			}
		}
	}

	// Load provisioned plugins if found
	if currentProvisionFile != "" {
		if err := loadProvisionedPlugins(k, currentProvisionFile, logger, kv); err != nil {
			logger.Error("failed to load provisioned plugins", "error", err)
			os.Exit(1)
		}
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Create health monitor
	go healthMonitor(k, logger)

	logger.Info("Alloy Core started", "data_dir", *dataDir)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("shutting down")

	server.Stop()
	if err := k.Shutdown(ctx); err != nil {
		logger.Error("kernel shutdown error", "error", err)
	}
}

// ProvisionManifest defines the structure of the plugin provisioning file.
type ProvisionManifest struct {
	Plugins []ProvisionPlugin `json:"plugins"`
}

type ProvisionPlugin struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Path         string           `json:"path"`
	LoadTime     string           `json:"load_time"`
	MaxMemoryMB  uint32           `json:"max_memory_mb"`
	MsgPerSecond int              `json:"msg_per_second"`
	Capabilities []api.Capability `json:"capabilities"`
}

// loadProvisionedPlugins loads plugins from a provisioning file.
func loadProvisionedPlugins(k *kernel.WITKernel, provisionFile string, logger *slog.Logger, kv storage.StateStore) error {
	logger.Info("loading provisioned plugins", "file", provisionFile)
	data, err := os.ReadFile(provisionFile)
	if err != nil {
		return fmt.Errorf("failed to read provision file: %w", err)
	}

	var manifest ProvisionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse provision file: %w", err)
	}

	for i, p := range manifest.Plugins {
		logger.Debug("processing provisioned plugin", "index", i, "id", p.ID, "type", p.Type)
		if p.Type == "native" {
			// Register native plugin from the registry
			constructor, ok := native.Registry[p.ID]
			if !ok {
				logger.Error("unsupported native plugin id", "id", p.ID)
				continue
			}

			pluginAny, err := constructor(context.Background(), logger, kv)
			if err != nil {
				logger.Error("failed to construct native plugin", "id", p.ID, "error", err)
				continue
			}

			if plugin, ok := pluginAny.(api.Plugin); ok {
				k.RegisterPlugin(plugin)
				logger.Info("registered native plugin", "id", p.ID)
			} else {
				logger.Error("native plugin does not implement api.Plugin", "id", p.ID)
			}
			continue
		}

		if p.Type != "wasm" {
			logger.Warn("unsupported plugin type in manifest", "id", p.ID, "type", p.Type)
			continue
		}

		// Resolve the plugin path (warn but don't fail for lazy)
		wasmPath := resolvePluginPath(provisionFile, p.Path)
		if wasmPath == "" {
			if p.LoadTime != "lazy" {
				logger.Error("could not resolve plugin WASM path", "id", p.ID, "requested", p.Path)
				continue
			}
			// Use the requested path as is for lazy loading
			wasmPath = p.Path
			logger.Warn("could not resolve lazy plugin WASM path, using as is", "id", p.ID, "requested", p.Path)
		}

		if p.LoadTime == "lazy" {
			// Register a lazy loader for this plugin
			k.RegisterPluginLoader(p.ID, &wasmLoader{
				k:            k,
				pluginID:     p.ID,
				path:         wasmPath,
				logger:       logger,
				maxMemoryMB:  p.MaxMemoryMB,
				msgPerSecond: p.MsgPerSecond,
				capabilities: p.Capabilities,
			}, api.PluginMetadata{
				ID:           p.ID,
				LoadTime:     api.LoadTimeLazy,
				Capabilities: p.Capabilities,
			})
			logger.Info("registered lazy-loaded plugin", "id", p.ID, "path", wasmPath)
		} else {
			// Load immediately on boot
			wasmBytes, err := os.ReadFile(wasmPath)
			if err != nil {
				logger.Error("failed to read plugin WASM for boot-load", "id", p.ID, "path", wasmPath, "error", err)
				continue
			}

			// Defaults if not provided in manifest
			if p.MaxMemoryMB == 0 {
				p.MaxMemoryMB = 128
			}
			if p.MsgPerSecond == 0 {
				p.MsgPerSecond = 1000
			}

			if err := k.RegisterWASMPluginAtScale(p.ID, wasmBytes, p.MaxMemoryMB, p.MsgPerSecond, p.Capabilities); err != nil {
				logger.Error("failed to register boot-loaded plugin", "id", p.ID, "error", err)
				continue
			}
			logger.Info("loaded plugin on boot", "id", p.ID, "path", wasmPath)
		}
	}

	return nil
}

// resolvePluginPath attempts to find the WASM file relative to several well-known locations.
func resolvePluginPath(manifestPath, pluginPath string) string {
	if filepath.IsAbs(pluginPath) {
		if _, err := os.Stat(pluginPath); err == nil {
			return pluginPath
		}
	}

	// 1. Try relative to the manifest file itself
	relToManifest := filepath.Join(filepath.Dir(manifestPath), pluginPath)
	if _, err := os.Stat(relToManifest); err == nil {
		return relToManifest
	}

	// 2. Try the official FHS location relative to the binary
	if exe, err := os.Executable(); err == nil {
		fhsPath := filepath.Join(filepath.Dir(exe), "..", "lib", "alloy", "plugins", pluginPath)
		if _, err := os.Stat(fhsPath); err == nil {
			return fhsPath
		}
	}

	// 3. Try common dev paths (relative to CWD)
	cwd, _ := os.Getwd()
	devPaths := []string{
		filepath.Join(cwd, "build", "dist", "usr", "lib", "alloy", "plugins", pluginPath),
		filepath.Join(cwd, "build", "plugins", pluginPath),
	}
	for _, p := range devPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// 4. Try system-wide location
	sysPath := filepath.Join("/usr/lib/alloy/plugins", pluginPath)
	if _, err := os.Stat(sysPath); err == nil {
		return sysPath
	}

	return ""
}

// wasmLoader implements the api.PluginLoader interface for lazy-loading WASM plugins.
type wasmLoader struct {
	k            *kernel.WITKernel
	pluginID     string
	path         string
	logger       *slog.Logger
	maxMemoryMB  uint32
	msgPerSecond int
	capabilities []api.Capability
}

func (l *wasmLoader) LoadPlugin(ctx context.Context, id string) (api.Plugin, error) {
	l.logger.Info("lazy-loading plugin", "id", id, "path", l.path)

	wasmBytes, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lazy-loaded WASM: %w", err)
	}

	// Defaults if not provided in manifest
	if l.maxMemoryMB == 0 {
		l.maxMemoryMB = 128
	}
	if l.msgPerSecond == 0 {
		l.msgPerSecond = 1000
	}

	if err := l.k.RegisterWASMPluginAtScale(id, wasmBytes, l.maxMemoryMB, l.msgPerSecond, l.capabilities); err != nil {
		return nil, fmt.Errorf("failed to register lazy-loaded WASM: %w", err)
	}

	p, ok := l.k.GetPlugin(id)
	if !ok {
		return nil, fmt.Errorf("plugin %s registered but could not be retrieved", id)
	}

	return p, nil
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
