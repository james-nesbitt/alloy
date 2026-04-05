package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/ipc"
	"github.com/james-nesbitt/alloy/pkg/security/identity"
)

func getAlloyRuntimeDir() string {
	if run := os.Getenv("XDG_RUNTIME_DIR"); run != "" {
		return filepath.Join(run, "alloy")
	}
	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}
	return filepath.Join(os.TempDir(), "alloy-"+user)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "core":
		launchCore(args)
	case "tui":
		launchFrontend("alloy-tui", args)
	case "gui":
		launchFrontend("alloy-gui", args)
	case "web":
		launchFrontend("alloy-web", args)
	case "version":
		fmt.Println("Alloy Tool v0.1.0")
	case "help":
		usage()
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		usage()
	}
}

func usage() {
	fmt.Println("Alloy Unified Entry Point")
	fmt.Println("\nUsage:")
	fmt.Println("  alloy core [options]          Launch a standalone core backend")
	fmt.Println("  alloy tui [options]           Launch TUI frontend (optionally with dedicated core)")
	fmt.Println("  alloy gui [options]           Launch GUI frontend (optionally with dedicated core)")
	fmt.Println("  alloy web [options]           Launch Web frontend (optionally with dedicated core)")
	fmt.Println("  alloy version                 Show version info")
	fmt.Println("\nCommon Options:")
	fmt.Println("  --data-dir DIR                Plugin data directory (default: ./build/data)")
	fmt.Println("  --provision FILE              Initial provisioning manifest")
	fmt.Println("\nCore Options:")
	fmt.Println("  --listen ADDR                 Listen address (default: unix://./alloy.sock)")
	fmt.Println("\nFrontend Options:")
	fmt.Println("  --socket ADDR                 Connect to existing core at ADDR")
	fmt.Println("  --dedicated                   Launch a dedicated protected core instance for this frontend")
	fmt.Println("  --network                     When using --dedicated, use a TCP network socket instead of Unix")
	fmt.Println("  --insecure                    Disable protection and use plain-text communication")
	fmt.Println("  --security-dir PATH           Path to the security/identity directory (Auto-generated for dedicated sessions)")
	fmt.Println("  --debug                       Enable verbose debug logging for all components")
	os.Exit(1)
}

func findBinary(name string) (string, error) {
	// 1. Check current executable's directory
	exe, err := os.Executable()
	if err == nil {
		path := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		// Look specifically for alloy-core in ../libexec/alloy/ (Standard installation layout)
		if name == "alloy-core" {
			instPath := filepath.Join(filepath.Dir(exe), "..", "libexec", "alloy", name)
			if _, err := os.Stat(instPath); err == nil {
				return instPath, nil
			}
		}
	}

	// 2. Check dev environment relative paths (build/dist/usr/bin)
	cwd, _ := os.Getwd()
	devPaths := []string{
		filepath.Join(cwd, "build", "dist", "usr", "bin", name),
		filepath.Join(cwd, "build", "dist", "usr", "libexec", "alloy", name),
		filepath.Join(cwd, "build", "bin", name), // Old layout flat
	}
	for _, p := range devPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 3. Check system FHS paths
	fhsPaths := []string{
		"/usr/bin/" + name,
		"/usr/local/bin/" + name,
		"/usr/libexec/alloy/" + name,
	}
	for _, p := range fhsPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// 4. Check system PATH
	return exec.LookPath(name)
}

func launchCore(args []string) {
	fs := flag.NewFlagSet("core", flag.ExitOnError)
	listen := fs.String("listen", "unix://./alloy.sock", "Listen address")
	dataDir := fs.String("data-dir", "./build/data", "Data directory")
	provision := fs.String("provision", "", "Provisioning file")
	debug := fs.Bool("debug", false, "Enable debug logging")
	sf := cmdutil.RegisterSecurityFlags(fs)
	fs.Parse(args)

	cmdutil.HandleSecurityError(sf.Validate())

	bin, err := findBinary("alloy-core")
	if err != nil {
		log.Fatalf("Fatal: alloy-core binary not found. %v", err)
	}

	cmdArgs := []string{"--listen", *listen, "--data-dir", *dataDir}
	if *provision != "" {
		cmdArgs = append(cmdArgs, "--provision", *provision)
	}
	if *debug {
		cmdArgs = append(cmdArgs, "--debug")
	}
	if *sf.Insecure {
		cmdArgs = append(cmdArgs, "--insecure")
	}
	if *sf.SecurityDir != "" {
		cmdArgs = append(cmdArgs, "--security-dir", *sf.SecurityDir)
	}

	cmd := exec.Command(bin, cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf(">> Launching Alloy Core: %s %s\n", bin, strings.Join(cmdArgs, " "))
	if err := cmd.Run(); err != nil {
		log.Fatalf("Core exited with error: %v", err)
	}
}

func waitForServer(rawAddr string, timeout time.Duration) error {
	network, addr := ipc.ParseAddress(rawAddr)
	start := time.Now()
	for time.Since(start) < timeout {
		conn, err := net.DialTimeout(network, addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for core to be ready at %s", rawAddr)
}

func launchFrontend(name string, args []string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	socket := fs.String("socket", "", "Connect to existing socket")
	dedicated := fs.Bool("dedicated", false, "Launch dedicated core")
	network := fs.Bool("network", false, "Use network socket for dedicated core")
	debug := fs.Bool("debug", false, "Enable debug logging")
	provision := fs.String("provision", "", "Initial provisioning manifest for dedicated core")
	dataDir := fs.String("data-dir", "./build/data", "Data directory for dedicated core")
	sf := cmdutil.RegisterSecurityFlags(fs)
	fs.Parse(args)

	cmdutil.HandleSecurityError(sf.Validate())

	bin, err := findBinary(name)
	if err != nil {
		log.Fatalf("Fatal: %s binary not found. %v", name, err)
	}

	targetSocket := *socket
	var coreCmd *exec.Cmd

	forceInsecure := *sf.Insecure
	sessionSecurityDir := *sf.SecurityDir

	if *dedicated || targetSocket == "" {
		// Launch a dedicated core
		coreBin, err := findBinary("alloy-core")
		if err != nil {
			log.Fatalf("Fatal: alloy-core binary not found for dedicated mode. %v", err)
		}

		runtimeDir := getAlloyRuntimeDir()
		os.MkdirAll(runtimeDir, 0700)

		addr := "unix://" + filepath.Join(runtimeDir, fmt.Sprintf("dedicated-%d.sock", os.Getpid()))
		if *network {
			addr = "tcp://127.0.0.1:9091"
		}
		targetSocket = addr

		coreArgs := []string{"--listen", addr, "--data-dir", *dataDir}
		if *provision != "" {
			coreArgs = append(coreArgs, "--provision", *provision)
		}
		if *debug {
			coreArgs = append(coreArgs, "--debug")
		}

		// Handle automated security for dedicated sessions
		if !forceInsecure && sessionSecurityDir == "" {
			sessionSecurityDir = filepath.Join(runtimeDir, fmt.Sprintf("session-%d", os.Getpid()))
			fmt.Printf(">> Provisioning ephemeral credentials in %s\n", sessionSecurityDir)

			store := identity.NewStore(sessionSecurityDir)
			ca, err := store.InitializeMachine()
			if err != nil {
				log.Fatalf("Failed to initialize ephemeral CA: %v", err)
			}
			// Pre-generate instance and client identities to ensure they're ready
			_, _ = store.CreateInstanceIdentity(ca, "core")
			// No need to create a specific client one here yet, as the frontend will do it via the same store dir.
		}

		if forceInsecure {
			coreArgs = append(coreArgs, "--insecure")
		} else if sessionSecurityDir != "" {
			coreArgs = append(coreArgs, "--security-dir", sessionSecurityDir)
		}

		coreCmd = exec.Command(coreBin, coreArgs...)
		// Propagate logs to stderr only so we don't mess up TUI/GUI output
		coreCmd.Stderr = os.Stderr

		fmt.Printf(">> Starting dedicated core on %s...\n", addr)
		if err := coreCmd.Start(); err != nil {
			log.Fatalf("Failed to start dedicated core: %v", err)
		}

		// Wait for socket to be ready (robustly)
		if err := waitForServer(addr, 5*time.Second); err != nil {
			coreCmd.Process.Kill()
			log.Fatalf("Dedicated core failed to start: %v", err)
		}
	}

	feArgs := []string{"--socket", targetSocket}
	if forceInsecure {
		feArgs = append(feArgs, "--insecure")
	}
	if sessionSecurityDir != "" {
		feArgs = append(feArgs, "--security-dir", sessionSecurityDir)
	}
	if *debug {
		feArgs = append(feArgs, "--debug")
	}

	// Any remaining arguments after -- are passed to the frontend
	feArgs = append(feArgs, fs.Args()...)

	cmd := exec.Command(bin, feArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf(">> Launching %s connecting to %s\n", name, targetSocket)

	// Clean up core if it was dedicated
	if coreCmd != nil {
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			<-sigCh
			coreCmd.Process.Signal(os.Interrupt)
		}()
	}

	if err := cmd.Run(); err != nil {
		if coreCmd != nil {
			coreCmd.Process.Kill()
		}
		// Cleanup session security dir if we created one
		if !forceInsecure && *sf.SecurityDir == "" && sessionSecurityDir != "" {
			os.RemoveAll(sessionSecurityDir)
		}
		log.Fatalf("%s exited with error: %v", name, err)
	}

	if coreCmd != nil {
		coreCmd.Process.Signal(os.Interrupt)
		coreCmd.Wait()
	}

	// Final cleanup of session security dir
	if !forceInsecure && *sf.SecurityDir == "" && sessionSecurityDir != "" {
		os.RemoveAll(sessionSecurityDir)
	}
}
