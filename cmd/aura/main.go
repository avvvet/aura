package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/collector"
	"github.com/avvvet/aura/internal/model"
	"github.com/avvvet/aura/internal/renderer"
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

	// render
	r := renderer.NewTableRenderer()
	if err := r.Render(snapshot); err != nil {
		log.Fatalf("render error: %v", err)
	}
}
