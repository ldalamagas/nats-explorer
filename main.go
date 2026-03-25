package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	nc "github.com/lefteris/nats-explorer/internal/nats"
	"github.com/lefteris/nats-explorer/internal/tui"
)

func main() {
	url := flag.String("url", "", "NATS server URL (default: $NATS_URL or nats://localhost:4222)")
	creds := flag.String("creds", "", "Path to NATS credentials file")
	nkey := flag.String("nkey", "", "Path to NKey seed file")
	flag.Parse()

	natsURL := *url
	if natsURL == "" {
		natsURL = os.Getenv("NATS_URL")
	}
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	connectOpts := nc.BuildConnectOptions(*creds, *nkey)
	client, err := nc.Connect(natsURL, connectOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to NATS: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	app := tui.NewApp(client)
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
