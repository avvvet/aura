package llm

import (
	"fmt"
	"strings"
)

func BuildPrompt(ic *IssueContext) string {
	var b strings.Builder

	b.WriteString(`You are a senior Kubernetes engineer with 10+ years of production experience.
You are analyzing a live cluster issue and must provide precise, actionable guidance.
You must respond ONLY with valid JSON. No explanation, no markdown, no preamble.

`)

	b.WriteString(fmt.Sprintf("CLUSTER: %s\n", ic.ClusterName))
	b.WriteString(fmt.Sprintf("ISSUE: %s\n", ic.IssueTitle))
	b.WriteString(fmt.Sprintf("SEVERITY: %s\n\n", ic.IssueSeverity))

	if len(ic.Identifiers) > 0 {
		b.WriteString("=== RESOURCE IDENTIFIERS — USE ONLY THESE EXACT VALUES IN YOUR COMMAND ===\n")
		for k, v := range ic.Identifiers {
			b.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
		b.WriteString("=========================================================================\n\n")
	}

	if len(ic.Events) > 0 {
		b.WriteString("=== CLUSTER CONTEXT ===\n")
		for _, e := range ic.Events {
			b.WriteString(fmt.Sprintf("  %s\n", e))
		}
		b.WriteString("=======================\n\n")
	}

	if ic.NodeState != "" {
		b.WriteString(fmt.Sprintf("NODE STATE: %s\n\n", ic.NodeState))
	}

	if len(ic.Logs) > 0 {
		b.WriteString("=== CONTAINER LOGS (last 20 lines) ===\n")
		for _, l := range ic.Logs {
			b.WriteString(fmt.Sprintf("  %s\n", l))
		}
		b.WriteString("======================================\n\n")
	}

	b.WriteString(`=== INSTRUCTIONS ===
Respond with this exact JSON:
{
  "type": "fix or investigate",
  "root_cause": "max 15 words specific to context above",
  "fix_explanation": "max 15 words — for fix: what command does / for investigate: what contractor should look for",
  "command": "single kubectl command max 100 chars",
  "watch_for": "single kubectl command to confirm fix worked",
  "risk": "max 15 words — consequence if not fixed",
  "confidence": "high|medium|low"
}

TYPE DECISION:
- "fix"         → command directly resolves the issue
- "investigate" → fix requires manual edit or more context

COMMAND RULES — CRITICAL:
- use ONLY values from RESOURCE IDENTIFIERS section above
- NEVER use values from your training knowledge or assumptions
- NEVER invent image names, registry URLs, versions, tags or resource names
- IMAGE_BASE in IDENTIFIERS is the authoritative image — use it EXACTLY as shown
- NAMESPACE in IDENTIFIERS is the authoritative namespace — use it EXACTLY
- RESOURCE_NAME in IDENTIFIERS is the authoritative resource name — use it EXACTLY
- CONTAINER_NAME in IDENTIFIERS is the authoritative container name — use it EXACTLY
- command must be a single line under 100 characters
- prefer: kubectl set, kubectl scale, kubectl rollout, kubectl delete
- NEVER use: kubectl patch --type=json, heredoc, <<<, <<EOF, multi-line, backslash continuation

SPECIFIC RULES:

- resource limits:
    type: fix
    kubectl set resources deployment/RESOURCE_NAME --limits=cpu=500m,memory=512Mi --requests=cpu=100m,memory=128Mi -n NAMESPACE
    replace RESOURCE_NAME and NAMESPACE with EXACT values from IDENTIFIERS

- unpinned image on standalone pod (POD_TYPE=standalone in IDENTIFIERS):
    type: investigate
    CHECK: kubectl describe pod RESOURCE_NAME -n NAMESPACE
    fix_explanation: standalone pod must be deleted and recreated with pinned image tag
    NEVER use kubectl set image for standalone pods

- unpinned image on managed pod (POD_TYPE=managed in IDENTIFIERS):
    type: fix
    kubectl set image deployment/RESOURCE_NAME CONTAINER_NAME=IMAGE_BASE:<pinned-version> -n NAMESPACE
    IMAGE_BASE must be taken EXACTLY from IDENTIFIERS — never use a different image
    CONTAINER_NAME must be taken EXACTLY from IDENTIFIERS
    RESOURCE_NAME must be taken EXACTLY from IDENTIFIERS
    NAMESPACE must be taken EXACTLY from IDENTIFIERS

- scale:
    type: fix
    kubectl scale deployment/RESOURCE_NAME --replicas=<count> -n NAMESPACE

- rollback:
    type: fix
    kubectl rollout undo deployment/RESOURCE_NAME -n NAMESPACE

- network policy missing:
    type: investigate — ALWAYS, never generate yaml inline
    CHECK command: kubectl get networkpolicies -n NAMESPACE
    use NAMESPACE from IDENTIFIERS above — never use --all-namespaces
    fix_explanation: describe what NetworkPolicy yaml contractor needs to create

- TLS missing, secrets in env, privileged container, security context:
    type: investigate — ALWAYS
    give best investigation kubectl command using values from IDENTIFIERS
    explain what contractor needs to manually fix
	
- root_cause must reference ONLY values from RESOURCE IDENTIFIERS
  use CURRENT_IMAGE value when describing image issues
  never use image names from your training knowledge in root_cause

CONFIDENCE:
- high   → root cause confirmed by events or logs
- medium → root cause probable but not fully confirmed
- low    → root cause unclear, always use investigate type

respond with JSON only, nothing else
===`)

	return b.String()
}
