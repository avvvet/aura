package llm

import (
	"fmt"
	"strings"
)

// BuildPrompt builds a dynamic natural language prompt from cluster context
func BuildPrompt(ic *IssueContext) string {
	var b strings.Builder

	b.WriteString("You are a senior Kubernetes engineer with 10+ years of production experience.\n")
	b.WriteString("Analyze these live cluster issues and provide precise, actionable guidance.\n\n")

	// issue list
	b.WriteString("THE ISSUES TO ANALYZE:\n")
	for i, issue := range ic.Issues {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, strings.ToUpper(issue.Severity), issue.Title))
	}
	b.WriteString("\n")

	// dynamic resource description
	b.WriteString("THE AFFECTED RESOURCE:\n")
	b.WriteString(buildResourceDescription(ic))
	b.WriteString("\n")

	// issue descriptions
	b.WriteString("ISSUE CONTEXT:\n")
	for _, issue := range ic.Issues {
		b.WriteString(fmt.Sprintf("Issue '%s':\n", issue.Title))
		b.WriteString(buildIssueDescription(ic, issue.Title))
		b.WriteString("\n")
	}

	// events
	if len(ic.Events) > 0 {
		b.WriteString("\nRECENT EVENTS:\n")
		for _, e := range ic.Events {
			b.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	// node state
	if ic.NodeState != "" {
		b.WriteString(fmt.Sprintf("\nNODE STATE:\n  %s\n", ic.NodeState))
	}

	// logs
	if len(ic.Logs) > 0 {
		b.WriteString("\nCONTAINER LOGS (most recent):\n")
		for _, l := range ic.Logs {
			b.WriteString(fmt.Sprintf("  %s\n", l))
		}
	}

	b.WriteString(buildInstructions(ic))

	return b.String()
}

// buildIssueDescription builds focused description per issue type
func buildIssueDescription(ic *IssueContext, issueTitle string) string {
	name := ic.ResourceName
	ns := ic.ResourceNamespace
	kind := ic.ResourceKind
	image := ic.Identifiers["CURRENT_IMAGE"]
	podType := ic.Identifiers["POD_TYPE"]

	switch issueTitle {
	case "has no resource limits":
		return fmt.Sprintf(
			"  The %s '%s' in namespace '%s' has no CPU or memory limits defined.\n"+
				"  Current CPU limit: %s\n"+
				"  Current memory limit: %s\n"+
				"  Focus ONLY on the missing resource limits — ignore any other issues.",
			kind, name, ns,
			ic.Identifiers["CPU_LIMIT"],
			ic.Identifiers["MEMORY_LIMIT"],
		)

	case "using unpinned image tag":
		if podType == "standalone" {
			return fmt.Sprintf(
				"  Standalone pod '%s' in namespace '%s' uses unpinned image: %s\n"+
					"  IMPORTANT: standalone pod — type MUST be 'fix'\n"+
					"  correct fix: kubectl delete pod %s -n %s\n"+
					"  never use kubectl set image on standalone pods.",
				name, ns, image, name, ns,
			)
		}
		return fmt.Sprintf(
			"  The %s '%s' in namespace '%s' uses unpinned image: %s\n"+
				"  Image base: %s\n"+
				"  Fix: update to pinned version tag.",
			kind, name, ns, image,
			ic.Identifiers["IMAGE_BASE"],
		)

	case "has no network policy":
		return fmt.Sprintf(
			"  Namespace '%s' has no NetworkPolicy resources.\n"+
				"  type MUST be 'investigate' — fix requires creating yaml manually.\n"+
				"  CHECK command: kubectl get networkpolicies -n %s\n"+
				"  never use --all-namespaces.",
			ns, ns,
		)

	case "is unattached and billing":
		return fmt.Sprintf(
			"  PVC '%s' in namespace '%s' is not mounted by any pod.\n"+
				"  Fix: delete if no longer needed.",
			name, ns,
		)

	case "is idle":
		return fmt.Sprintf(
			"  Namespace '%s' has no active workloads.\n"+
				"  Fix: delete if no longer needed.",
			name,
		)

	case "running privileged container":
		return fmt.Sprintf(
			"  Pod '%s' in namespace '%s' runs a privileged container.\n"+
				"  type MUST be 'investigate' — fix requires editing deployment yaml.",
			name, ns,
		)

	case "exposing secrets in env vars":
		return fmt.Sprintf(
			"  Pod '%s' in namespace '%s' has secrets in plain env vars.\n"+
				"  type MUST be 'investigate' — fix requires using secretRef in yaml.",
			name, ns,
		)

	case "has no TLS configured":
		return fmt.Sprintf(
			"  Ingress '%s' in namespace '%s' has no TLS.\n"+
				"  type MUST be 'investigate' — fix requires editing ingress yaml.",
			name, ns,
		)

	default:
		return fmt.Sprintf(
			"  Issue '%s' on %s '%s' in namespace '%s'.",
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

	return b.String()
}

// buildInstructions returns JSON response instructions
func buildInstructions(ic *IssueContext) string {
	issueCount := len(ic.Issues)

	return fmt.Sprintf(`
Respond ONLY with a JSON array of %d guidance objects — one per issue:
[
  {
    "issue": "exact issue title from THE ISSUES list above",
    "type": "fix or investigate",
    "root_cause": "max 15 words — specific cause from context above",
    "fix_explanation": "max 15 words — what needs to be done",
    "command": "single kubectl command using exact values from context above",
    "watch_for": "single kubectl command to confirm fix worked",
    "risk": "max 15 words — consequence if not fixed",
    "confidence": "high|medium|low"
  }
]

DECISION PRINCIPLE:
type "fix"         → command directly resolves the issue, no yaml needed
type "investigate" → fix requires yaml creation or manual file editing

COMMAND PRINCIPLE:
→ use ONLY exact names, namespaces, images from THE AFFECTED RESOURCE above
→ if exact value unknown use <description> as placeholder
→ single line under 100 characters
→ never use heredoc, <<<, <<EOF, --type=json patch
→ never use --grace-period=0 or --force
→ never invent values from training knowledge
→ never suggest :latest as replacement — use <valid-tag> placeholder
→ for resource limits use compact single line:
  kubectl set resources deployment/RESOURCE_NAME --limits=cpu=500m,memory=512Mi --requests=cpu=100m,memory=128Mi -n NAMESPACE

respond with JSON array only — no markdown, no explanation
`, issueCount)
}
