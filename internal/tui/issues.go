package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/avvvet/aura/internal/model"
)

// detectIssues analyzes the snapshot and returns a list of issues
func detectIssues(s *model.ClusterSnapshot) []Issue {
	var issues []Issue

	if s == nil {
		return issues
	}

	// check nodes
	for _, n := range s.Nodes {
		if strings.ToLower(n.Status) != "ready" {
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "node",
				Title:        fmt.Sprintf("node is %s", n.Status),
				Resource:     n.Name,
				Namespace:    "cluster",
				Meta:         fmt.Sprintf("role: %s  ·  version: %s", n.Roles, n.Version),
				Command:      fmt.Sprintf("kubectl describe node %s", n.Name),
				DetectedAt:   time.Now(),
			})
		}
	}

	// check pods
	for _, p := range s.Pods {
		switch strings.ToLower(p.Status) {
		case "failed", "crashloopbackoff", "error":
			title := fmt.Sprintf("pod is %s", p.Status)
			resource := p.Name
			resourceType := "pod"
			meta := fmt.Sprintf("pod: %s  ·  restarts: %d  ·  node: %s", p.Name, p.Restarts, p.Node)

			if p.OwnerKind == "deployment" && p.OwnerName != "" {
				title = fmt.Sprintf("deployment has a %s pod", p.Status)
				resource = p.OwnerName
				resourceType = "deployment"
				meta = fmt.Sprintf("pod: %s  ·  restarts: %d", p.Name, p.Restarts)
			}

			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: resourceType,
				Title:        title,
				Resource:     resource,
				Namespace:    p.Namespace,
				Meta:         meta,
				Command:      fmt.Sprintf("kubectl describe %s %s -n %s", resourceType, resource, p.Namespace),
				DetectedAt:   time.Now(),
			})

		case "pending":
			resource := p.Name
			resourceType := "pod"
			title := "pod is pending"
			meta := fmt.Sprintf("pod: %s  ·  node: %s", p.Name, p.Node)

			if p.OwnerKind == "deployment" && p.OwnerName != "" {
				title = "deployment has a pending pod"
				resource = p.OwnerName
				resourceType = "deployment"
				meta = fmt.Sprintf("pod: %s", p.Name)
			}

			issues = append(issues, Issue{
				Severity:     "warning",
				ResourceType: resourceType,
				Title:        title,
				Resource:     resource,
				Namespace:    p.Namespace,
				Meta:         meta,
				Command:      fmt.Sprintf("kubectl describe %s %s -n %s", resourceType, resource, p.Namespace),
				DetectedAt:   time.Now(),
			})
		}

		// high restarts
		if p.Restarts > 5 {
			resource := p.Name
			resourceType := "pod"
			title := fmt.Sprintf("pod has %d restarts", p.Restarts)
			meta := fmt.Sprintf("pod: %s  ·  status: %s", p.Name, p.Status)

			if p.OwnerKind == "deployment" && p.OwnerName != "" {
				title = fmt.Sprintf("deployment pod restarting %d times", p.Restarts)
				resource = p.OwnerName
				resourceType = "deployment"
				meta = fmt.Sprintf("pod: %s  ·  status: %s", p.Name, p.Status)
			}

			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: resourceType,
				Title:        title,
				Resource:     resource,
				Namespace:    p.Namespace,
				Meta:         meta,
				Command:      fmt.Sprintf("kubectl logs %s -n %s --previous --tail=50", p.Name, p.Namespace),
				DetectedAt:   time.Now(),
			})
		}
	}

	// check deployments
	for _, d := range s.Deployments {
		if d.Available == 0 && d.UpToDate > 0 {
			issues = append(issues, Issue{
				Severity:     "critical",
				ResourceType: "deployment",
				Title:        "deployment has 0 available pods",
				Resource:     d.Name,
				Namespace:    d.Namespace,
				Meta:         fmt.Sprintf("ready: %s  ·  age: %s", d.Ready, d.Age),
				Command:      fmt.Sprintf("kubectl describe deployment %s -n %s", d.Name, d.Namespace),
				DetectedAt:   time.Now(),
			})
		}
	}

	// good practice — no resource limits
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
			Title:        "deployment has no resource limits",
			Resource:     name,
			Namespace:    ns,
			Meta:         "risk: node starvation",
			Command:      fmt.Sprintf("kubectl set resources deployment/%s --requests=cpu=100m,memory=128Mi --limits=cpu=500m,memory=256Mi -n %s", name, ns),
			DetectedAt:   time.Now(),
		})
	}

	// good practice — unattached pvcs
	if len(s.CostSignals.UnattachedPVCs) > 0 {
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
				Title:        "pvc is unattached and billing",
				Resource:     name,
				Namespace:    ns,
				Meta:         fmt.Sprintf("namespace: %s  ·  ~$10/month waste", ns),
				Command:      fmt.Sprintf("kubectl delete pvc %s -n %s", name, ns),
				DetectedAt:   time.Now(),
			})
		}
	}

	// good practice — idle namespaces
	for _, ns := range s.CostSignals.IdleNamespaces {
		issues = append(issues, Issue{
			Severity:     "warning",
			ResourceType: "namespace",
			Title:        "namespace is idle",
			Resource:     ns,
			Namespace:    ns,
			Meta:         "no active workloads  ·  ~$340/month waste",
			Command:      fmt.Sprintf("kubectl delete namespace %s", ns),
			DetectedAt:   time.Now(),
		})
	}

	// security issues
	issues = append(issues, detectSecurityIssues(s)...)

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

// detectResolved checks which previous issues no longer appear in current issues
func detectResolved(current []Issue, previous []ResolvedIssue, prev []Issue) []ResolvedIssue {
	resolved := previous

	currentTitles := make(map[string]bool)
	for _, c := range current {
		currentTitles[c.Title+c.Resource] = true
	}

	for _, p := range prev {
		if !currentTitles[p.Title+p.Resource] {
			resolved = append(resolved, ResolvedIssue{
				Title:      fmt.Sprintf("%s %s", p.ResourceType, p.Title),
				ResolvedAt: time.Now(),
			})
		}
	}

	if len(resolved) > 5 {
		resolved = resolved[len(resolved)-5:]
	}

	return resolved
}
