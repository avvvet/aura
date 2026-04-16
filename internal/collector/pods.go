package collector

import (
	"context"
	"fmt"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodCollector collects pod information from the cluster
type PodCollector struct {
	client *client.Client
}

// NewPodCollector creates a new PodCollector
func NewPodCollector(c *client.Client) *PodCollector {
	return &PodCollector{client: c}
}

// Collect gathers all pod data and fills snapshot.Pods
func (p *PodCollector) Collect(ctx context.Context, snapshot *model.ClusterSnapshot) error {
	pods, err := p.client.Kubernetes.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to collect pods: %w", err)
	}

	for _, pod := range pods.Items {
		// count restarts across all containers
		var restarts int32
		var ready int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
			if cs.Ready {
				ready++
			}
		}

		totalContainers := int32(len(pod.Spec.Containers))

		// get resource requests and limits from first container
		cpuRequest := ""
		memRequest := ""
		cpuLimit := ""
		memLimit := ""

		if len(pod.Spec.Containers) > 0 {
			c := pod.Spec.Containers[0]
			if req := c.Resources.Requests; req != nil {
				if cpu := req.Cpu(); cpu != nil {
					cpuRequest = cpu.String()
				}
				if mem := req.Memory(); mem != nil {
					memRequest = mem.String()
				}
			}
			if lim := c.Resources.Limits; lim != nil {
				if cpu := lim.Cpu(); cpu != nil {
					cpuLimit = cpu.String()
				}
				if mem := lim.Memory(); mem != nil {
					memLimit = mem.String()
				}
			}
		}

		snapshot.Pods = append(snapshot.Pods, model.Pod{
			Name:          pod.Name,
			Namespace:     pod.Namespace,
			Status:        string(pod.Status.Phase),
			Ready:         fmt.Sprintf("%d/%d", ready, totalContainers),
			Restarts:      restarts,
			Age:           age(pod.CreationTimestamp.Time),
			Node:          pod.Spec.NodeName,
			CPURequest:    cpuRequest,
			MemoryRequest: memRequest,
			CPULimit:      cpuLimit,
			MemoryLimit:   memLimit,
		})
	}

	return nil
}
