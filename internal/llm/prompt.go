package llm

import (
	"fmt"
	"strings"
)

// BuildPrompt builds a dynamic natural language prompt from cluster context
func BuildPrompt(ic *IssueContext) string {
	loader := NewPromptLoader()
	var b strings.Builder

	// base from file — principles only
	b.WriteString(loader.LoadBase())
	b.WriteString("\n\n")

	// issues list — clean titles only
	b.WriteString("## THE ISSUES TO ANALYZE\n")
	for i, issue := range ic.Issues {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, issue.Title))
	}
	b.WriteString("\n")

	// dynamic issue description — built in Go per issue type
	b.WriteString("## ISSUE CONTEXT\n")
	for _, issue := range ic.Issues {
		b.WriteString(fmt.Sprintf("### %s\n", issue.Title))
		b.WriteString(buildIssueDescription(ic, issue.Title))
		b.WriteString("\n")
	}

	// resource description
	b.WriteString("## THE AFFECTED RESOURCE\n")
	b.WriteString(buildResourceDescription(ic))
	b.WriteString("\n")

	// external guidance from file — additional hints
	issueContext := loader.LoadIssue(ic.ResourceKind)
	if issueContext != "" {
		b.WriteString("## ADDITIONAL GUIDANCE\n")
		b.WriteString(issueContext)
		b.WriteString("\n")
	}

	// events
	if len(ic.Events) > 0 {
		b.WriteString("\n## RECENT EVENTS\n")
		for _, e := range ic.Events {
			b.WriteString(fmt.Sprintf("- %s\n", e))
		}
	}

	// node state
	if ic.NodeState != "" {
		b.WriteString(fmt.Sprintf("\n## NODE STATE\n%s\n", ic.NodeState))
	}

	// logs
	if len(ic.Logs) > 0 {
		b.WriteString("\n## CONTAINER LOGS\n")
		for _, l := range ic.Logs {
			b.WriteString(fmt.Sprintf("  %s\n", l))
		}
	}

	b.WriteString(buildInstructions(ic))

	return b.String()
}

// buildIssueDescription builds focused natural language description per issue type
func buildIssueDescription(ic *IssueContext, issueTitle string) string {
	name := ic.ResourceName
	ns := ic.ResourceNamespace
	kind := ic.ResourceKind
	image := ic.Identifiers["CURRENT_IMAGE"]
	podType := ic.Identifiers["POD_TYPE"]

	switch issueTitle {
	case "has no resource limits":
		return fmt.Sprintf(
			"The %s '%s' in namespace '%s' has no CPU or memory limits defined.\n"+
				"Current CPU limit: %s\n"+
				"Current memory limit: %s\n"+
				"Focus ONLY on the missing resource limits — ignore any other issues.",
			kind, name, ns,
			ic.Identifiers["CPU_LIMIT"],
			ic.Identifiers["MEMORY_LIMIT"],
		)

	case "using unpinned image tag":
		if podType == "standalone" {
			return fmt.Sprintf(
				"Standalone pod '%s' in namespace '%s' uses unpinned image: %s\n"+
					"This is a STANDALONE pod — not managed by a deployment.\n"+
					"Fix requires two steps joined with &&:\n"+
					"Step 1: kubectl delete pod %s -n %s\n"+
					"Step 2: kubectl run %s --image=%s:<pinned-version> -n %s\n"+
					"Provide both commands joined with && as a single fix command.",
				name, ns, image,
				name, ns,
				name, ic.Identifiers["IMAGE_BASE"], ns,
			)
		}
		return fmt.Sprintf(
			"The %s '%s' in namespace '%s' uses unpinned image: %s\n"+
				"Image base: %s\n"+
				"Fix: update to a pinned version tag using kubectl set image.",
			kind, name, ns, image,
			ic.Identifiers["IMAGE_BASE"],
		)

	case "has no network policy":
		return fmt.Sprintf(
			"Namespace '%s' has no NetworkPolicy resources defined.\n"+
				"All pods communicate freely without restriction.\n"+
				"Fix: generate kubectl apply with a default-deny-all NetworkPolicy yaml.\n"+
				"Use namespace '%s' in the yaml metadata.",
			ns, ns,
		)

	case "is unattached and billing":
		return fmt.Sprintf(
			"PVC '%s' in namespace '%s' is not mounted by any pod.\n"+
				"It is still provisioned and incurring storage costs.\n"+
				"Fix: delete if no longer needed.",
			name, ns,
		)

	case "is idle":
		return fmt.Sprintf(
			"Namespace '%s' has no active workloads.\n"+
				"Fix: delete if no longer needed.",
			name,
		)

	case "running privileged container":
		return fmt.Sprintf(
			"Pod '%s' in namespace '%s' runs a privileged container.\n"+
				"Fix requires editing the deployment spec to remove privileged: true.",
			name, ns,
		)

	case "exposing secrets in env vars":
		return fmt.Sprintf(
			"Pod '%s' in namespace '%s' has sensitive values in plain env vars.\n"+
				"Fix requires updating deployment to use secretKeyRef instead.",
			name, ns,
		)

	case "has no TLS configured":
		return fmt.Sprintf(
			"Ingress '%s' in namespace '%s' has no TLS configuration.\n"+
				"Traffic is unencrypted.\n"+
				"Fix: create TLS secret and patch ingress to add TLS section.",
			name, ns,
		)

	case "using host network":
		return fmt.Sprintf(
			"Pod '%s' in namespace '%s' uses host network.\n"+
				"This bypasses Kubernetes network isolation.",
			name, ns,
		)

	case "is not ready":
		return fmt.Sprintf(
			"Node '%s' is in NotReady state.\n"+
				"Investigate conditions and events to find root cause.",
			name,
		)

	default:
		return fmt.Sprintf(
			"Issue '%s' detected on %s '%s' in namespace '%s'.",
			issueTitle, kind, name, ns,
		)
	}
}

// buildResourceDescription builds natural language resource summary
func buildResourceDescription(ic *IssueContext) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("- Resource type: %s\n", ic.ResourceKind))
	b.WriteString(fmt.Sprintf("- Name: %s\n", ic.ResourceName))
	b.WriteString(fmt.Sprintf("- Namespace: %s\n", ic.ResourceNamespace))

	if v, ok := ic.Identifiers["CONTAINER_NAME"]; ok {
		b.WriteString(fmt.Sprintf("- Container name: %s\n", v))
	}
	if v, ok := ic.Identifiers["CURRENT_IMAGE"]; ok {
		b.WriteString(fmt.Sprintf("- Current image: %s\n", v))
	}
	if v, ok := ic.Identifiers["IMAGE_BASE"]; ok {
		b.WriteString(fmt.Sprintf("- Image base: %s\n", v))
	}
	if v, ok := ic.Identifiers["CURRENT_TAG"]; ok {
		b.WriteString(fmt.Sprintf("- Current tag: %s\n", v))
	}
	if v, ok := ic.Identifiers["CPU_LIMIT"]; ok {
		b.WriteString(fmt.Sprintf("- CPU limit: %s\n", v))
	}
	if v, ok := ic.Identifiers["MEMORY_LIMIT"]; ok {
		b.WriteString(fmt.Sprintf("- Memory limit: %s\n", v))
	}
	if v, ok := ic.Identifiers["CPU_REQUEST"]; ok {
		b.WriteString(fmt.Sprintf("- CPU request: %s\n", v))
	}
	if v, ok := ic.Identifiers["MEMORY_REQUEST"]; ok {
		b.WriteString(fmt.Sprintf("- Memory request: %s\n", v))
	}
	if v, ok := ic.Identifiers["REPLICA_COUNT"]; ok {
		b.WriteString(fmt.Sprintf("- Replicas: %s\n", v))
	}
	if v, ok := ic.Identifiers["POD_TYPE"]; ok {
		b.WriteString(fmt.Sprintf("- Pod type: %s\n", v))
	}
	if v, ok := ic.Identifiers["OWNER_DEPLOYMENT"]; ok {
		b.WriteString(fmt.Sprintf("- Owner deployment: %s\n", v))
	}
	if v, ok := ic.Identifiers["POD_COUNT"]; ok {
		b.WriteString(fmt.Sprintf("- Pods in namespace: %s\n", v))
	}
	if v, ok := ic.Identifiers["NODE_NAME"]; ok {
		b.WriteString(fmt.Sprintf("- Node: %s\n", v))
	}
	if v, ok := ic.Identifiers["INGRESS_HOST"]; ok {
		b.WriteString(fmt.Sprintf("- Ingress host: %s\n", v))
	}
	if v, ok := ic.Identifiers["INGRESS_CLASS"]; ok {
		b.WriteString(fmt.Sprintf("- Ingress class: %s\n", v))
	}
	if v, ok := ic.Identifiers["STORAGE_CLASS"]; ok {
		b.WriteString(fmt.Sprintf("- Storage class: %s\n", v))
	}
	if v, ok := ic.Identifiers["CAPACITY"]; ok {
		b.WriteString(fmt.Sprintf("- Capacity: %s\n", v))
	}

	return b.String()
}

// buildInstructions returns JSON response format instructions
func buildInstructions(ic *IssueContext) string {
	issueCount := len(ic.Issues)
	return fmt.Sprintf(`
## RESPONSE FORMAT

Respond ONLY with a JSON array of %d guidance objects — one per issue:
[
  {
    "issue": "copy the exact title text from THE ISSUES TO ANALYZE above — no brackets, no numbers, no severity prefix",
    "type": "fix or investigate",
    "root_cause": "max 15 words — specific cause from context above",
    "fix_explanation": "max 15 words — what needs to be done",
    "command": "best kubectl command to fix or investigate",
    "watch_for": "single kubectl command to confirm fix worked",
    "risk": "max 15 words — consequence if not fixed",
    "confidence": "high|medium|low"
  }
]

respond with JSON array only — no markdown, no explanation, no preamble
`, issueCount)
}
