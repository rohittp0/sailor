package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"sailor/internal/config"
	"sailor/internal/do"
	"sailor/internal/ui"
)

func main() {
	tok, err := config.Token()
	if errors.Is(err, config.ErrNoToken) {
		fmt.Fprintf(os.Stderr,
			"No DigitalOcean API token found.\nSet %s or run `doctl auth init`.\n", config.EnvVar)
		os.Exit(1)
	} else if err != nil {
		fmt.Fprintln(os.Stderr, "token error:", err)
		os.Exit(1)
	}

	client := do.NewClient(tok)

	if len(os.Args) > 1 && os.Args[1] == "--probe" {
		runProbe(client)
		return
	}

	hosts, err := config.LoadHosts()
	if err != nil {
		fmt.Fprintln(os.Stderr, "loading SSH profiles:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(ui.New(client, hosts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sailor:", err)
		os.Exit(1)
	}
}
