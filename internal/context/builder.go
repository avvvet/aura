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
	ResourceName      string
	ResourceNamespace string
	ResourceKind      string
	IssueTitle        string
	IssueSeverity     string
	Identifiers       map[string]string
	Events            []string
	Logs              []string
	NodeState         string
	ClusterName       string
	Context           string
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
		Identifiers:       make(map[string]string),
	}

	// always present identifiers
	ic.Identifiers["RESOURCE_KIND"] = kind
	ic.Identifiers["RESOURCE_NAME"] = resourceName
	ic.Identifiers["NAMESPACE"] = namespace

	switch strings.ToLower(kind) {
	case "pod":
		b.enrichPodContext(ctx, ic)
	case "deployment":
		b.enrichDeploymentContext(ctx, ic)
	case "node":
		b.enrichNodeContext(ctx, ic)
	case "namespace":
		b.enrichNamespaceContext(ctx, ic)
	case "ingress":
		b.enrichIngressContext(ctx, ic)
	case "pvc":
		b.enrichPVCContext(ctx, ic)
	default:
		b.enrichGenericContext(ctx, ic)
	}

	return ic, nil
}

// enrichPodContext fetches focused context for a pod issue
func (b *Builder) enrichPodContext(ctx context.Context, ic *IssueContext) {
	pod, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	// standalone or managed
	if len(pod.OwnerReferences) == 0 {
		ic.Identifiers["POD_TYPE"] = "standalone"
		ic.Identifiers["FIX_METHOD"] = "delete and recreate with pinned image"
	} else {
		ic.Identifiers["POD_TYPE"] = "managed"
		owner := pod.OwnerReferences[0]
		if owner.Kind == "ReplicaSet" {
			parts := strings.Split(owner.Name, "-")
			if len(parts) > 1 {
				ic.Identifiers["OWNER_DEPLOYMENT"] = strings.Join(parts[:len(parts)-1], "-")
			}
		}
	}

	// container identifiers
	for _, c := range pod.Spec.Containers {
		ic.Identifiers["CONTAINER_NAME"] = c.Name
		ic.Identifiers["CURRENT_IMAGE"] = c.Image
		parts := strings.SplitN(c.Image, ":", 2)
		ic.Identifiers["IMAGE_BASE"] = parts[0]
		if len(parts) == 2 {
			ic.Identifiers["CURRENT_TAG"] = parts[1]
		} else {
			ic.Identifiers["CURRENT_TAG"] = "latest"
		}
		ic.Events = append(ic.Events,
			fmt.Sprintf("container '%s' image: %s", c.Name, c.Image),
		)
	}

	// pod phase
	ic.Events = append(ic.Events,
		fmt.Sprintf("pod phase: %s", pod.Status.Phase),
	)

	// container statuses
	for _, cs := range pod.Status.ContainerStatuses {
		ic.Events = append(ic.Events,
			fmt.Sprintf("container '%s' ready=%v restarts=%d",
				cs.Name, cs.Ready, cs.RestartCount),
		)
		if cs.LastTerminationState.Terminated != nil {
			ic.Events = append(ic.Events,
				fmt.Sprintf("container '%s' last exit: reason=%s exitCode=%d",
					cs.Name,
					cs.LastTerminationState.Terminated.Reason,
					cs.LastTerminationState.Terminated.ExitCode,
				),
			)
		}
	}

	// warning events only — max 10
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		count := 0
		for _, e := range events.Items {
			if e.Type == "Warning" && count < 10 {
				ic.Events = append(ic.Events,
					fmt.Sprintf("[Warning] %s: %s", e.Reason, e.Message),
				)
				count++
			}
		}
	}

	// current logs — last 20 lines
	tailLines := int64(20)
	logs, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		GetLogs(ic.ResourceName, &corev1.PodLogOptions{
			TailLines: &tailLines,
		}).DoRaw(ctx)
	if err == nil && len(logs) > 0 {
		for _, l := range strings.Split(string(logs), "\n") {
			if l != "" {
				ic.Logs = append(ic.Logs, l)
			}
		}
	}

	// previous container logs
	prevLogs, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		GetLogs(ic.ResourceName, &corev1.PodLogOptions{
			TailLines: &tailLines,
			Previous:  true,
		}).DoRaw(ctx)
	if err == nil && len(prevLogs) > 0 {
		ic.Logs = append(ic.Logs, "--- previous container logs ---")
		for _, l := range strings.Split(string(prevLogs), "\n") {
			if l != "" {
				ic.Logs = append(ic.Logs, l)
			}
		}
	}

	// node state
	if pod.Spec.NodeName != "" {
		ic.Identifiers["NODE_NAME"] = pod.Spec.NodeName
		node, err := b.client.Kubernetes.CoreV1().
			Nodes().Get(ctx, pod.Spec.NodeName, metav1.GetOptions{})
		if err == nil {
			var conditions []string
			for _, c := range node.Status.Conditions {
				conditions = append(conditions,
					fmt.Sprintf("%s=%s", c.Type, c.Status),
				)
			}
			ic.NodeState = fmt.Sprintf("node: %s  conditions: %s  cpu: %s  memory: %s",
				pod.Spec.NodeName,
				strings.Join(conditions, ", "),
				node.Status.Capacity.Cpu().String(),
				node.Status.Capacity.Memory().String(),
			)
		}
	}
}

// enrichDeploymentContext fetches focused context for a deployment issue
func (b *Builder) enrichDeploymentContext(ctx context.Context, ic *IssueContext) {
	deploy, err := b.client.Kubernetes.AppsV1().
		Deployments(ic.ResourceNamespace).
		Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	// replica count
	if deploy.Spec.Replicas != nil {
		ic.Identifiers["REPLICA_COUNT"] = fmt.Sprintf("%d", *deploy.Spec.Replicas)
	}

	// replica status
	ic.Events = append(ic.Events,
		fmt.Sprintf("replicas: desired=%d ready=%d available=%d updated=%d",
			*deploy.Spec.Replicas,
			deploy.Status.ReadyReplicas,
			deploy.Status.AvailableReplicas,
			deploy.Status.UpdatedReplicas,
		),
	)

	// container info
	for _, c := range deploy.Spec.Template.Spec.Containers {
		ic.Identifiers["CONTAINER_NAME"] = c.Name
		ic.Identifiers["CURRENT_IMAGE"] = c.Image
		parts := strings.SplitN(c.Image, ":", 2)
		ic.Identifiers["IMAGE_BASE"] = parts[0]
		if len(parts) == 2 {
			ic.Identifiers["CURRENT_TAG"] = parts[1]
		} else {
			ic.Identifiers["CURRENT_TAG"] = "latest"
		}

		if c.Resources.Limits != nil {
			ic.Identifiers["CPU_LIMIT"] = c.Resources.Limits.Cpu().String()
			ic.Identifiers["MEMORY_LIMIT"] = c.Resources.Limits.Memory().String()
		} else {
			ic.Identifiers["CPU_LIMIT"] = "none"
			ic.Identifiers["MEMORY_LIMIT"] = "none"
		}

		if c.Resources.Requests != nil {
			ic.Identifiers["CPU_REQUEST"] = c.Resources.Requests.Cpu().String()
			ic.Identifiers["MEMORY_REQUEST"] = c.Resources.Requests.Memory().String()
		} else {
			ic.Identifiers["CPU_REQUEST"] = "none"
			ic.Identifiers["MEMORY_REQUEST"] = "none"
		}

		ic.Events = append(ic.Events,
			fmt.Sprintf("container '%s' image: %s", c.Name, c.Image),
		)
		if c.Resources.Limits != nil {
			ic.Events = append(ic.Events,
				fmt.Sprintf("container '%s' limits: cpu=%s memory=%s",
					c.Name,
					c.Resources.Limits.Cpu().String(),
					c.Resources.Limits.Memory().String(),
				),
			)
		} else {
			ic.Events = append(ic.Events,
				fmt.Sprintf("container '%s' limits: none", c.Name),
			)
		}
		if c.Resources.Requests != nil {
			ic.Events = append(ic.Events,
				fmt.Sprintf("container '%s' requests: cpu=%s memory=%s",
					c.Name,
					c.Resources.Requests.Cpu().String(),
					c.Resources.Requests.Memory().String(),
				),
			)
		} else {
			ic.Events = append(ic.Events,
				fmt.Sprintf("container '%s' requests: none", c.Name),
			)
		}
	}

	// warning events only — max 10
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		count := 0
		for _, e := range events.Items {
			if e.Type == "Warning" && count < 10 {
				ic.Events = append(ic.Events,
					fmt.Sprintf("[Warning] %s: %s", e.Reason, e.Message),
				)
				count++
			}
		}
	}
}

// enrichNamespaceContext fetches focused context for a namespace issue
func (b *Builder) enrichNamespaceContext(ctx context.Context, ic *IssueContext) {
	policies, err := b.client.Kubernetes.NetworkingV1().
		NetworkPolicies(ic.ResourceNamespace).
		List(ctx, metav1.ListOptions{})
	if err == nil {
		if len(policies.Items) == 0 {
			ic.Events = append(ic.Events,
				fmt.Sprintf("namespace '%s' has 0 network policies", ic.ResourceNamespace),
			)
		} else {
			for _, p := range policies.Items {
				ic.Events = append(ic.Events,
					fmt.Sprintf("existing policy: %s", p.Name),
				)
			}
		}
	}

	pods, err := b.client.Kubernetes.CoreV1().
		Pods(ic.ResourceNamespace).
		List(ctx, metav1.ListOptions{})
	if err == nil {
		ic.Identifiers["POD_COUNT"] = fmt.Sprintf("%d", len(pods.Items))
		ic.Events = append(ic.Events,
			fmt.Sprintf("pods in namespace: %d", len(pods.Items)),
		)
	}
}

// enrichIngressContext fetches context for an ingress issue
func (b *Builder) enrichIngressContext(ctx context.Context, ic *IssueContext) {
	ing, err := b.client.Kubernetes.NetworkingV1().
		Ingresses(ic.ResourceNamespace).
		Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	if len(ing.Spec.TLS) == 0 {
		ic.Events = append(ic.Events, "ingress has no TLS configuration")
	}

	for _, rule := range ing.Spec.Rules {
		ic.Identifiers["INGRESS_HOST"] = rule.Host
		ic.Events = append(ic.Events,
			fmt.Sprintf("host: %s", rule.Host),
		)
	}

	if ing.Spec.IngressClassName != nil {
		ic.Identifiers["INGRESS_CLASS"] = *ing.Spec.IngressClassName
		ic.Events = append(ic.Events,
			fmt.Sprintf("ingress class: %s", *ing.Spec.IngressClassName),
		)
	}
}

// enrichPVCContext fetches context for a PVC issue
func (b *Builder) enrichPVCContext(ctx context.Context, ic *IssueContext) {
	pvc, err := b.client.Kubernetes.CoreV1().
		PersistentVolumeClaims(ic.ResourceNamespace).
		Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	ic.Identifiers["PVC_STATUS"] = string(pvc.Status.Phase)
	ic.Events = append(ic.Events,
		fmt.Sprintf("pvc status: %s", pvc.Status.Phase),
	)

	if pvc.Spec.StorageClassName != nil {
		ic.Identifiers["STORAGE_CLASS"] = *pvc.Spec.StorageClassName
		ic.Events = append(ic.Events,
			fmt.Sprintf("storage class: %s", *pvc.Spec.StorageClassName),
		)
	}

	if storage := pvc.Status.Capacity.Storage(); storage != nil {
		ic.Identifiers["CAPACITY"] = storage.String()
		ic.Events = append(ic.Events,
			fmt.Sprintf("capacity: %s", storage.String()),
		)
	}
}

// enrichNodeContext fetches context for a node issue
func (b *Builder) enrichNodeContext(ctx context.Context, ic *IssueContext) {
	node, err := b.client.Kubernetes.CoreV1().
		Nodes().Get(ctx, ic.ResourceName, metav1.GetOptions{})
	if err != nil {
		return
	}

	ic.Identifiers["NODE_NAME"] = ic.ResourceName

	for _, c := range node.Status.Conditions {
		ic.Events = append(ic.Events,
			fmt.Sprintf("condition %s=%s reason: %s message: %s",
				c.Type, c.Status, c.Reason, c.Message),
		)
	}

	ic.NodeState = fmt.Sprintf("cpu: %s  memory: %s  pods: %s",
		node.Status.Capacity.Cpu().String(),
		node.Status.Capacity.Memory().String(),
		node.Status.Capacity.Pods().String(),
	)

	events, err := b.client.Kubernetes.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		count := 0
		for _, e := range events.Items {
			if e.Type == "Warning" && count < 10 {
				ic.Events = append(ic.Events,
					fmt.Sprintf("[Warning] %s: %s", e.Reason, e.Message),
				)
				count++
			}
		}
	}
}

// enrichGenericContext fetches warning events for any resource
func (b *Builder) enrichGenericContext(ctx context.Context, ic *IssueContext) {
	events, err := b.client.Kubernetes.CoreV1().Events(ic.ResourceNamespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s", ic.ResourceName),
	})
	if err == nil {
		count := 0
		for _, e := range events.Items {
			if e.Type == "Warning" && count < 10 {
				ic.Events = append(ic.Events,
					fmt.Sprintf("[Warning] %s: %s", e.Reason, e.Message),
				)
				count++
			}
		}
	}
}
