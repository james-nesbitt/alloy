package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/cmdutil"
	"github.com/james-nesbitt/alloy/pkg/frontend"
)

func main() {
	name := flag.String("name", "alloy-tui", "Name of the TUI component")
	actor := flag.String("actor", "", "Actor identity (defaults to name)")
	socket := flag.String("socket", "", "Socket address (defaults to path in runtime dir)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	sf := cmdutil.RegisterSecurityFlags(flag.CommandLine)
	flag.Parse()

	cmdutil.HandleSecurityError(sf.Validate())
	logger := cmdutil.SetupLogger(*debug)

	client, err := cmdutil.InitClient(*name, *actor, *socket, sf)
	if err != nil {
		fmt.Printf("Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	msgCh := make(chan api.Message, 100)
	client.OnMessage(func(msg api.Message) {
		logger.Debug("received message", "sender", msg.Sender, "method", msg.Method)
		msgCh <- msg
	})

	m := NewModel(client, msgCh)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
