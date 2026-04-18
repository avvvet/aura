package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/avvvet/aura/internal/model"
)

// detectIssues analyzes the snapshot per resource type
// mirrors exactly how the health grid counts resources
func detectIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	if s == nil {
		return issues
	}

	// detect per node
	issues = append(issues, detectNodeIssues(s)...)

	// detect per deployment
	issues = append(issues, detectDeploymentIssues(s)...)

	// detect per namespace
	issues = append(issues, detectNamespaceIssues(s)...)

	// detect per pvc
	issues = append(issues, detectPVCIssues(s)...)

	// detect standalone pod issues
	issues = append(issues, detectStandalonePodIssues(s)...)

	// detect security issues
	issues = append(issues, detectSecurityIssues(s)...)

	return issues
}

// detectNodeIssues loops nodes directly
func detectNodeIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	for _, n := range s.Nodes {
		if strings.ToLower(n.Status) != "ready" {
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "node",
				Title:        "is not ready",
				Resource:     n.Name,
				Namespace:    "cluster",
				Meta:         fmt.Sprintf("role: %s  version: %s", n.Roles, n.Version),
				Command:      fmt.Sprintf("kubectl describe node %s", n.Name),
				DetectedAt:   time.Now(),
			})
		}
	}

	return issues
}

// detectDeploymentIssues loops deployments directly
// uses pods only as evidence not as issue source
func detectDeploymentIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	for _, d := range s.Deployments {
		// get pods for this deployment
		pods := getDeploymentPods(d.Name, d.Namespace, s)

		// check availability
		if d.Available == 0 && d.UpToDate > 0 {
			reason, meta := analyzeDeploymentFailure(d, pods)
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "deployment",
				Title:        reason,
				Resource:     d.Name,
				Namespace:    d.Namespace,
				Meta:         meta,
				Command:      fmt.Sprintf("kubectl describe deployment %s -n %s", d.Name, d.Namespace),
				DetectedAt:   time.Now(),
			})
			continue
		}

		// check partial availability
		if d.Available > 0 && d.Available < d.UpToDate {
			issues = append(issues, Issue{
				Severity:     "warning",
				ResourceType: "deployment",
				Title:        "partially available",
				Resource:     d.Name,
				Namespace:    d.Namespace,
				Meta:         fmt.Sprintf("ready: %s  expected: %d", d.Ready, d.UpToDate),
				Command:      fmt.Sprintf("kubectl describe deployment %s -n %s", d.Name, d.Namespace),
				DetectedAt:   time.Now(),
			})
		}

		// check high restarts across pods
		for _, p := range pods {
			if p.Restarts > 5 {
				issues = append(issues, Issue{
					Severity:     "critical",
					ResourceType: "deployment",
					Title:        fmt.Sprintf("pod restarting %d times", p.Restarts),
					Resource:     d.Name,
					Namespace:    d.Namespace,
					Meta:         fmt.Sprintf("pod: %s  restarts: %d", p.Name, p.Restarts),
					Command:      fmt.Sprintf("kubectl logs %s -n %s --previous --tail=50", p.Name, d.Namespace),
					DetectedAt:   time.Now(),
				})
				break // one issue per deployment
			}
		}
	}

	// good practice — no resource limits per deployment
	for _, deplRef := range s.CostSignals.PodsWithNoLimits {
		parts := strings.SplitN(deplRef, "/", 2)
		ns, name := "default", deplRef
		if len(parts) == 2 {
			ns = parts[0]
			name = parts[1]
		}
		issues = append(issues, Issue{
			Severity:     "warning",
			ResourceType: "deployment",
			Title:        "has no resource limits",
			Resource:     name,
			Namespace:    ns,
			Meta:         "risk: node starvation",
			Command:      fmt.Sprintf("kubectl set resources deployment/%s --requests=cpu=100m,memory=128Mi --limits=cpu=500m,memory=512Mi -n %s", name, ns),
			DetectedAt:   time.Now(),
		})
	}

	return issues
}

// getDeploymentPods returns pods belonging to a deployment
func getDeploymentPods(deployName, namespace string, s *model.ClusterSnapshot) []model.Pod {
	var pods []model.Pod
	for _, p := range s.Pods {
		if p.Namespace == namespace &&
			p.OwnerKind == "deployment" &&
			p.OwnerName == deployName {
			pods = append(pods, p)
		}
	}
	return pods
}

// analyzeDeploymentFailure determines root cause from pod states
func analyzeDeploymentFailure(d model.Deployment, pods []model.Pod) (title, meta string) {
	podSummary := fmt.Sprintf("pods: %d/%d ready", d.Available, d.UpToDate)

	// collect all container states as evidence
	var waitingReasons []string
	var terminatedReasons []string

	for _, p := range pods {
		for _, cs := range p.ContainerStates {
			if cs.WaitingReason != "" {
				waitingReasons = append(waitingReasons, cs.WaitingReason)
			}
			if cs.TerminatedReason != "" {
				terminatedReasons = append(terminatedReasons, cs.TerminatedReason)
			}
		}
	}

	// most specific reason from Kubernetes directly
	if len(waitingReasons) > 0 {
		return fmt.Sprintf("pods waiting: %s", waitingReasons[0]),
			fmt.Sprintf("%s  reason: %s", podSummary, waitingReasons[0])
	}

	if len(terminatedReasons) > 0 {
		return fmt.Sprintf("pods terminated: %s", terminatedReasons[0]),
			fmt.Sprintf("%s  reason: %s", podSummary, terminatedReasons[0])
	}

	// generic fallback
	return "has unhealthy pods", podSummary
}

// detectStandalonePodIssues detects issues on pods not owned by any deployment
func detectStandalonePodIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	for _, p := range s.Pods {
		// skip pods owned by deployments — handled by detectDeploymentIssues
		if p.OwnerKind == "deployment" || p.OwnerName != "" {
			continue
		}

		switch strings.ToLower(p.Status) {
		case "failed", "error":
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "pod",
				Title:        fmt.Sprintf("is %s", p.Status),
				Resource:     p.Name,
				Namespace:    p.Namespace,
				Meta:         fmt.Sprintf("restarts: %d  node: %s", p.Restarts, p.Node),
				Command:      fmt.Sprintf("kubectl logs %s -n %s --previous", p.Name, p.Namespace),
				DetectedAt:   time.Now(),
			})
		case "pending":
			issues = append(issues, Issue{
				Severity:     "warning",
				ResourceType: "pod",
				Title:        "is pending",
				Resource:     p.Name,
				Namespace:    p.Namespace,
				Meta:         fmt.Sprintf("node: %s", p.Node),
				Command:      fmt.Sprintf("kubectl describe pod %s -n %s", p.Name, p.Namespace),
				DetectedAt:   time.Now(),
			})
		}

		if p.Restarts > 5 {
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "pod",
				Title:        fmt.Sprintf("restarting %d times", p.Restarts),
				Resource:     p.Name,
				Namespace:    p.Namespace,
				Meta:         fmt.Sprintf("status: %s  node: %s", p.Status, p.Node),
				Command:      fmt.Sprintf("kubectl logs %s -n %s --previous --tail=50", p.Name, p.Namespace),
				DetectedAt:   time.Now(),
			})
		}
	}

	return issues
}

// detectNamespaceIssues loops namespaces directly
func detectNamespaceIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	// idle namespaces
	for _, ns := range s.CostSignals.IdleNamespaces {
		issues = append(issues, Issue{
			Severity:     "warning",
			ResourceType: "namespace",
			Title:        "is idle",
			Resource:     ns,
			Namespace:    ns,
			Meta:         "no active workloads  ·  ~$340/month waste",
			Command:      fmt.Sprintf("kubectl delete namespace %s", ns),
			DetectedAt:   time.Now(),
		})
	}

	return issues
}

// detectPVCIssues loops pvcs directly
func detectPVCIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	for _, pvcRef := range s.CostSignals.UnattachedPVCs {
		parts := strings.SplitN(pvcRef, "/", 2)
		ns, name := "default", pvcRef
		if len(parts) == 2 {
			ns = parts[0]
			name = parts[1]
		}
		issues = append(issues, Issue{
			Severity:     "warning",
			ResourceType: "pvc",
			Title:        "is unattached and billing",
			Resource:     name,
			Namespace:    ns,
			Meta:         "~$10/month waste",
			Command:      fmt.Sprintf("kubectl delete pvc %s -n %s", name, ns),
			DetectedAt:   time.Now(),
		})
	}

	return issues
}

// detectSecurityIssues checks for security misconfigurations
func detectSecurityIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue
	sec := s.SecuritySignals

	if len(sec.PrivilegedContainers) > 0 {
		for _, ref := range sec.PrivilegedContainers {
			parts := strings.SplitN(ref, "/", 4)
			ns, pod, container := "default", ref, ""
			if len(parts) >= 3 {
				ns = parts[0]
				pod = parts[1]
				container = parts[2]
			}
			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: "pod",
				Title:        "running privileged container",
				Resource:     pod,
				Namespace:    ns,
				Meta:         fmt.Sprintf("container: %s", container),
				Command:      fmt.Sprintf("kubectl describe pod %s -n %s", pod, ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	if len(sec.SecretsInEnvVars) > 0 {
		for _, ref := range sec.SecretsInEnvVars {
			parts := strings.SplitN(ref, "/", 4)
			ns, pod, container := "default", ref, ""
			if len(parts) >= 3 {
				ns = parts[0]
				pod = parts[1]
				container = parts[2]
			}
			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: "pod",
				Title:        "exposing secrets in env vars",
				Resource:     pod,
				Namespace:    ns,
				Meta:         fmt.Sprintf("container: %s  ·  use secretRef instead", container),
				Command:      fmt.Sprintf("kubectl get pod %s -n %s -o jsonpath='{.spec.containers[*].env}'", pod, ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	if len(sec.HostNetworkPods) > 0 {
		for _, ref := range sec.HostNetworkPods {
			parts := strings.SplitN(ref, "/", 2)
			ns, pod := "default", ref
			if len(parts) == 2 {
				ns = parts[0]
				pod = parts[1]
			}
			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: "pod",
				Title:        "using host network",
				Resource:     pod,
				Namespace:    ns,
				Meta:         "bypasses network isolation",
				Command:      fmt.Sprintf("kubectl describe pod %s -n %s", pod, ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	if len(sec.IngressesWithoutTLS) > 0 {
		for _, ref := range sec.IngressesWithoutTLS {
			parts := strings.SplitN(ref, "/", 2)
			ns, name := "default", ref
			if len(parts) == 2 {
				ns = parts[0]
				name = parts[1]
			}
			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: "ingress",
				Title:        "has no TLS configured",
				Resource:     name,
				Namespace:    ns,
				Meta:         "traffic is unencrypted",
				Command:      fmt.Sprintf("kubectl describe ingress %s -n %s", name, ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	if len(sec.NamespacesWithoutNetPol) > 0 {
		for _, ns := range sec.NamespacesWithoutNetPol {
			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: "namespace",
				Title:        "has no network policy",
				Resource:     ns,
				Namespace:    ns,
				Meta:         "unrestricted pod-to-pod communication",
				Command:      fmt.Sprintf("kubectl get networkpolicies -n %s", ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	if len(sec.LatestImageTags) > 0 {
		for _, ref := range sec.LatestImageTags {
			parts := strings.SplitN(ref, "/", 4)
			ns, pod, container, owner := "default", ref, "", "standalone"
			if len(parts) == 4 {
				ns = parts[0]
				pod = parts[1]
				container = parts[2]
				owner = parts[3]
			}

			resource := pod
			resourceType := "pod"
			command := fmt.Sprintf("kubectl describe pod %s -n %s", pod, ns)
			meta := fmt.Sprintf("container: %s  ·  standalone pod, delete and recreate with pinned tag", container)

			if strings.HasPrefix(owner, "deployment:") {
				deployName := strings.TrimPrefix(owner, "deployment:")
				resource = deployName
				resourceType = "deployment"
				command = fmt.Sprintf("kubectl set image deployment/%s %s=<image>:<pinned-version> -n %s", deployName, container, ns)
				meta = fmt.Sprintf("container: %s", container)
			}

			issues = append(issues, Issue{
				Severity:     "security",
				ResourceType: resourceType,
				Title:        "using unpinned image tag",
				Resource:     resource,
				Namespace:    ns,
				Meta:         meta,
				Command:      command,
				DetectedAt:   time.Now(),
			})
		}
	}

	return issues
}

// detectResolved checks which previous issues no longer appear
func detectResolved(current []Issue, previous []ResolvedIssue, prev []Issue) []ResolvedIssue {
	resolved := previous

	currentKeys := make(map[string]bool)
	for _, c := range current {
		currentKeys[c.ResourceType+c.Resource+c.Title] = true
	}

	for _, p := range prev {
		if !currentKeys[p.ResourceType+p.Resource+p.Title] {
			resolved = append(resolved, ResolvedIssue{
				Title:        fmt.Sprintf("%s %s — %s", p.ResourceType, p.Resource, p.Title),
				ResolvedAt:   time.Now(),
				ResourceType: p.ResourceType,
				Resource:     p.Resource,
			})
		}
	}

	if len(resolved) > 5 {
		resolved = resolved[len(resolved)-5:]
	}

	return resolved
}
