package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jnesbitt/alloy-go/api"
	"github.com/jnesbitt/alloy-go/pkg/ipc"
	"github.com/jnesbitt/alloy-go/pkg/security/identity"
)

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
	if len(os.Args) < 2 {
		usage()
	}

	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Println("Alloy CLI v0.0.1")
	case "ping":
		ping(os.Args[2:])
	case "discover":
		discover(os.Args[2:])
	case "list":
		list(os.Args[2:])
	case "stop":
		stop(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", cmd)
		usage()
	}
}

func usage() {
	fmt.Println("Alloy CLI")
	fmt.Println("Usage: alloy-cli <command> [options]")
	fmt.Println("\nCommands:")
	fmt.Println("  version          Show version info")
	fmt.Println("  ping             Ping the alloy-core")
	fmt.Println("  discover         Discover active targets in an instance")
	fmt.Println("  list             List running alloy-core instances")
	fmt.Println("  stop             Stop a running alloy-core instance")
	os.Exit(1)
}

func stop(args []string) {
	fs := flag.NewFlagSet("stop", flag.ExitOnError)
	name := fs.String("name", "default", "Name of the instance to stop")
	fs.Parse(args)

	store := identity.NewStore(getAlloyHome())
	instances, err := store.ListInstances(getAlloyRuntimeDir())
	if err != nil {
		fmt.Printf("Failed to list instances: %v\n", err)
		os.Exit(1)
	}

	for _, inst := range instances {
		if inst.Name == *name {
			process, err := os.FindProcess(inst.PID)
			if err != nil {
				fmt.Printf("Failed to find process %d: %v\n", inst.PID, err)
				os.Exit(1)
			}
			fmt.Printf("Stopping instance %s (PID %d)...\n", *name, inst.PID)
			if err := process.Signal(os.Interrupt); err != nil {
				fmt.Printf("Failed to stop process: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	fmt.Printf("Instance %s not found.\n", *name)
}

func discover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	actor := fs.String("actor", "cli-discover", "Actor identity")
	socket := fs.String("socket", filepath.Join(getAlloyRuntimeDir(), "default.sock"), "Socket address of the core")
	alloyHome := fs.String("home", getAlloyHome(), "Directory for alloy identities")
	insecure := fs.Bool("insecure", false, "Disable mTLS")
	fs.Parse(args)

	var tlsConfig *tls.Config
	if !*insecure {
		store := identity.NewStore(*alloyHome)
		ca, _ := store.InitializeMachine()
		tlsConfig, _ = store.GetClientTLSConfig(ca, "cli-discover")
	}

	client, err := ipc.Dial(*socket, tlsConfig)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	msg := api.Message{
		ID:        "discover-1",
		Type:      api.TypeRequest,
		Sender:    "cli-discover",
		Actor:     *actor,
		Target:    "plugin-command-manager",
		Method:    "discover",
		Timestamp: time.Now().Unix(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Call(ctx, msg)
	if err != nil {
		fmt.Printf("Discovery failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Active Targets:")
	fmt.Printf("%-20s %-10s %-40s\n", "ID", "TYPE", "CAPABILITIES")

	var data struct {
		Targets []struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			Capabilities []struct {
				Method      string `json:"method"`
				Description string `json:"description"`
			} `json:"capabilities"`
		} `json:"targets"`
	}

	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		fmt.Printf("Failed to parse response: %v\n", err)
		fmt.Printf("Raw: %s\n", string(resp.Payload))
		os.Exit(1)
	}

	for _, t := range data.Targets {
		caps := ""
		for i, c := range t.Capabilities {
			if i > 0 {
				caps += ", "
			}
			caps += c.Method
		}
		fmt.Printf("%-20s %-10s %-40s\n", t.ID, t.Type, caps)
	}
}

func list(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.Parse(args)

	store := identity.NewStore(getAlloyHome())
	instances, err := store.ListInstances(getAlloyRuntimeDir())
	if err != nil {
		fmt.Printf("Failed to list instances: %v\n", err)
		os.Exit(1)
	}

	if len(instances) == 0 {
		fmt.Println("No running instances found.")
		return
	}

	fmt.Printf("%-20s %-10s %-30s %-20s\n", "NAME", "PID", "SOCKET", "STARTED")
	for _, inst := range instances {
		fmt.Printf("%-20s %-10d %-30s %-20s\n",
			inst.Name, inst.PID, inst.Socket, inst.StartTime.Format(time.Kitchen))
	}
}

func ping(args []string) {
	fs := flag.NewFlagSet("ping", flag.ExitOnError)
	name := fs.String("name", "cli-client", "Name of the component to use")
	actor := fs.String("actor", "", "Actor identity (defaults to name)")
	target := fs.String("target", "kernel", "Target component")
	method := fs.String("method", "ping", "Method to call")

	defaultSocket := filepath.Join(getAlloyRuntimeDir(), "default.sock")
	socket := fs.String("socket", defaultSocket, "Socket address of the core")
	alloyHome := fs.String("home", getAlloyHome(), "Directory for alloy identities")

	timeoutSec := fs.Int("timeout", 5, "Timeout in seconds")
	insecure := fs.Bool("insecure", false, "Disable mTLS")
	fs.Parse(args)

	if *actor == "" {
		*actor = *name
	}

	var tlsConfig *tls.Config
	var err error
	if !*insecure {
		store := identity.NewStore(*alloyHome)
		ca, err := store.InitializeMachine()
		if err != nil {
			fmt.Printf("Failed to load machine pki: %v\n", err)
			os.Exit(1)
		}
		tlsConfig, err = store.GetClientTLSConfig(ca, *name)
		if err != nil {
			fmt.Printf("Failed to create client tls config: %v\n", err)
			os.Exit(1)
		}
	}

	client, err := ipc.Dial(*socket, tlsConfig)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	msg := api.Message{
		ID:        "ping-1",
		Type:      api.TypeRequest,
		Sender:    *name,
		Actor:     *actor,
		Target:    *target,
		Method:    *method,
		Timestamp: time.Now().Unix(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
	defer cancel()

	resp, err := client.Call(ctx, msg)
	if err != nil {
		fmt.Printf("Call failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Received response: %s\n", string(resp.Payload))
}
