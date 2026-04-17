package llm

import (
	"fmt"
	"strings"
)

// BuildPrompt builds a structured prompt for the LLM
func BuildPrompt(ic *IssueContext) string {
	var b strings.Builder

	b.WriteString(`You are a senior Kubernetes engineer with 10+ years of production experience.
You are analyzing a live cluster issue and must provide precise, actionable guidance.
You must respond ONLY with valid JSON. No explanation, no markdown, no preamble.

`)

	b.WriteString(fmt.Sprintf("CLUSTER: %s\n", ic.ClusterName))
	b.WriteString(fmt.Sprintf("RESOURCE: %s/%s (%s)\n", ic.ResourceNamespace, ic.ResourceName, ic.ResourceKind))
	b.WriteString(fmt.Sprintf("ISSUE: %s\n", ic.IssueTitle))
	b.WriteString(fmt.Sprintf("SEVERITY: %s\n", ic.IssueSeverity))

	if ic.NodeState != "" {
		b.WriteString(fmt.Sprintf("\nNODE STATE:\n%s\n", ic.NodeState))
	}

	if len(ic.Events) > 0 {
		b.WriteString("\nKUBERNETES EVENTS:\n")
		for _, e := range ic.Events {
			b.WriteString(fmt.Sprintf("  %s\n", e))
		}
	}

	if len(ic.Logs) > 0 {
		b.WriteString("\nCONTAINER LOGS (last 30 lines):\n")
		for _, l := range ic.Logs {
			b.WriteString(fmt.Sprintf("  %s\n", l))
		}
	}

	b.WriteString(`
Based on the above context, respond with this exact JSON structure:
{
  "root_cause": "maximum 15 words explaining exactly why this is happening",
  "fix_explanation": "plain English explanation of what the engineer needs to do, maximum 15 words",
  "command": "single kubectl command that APPLIES the fix, not a diagnostic command",
  "watch_for": "single kubectl command to confirm fix worked",
  "risk": "maximum 10 words on what happens if not fixed",
  "confidence": "high|medium|low"
}

Rules:
- root_cause must be 15 words or less, specific to the context
- fix_explanation is plain English for non-experts, no kubectl, no jargon
- command must be a SINGLE LINE kubectl command that FIXES the issue
- command must CHANGE something, not just LIST or GET information
- example of WRONG command: kubectl get pods -o json | jq ...
- example of RIGHT command: kubectl set resources deployment/myapp --limits=memory=512Mi -n production
- command must use real resource names and namespaces from the context
- NEVER use heredoc, multi-line, backslash continuation
- risk must be 10 words or less
- confidence: high if cause is clear from logs/events, medium if probable, low if uncertain
- respond with JSON only, nothing else
`)

	return b.String()
}
