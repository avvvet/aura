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
				Severity:   "critical",
				Title:      fmt.Sprintf("%s is NotReady", n.Name),
				Resource:   n.Name,
				Namespace:  "cluster",
				Meta:       fmt.Sprintf("role: %s  ·  version: %s", n.Roles, n.Version),
				Command:    fmt.Sprintf("kubectl describe node %s | tail -20", n.Name),
				DetectedAt: time.Now(),
			})
		}
	}

	// check pods
	for _, p := range s.Pods {
		switch strings.ToLower(p.Status) {
		case "failed", "crashloopbackoff", "error":
			issues = append(issues, Issue{
				Severity:   "critical",
				Title:      fmt.Sprintf("%s is %s", p.Name, p.Status),
				Resource:   p.Name,
				Namespace:  p.Namespace,
				Meta:       fmt.Sprintf("namespace: %s  ·  restarts: %d  ·  node: %s", p.Namespace, p.Restarts, p.Node),
				Command:    fmt.Sprintf("kubectl logs %s -n %s --previous", p.Name, p.Namespace),
				DetectedAt: time.Now(),
			})
		case "pending":
			issues = append(issues, Issue{
				Severity:   "warning",
				Title:      fmt.Sprintf("%s is Pending", p.Name),
				Resource:   p.Name,
				Namespace:  p.Namespace,
				Meta:       fmt.Sprintf("namespace: %s  ·  node: %s", p.Namespace, p.Node),
				Command:    fmt.Sprintf("kubectl describe pod %s -n %s | grep -A10 Events", p.Name, p.Namespace),
				DetectedAt: time.Now(),
			})
		}

		// high restarts
		if p.Restarts > 5 {
			issues = append(issues, Issue{
				Severity:   "critical",
				Title:      fmt.Sprintf("%s has %d restarts", p.Name, p.Restarts),
				Resource:   p.Name,
				Namespace:  p.Namespace,
				Meta:       fmt.Sprintf("namespace: %s  ·  status: %s", p.Namespace, p.Status),
				Command:    fmt.Sprintf("kubectl logs %s -n %s --previous --tail=50", p.Name, p.Namespace),
				DetectedAt: time.Now(),
			})
		}
	}

	// check deployments
	for _, d := range s.Deployments {
		if d.Available == 0 && d.UpToDate > 0 {
			issues = append(issues, Issue{
				Severity:   "critical",
				Title:      fmt.Sprintf("%s has 0 available pods", d.Name),
				Resource:   d.Name,
				Namespace:  d.Namespace,
				Meta:       fmt.Sprintf("namespace: %s  ·  ready: %s", d.Namespace, d.Ready),
				Command:    fmt.Sprintf("kubectl describe deployment %s -n %s", d.Name, d.Namespace),
				DetectedAt: time.Now(),
			})
		}
	}

	// good practice — no resource limits
	if len(s.CostSignals.PodsWithNoLimits) > 0 {
		issues = append(issues, Issue{
			Severity:   "warning",
			Title:      fmt.Sprintf("%d deployments have no resource limits", len(s.CostSignals.PodsWithNoLimits)),
			Resource:   "multiple",
			Namespace:  "cluster",
			Meta:       strings.Join(s.CostSignals.PodsWithNoLimits, ", "),
			Command:    "kubectl describe deployment <name> -n <namespace> | grep -A5 Limits",
			DetectedAt: time.Now(),
		})
	}

	// good practice — unattached pvcs
	if len(s.CostSignals.UnattachedPVCs) > 0 {
		issues = append(issues, Issue{
			Severity:   "warning",
			Title:      fmt.Sprintf("%d unattached PVCs — potential waste", len(s.CostSignals.UnattachedPVCs)),
			Resource:   "multiple",
			Namespace:  "cluster",
			Meta:       strings.Join(s.CostSignals.UnattachedPVCs, ", "),
			Command:    "kubectl get pvc -A | grep -v Bound",
			DetectedAt: time.Now(),
		})
	}

	// good practice — idle namespaces
	if len(s.CostSignals.IdleNamespaces) > 0 {
		issues = append(issues, Issue{
			Severity:   "warning",
			Title:      fmt.Sprintf("%d idle namespaces — ~$340/month waste", len(s.CostSignals.IdleNamespaces)),
			Resource:   "multiple",
			Namespace:  "cluster",
			Meta:       strings.Join(s.CostSignals.IdleNamespaces, ", "),
			Command:    fmt.Sprintf("kubectl delete namespace %s", strings.Join(s.CostSignals.IdleNamespaces, " ")),
			DetectedAt: time.Now(),
		})
	}

	return issues
}

// detectResolved checks which previous issues no longer appear in current issues
func detectResolved(current []Issue, previous []ResolvedIssue) []ResolvedIssue {
	resolved := previous

	// keep only last 5 resolved issues
	if len(resolved) > 5 {
		resolved = resolved[len(resolved)-5:]
	}

	return resolved
}
