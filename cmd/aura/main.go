package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/collector"
	"github.com/avvvet/aura/internal/model"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file")
	kubecontext := flag.String("context", "", "kubernetes context to use")
	flag.Parse()

	// build k8s client
	c, err := client.New(*kubeconfig, *kubecontext)
	if err != nil {
		log.Fatalf("failed to connect to cluster: %v", err)
	}

	fmt.Printf("connected to cluster: %s (context: %s)\n", c.ClusterName, c.Context)

	// build snapshot
	snapshot := &model.ClusterSnapshot{
		CapturedAt:  time.Now(),
		ClusterName: c.ClusterName,
		Context:     c.Context,
	}

	ctx := context.Background()

	// run collectors
	collectors := []collector.Collector{
		collector.NewNodeCollector(c),
		collector.NewNamespaceCollector(c),
	}

	for _, col := range collectors {
		if err := col.Collect(ctx, snapshot); err != nil {
			log.Printf("collector error: %v", err)
		}
	}

	// print raw results to confirm
	fmt.Printf("\nNodes (%d):\n", len(snapshot.Nodes))
	for _, n := range snapshot.Nodes {
		fmt.Printf("  %s\t%s\t%s\t%s\t%s\n", n.Name, n.Status, n.Roles, n.Version, n.Age)
	}

	fmt.Printf("\nNamespaces (%d):\n", len(snapshot.Namespaces))
	for _, ns := range snapshot.Namespaces {
		fmt.Printf("  %s\t%s\t%s\n", ns.Name, ns.Status, ns.Age)
	}
}
