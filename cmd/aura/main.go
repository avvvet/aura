package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/tui"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file")
	kubecontext := flag.String("context", "", "kubernetes context to use")
	flag.Parse()

	// build k8s client
	c, err := client.New(*kubeconfig, *kubecontext)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to cluster: %v\n", err)
		os.Exit(1)
	}

	// start bubbletea live TUI
	m := tui.New(c)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		log.Fatalf("failed to start aura: %v", err)
	}
}
