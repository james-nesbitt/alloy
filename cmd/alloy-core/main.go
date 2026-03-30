package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/ipc"
	"github.com/james-nesbitt/alloy/pkg/kernel"
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
	projectManifest := flag.String("manifest", "alloy-project.json", "Project manifest file")
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

	// Create kernel
	k, err := kernel.New(logger, kv, *dataDir, *metricsAddr)
	if err != nil {
		logger.Error("failed to create Alloy kernel", "error", err)
		os.Exit(1)
	}
	// Insecure flag controls RBAC enforcement
	k.SetInsecure(*sf.Insecure)

	// Create context for kernel shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up security/identity (mTLS)
	var tlsConfig *tls.Config
	if !*sf.NoMTLS {
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
			logger.Warn("mTLS initialization failed, falling back to plain communication", "dir", secDir)
		}
	} else {
		logger.Warn("mTLS disabled - using plain communication")
	}

	if *sf.Insecure {
		logger.Warn("RBAC enforcement disabled")
	}

	// Create and start IPC server BEFORE loading plugins so tests can connect while loading
	server := ipc.NewServer(logger, k, tlsConfig)

	// Step: Apply Project Manifest if found
	if *projectManifest != "" {
		if _, err := os.Stat(*projectManifest); err == nil {
			manifest, err := kernel.LoadManifest(*projectManifest)
			if err != nil {
				logger.Error("failed to load project manifest", "error", err)
			} else {
				if err := k.ApplyManifest(ctx, manifest); err != nil {
					logger.Error("failed to apply project manifest", "error", err)
				}
			}
		}
	}

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
		data, err := os.ReadFile(currentProvisionFile)
		if err != nil {
			logger.Error("failed to read provision file", "error", err)
			os.Exit(1)
		}

		var manifest struct {
			Plugins []kernel.PluginDef `json:"plugins"`
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			logger.Error("failed to parse provision file", "error", err)
			os.Exit(1)
		}

		// Resolve paths relative to manifest
		for i := range manifest.Plugins {
			if manifest.Plugins[i].Type == "wasm" {
				manifest.Plugins[i].Path = kernel.ResolvePluginPath(currentProvisionFile, manifest.Plugins[i].Path)
			}
		}

		if err := k.Provision(manifest.Plugins); err != nil {
			logger.Error("failed to provision plugins", "error", err)
			os.Exit(1)
		}
	}

	// Set up signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Alloy Core started", "data_dir", *dataDir)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("shutting down")

	server.Stop()
	if err := k.Shutdown(ctx); err != nil {
		logger.Error("kernel shutdown error", "error", err)
	}
}
