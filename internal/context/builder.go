package context

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/avvvet/aura/internal/client"
	"github.com/avvvet/aura/internal/model"
)

// IssueContext holds rich context for a specific issue
type IssueContext struct {
	// the issue itself
	ResourceName      string
	ResourceNamespace string
	ResourceKind      string
	IssueTitle        string
	IssueSeverity     string

	// enriched context
	Events    []string
	Logs      []string
	NodeState string

	// cluster context
	ClusterName string
	Context     string
}

// Builder fetches rich context per issue from the cluster
type Builder struct {
	client *client.Client
}

// New creates a new context Builder
func New(c *client.Client) *Builder {
	return &Builder{client: c}
}

// Build fetches rich context for a specific issue
func (b *Builder) Build(ctx context.Context, snapshot *model.ClusterSnapshot, resourceName, namespace, kind string) (*IssueContext, error) {
	ic := &IssueContext{
		ResourceName:      resourceName,
		ResourceNamespace: namespace,
		ResourceKind:      kind,
		ClusterName:       snapshot.ClusterName,
		Context:           snapshot.Context,
	}

	switch strings.ToLower(kind) {
	case "pod":
		b.enrichPodContext(ctx, ic)
	case "deployment":
		b.enrichDeploymentContext(ctx, ic, snapshot)
	case "node":
		b.enrichNodeContext(ctx, ic)
	default:
		b.enrichGenericContext(ctx, ic)
	}

	return ic, nil
}

// enrichPodContext fetches events and logs for a pod
func (b *Builder) enrichPodContext(ctx context.Context, ic *IssueContext) {
	// fetch events
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		for _, e := range events.Items {
			if e.Type == "Warning" {
				ic.Events = append(ic.Events,
					fmt.Sprintf("[%s] %s: %s", e.Type, e.Reason, e.Message),
				)
			}
		}
	}

	// fetch last 30 lines of logs
	tailLines := int64(30)
	logs, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		GetLogs(ic.ResourceName, &corev1.PodLogOptions{
			TailLines: &tailLines,
		}).
		DoRaw(ctx)
	if err == nil && len(logs) > 0 {
		lines := strings.Split(string(logs), "\n")
		for _, l := range lines {
			if l != "" {
				ic.Logs = append(ic.Logs, l)
			}
		}
	}

	// fetch previous container logs if pod restarted
	prevLogs, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		GetLogs(ic.ResourceName, &corev1.PodLogOptions{
			TailLines: &tailLines,
			Previous:  true,
		}).
		DoRaw(ctx)
	if err == nil && len(prevLogs) > 0 {
		ic.Logs = append(ic.Logs, "--- previous container logs ---")
		lines := strings.Split(string(prevLogs), "\n")
		for _, l := range lines {
			if l != "" {
				ic.Logs = append(ic.Logs, l)
			}
		}
	}

	// get node state the pod is on
	pod, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err == nil && pod.Spec.NodeName != "" {
		node, err := b.client.Kubernetes.CoreV1().
			Nodes().
			Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
		if err == nil {
			var conditions []string
			for _, c := range node.Status.Conditions {
				conditions = append(conditions,
					fmt.Sprintf("%s=%s", c.Type, c.Status),
				)
			}
			ic.NodeState = fmt.Sprintf("node: %s conditions: %s cpu: %s memory: %s",
				pod.Spec.NodeName,
				strings.Join(conditions, ", "),
				node.Status.Capacity.Cpu().String(),
				node.Status.Capacity.Memory().String(),
			)
		}
	}
}

// enrichDeploymentContext fetches events for a deployment and its pods
func (b *Builder) enrichDeploymentContext(ctx context.Context, ic *IssueContext, snapshot *model.ClusterSnapshot) {
	// fetch deployment events
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		for _, e := range events.Items {
			ic.Events = append(ic.Events,
				fmt.Sprintf("[%s] %s: %s", e.Type, e.Reason, e.Message),
			)
		}
	}

	// find failing pods for this deployment and get their logs
	for _, pod := range snapshot.Pods {
		if pod.Namespace == ic.ResourceNamespace &&
			strings.Contains(pod.Name, ic.ResourceName) &&
			strings.ToLower(pod.Status) != "running" {

			podCtx := &IssueContext{
				ResourceName:      pod.Name,
				ResourceNamespace: pod.Namespace,
			}
			b.enrichPodContext(ctx, podCtx)
			ic.Logs = append(ic.Logs, podCtx.Logs...)
			ic.Events = append(ic.Events, podCtx.Events...)
		}
	}
}

// enrichNodeContext fetches node conditions and events
func (b *Builder) enrichNodeContext(ctx context.Context, ic *IssueContext) {
	events, err := b.client.Kubernetes.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		for _, e := range events.Items {
			ic.Events = append(ic.Events,
				fmt.Sprintf("[%s] %s: %s", e.Type, e.Reason, e.Message),
			)
		}
	}

	node, err := b.client.Kubernetes.CoreV1().Nodes().Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err == nil {
		var conditions []string
		for _, c := range node.Status.Conditions {
			conditions = append(conditions,
				fmt.Sprintf("%s=%s (reason: %s)", c.Type, c.Status, c.Reason),
			)
		}
		ic.NodeState = strings.Join(conditions, "\n")
	}
}

// enrichGenericContext fetches events for any resource
func (b *Builder) enrichGenericContext(ctx context.Context, ic *IssueContext) {
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		for _, e := range events.Items {
			ic.Events = append(ic.Events,
				fmt.Sprintf("[%s] %s: %s", e.Type, e.Reason, e.Message),
			)
		}
	}
}
