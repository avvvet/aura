## what is aura?

You just joined a team, landed a contract, inherited an unknown kubernetes cluster, or need to troubleshoot fast. One command. Zero setup. aura watches your cluster live, thinks like a senior Kubernetes engineer, tells you exactly what to fix, and confirms when you fixed it.


# aura

> the light that guides you through darkness.

`aura` is a live Kubernetes cluster intelligence tool for engineers who need the complete picture fast. No agents, no SaaS, no cloud dependency. Just run `aura` and everything becomes clear.

---

https://github.com/user-attachments/assets/df070b65-13b9-4b96-80ae-914ddaff48ed

---

## how it works

```
run aura
  ↓
live terminal UI opens — probes cluster every 30s
  ↓
health grid        → complete cluster snapshot at a glance
must fix           → critical issues with exact kubectl commands
good practice      → recommendations to improve stability and cost
security           → misconfigurations and vulnerabilities detected
  ↓
press 'a'          → AI analysis view
  ↓
WHY    → root cause in plain English
ACTION → what to do, explained simply
RUN    → exact kubectl command to fix
RISK   → what happens if not fixed
  ↓
press 'esc'        → back to live view
  ↓
aura confirms when issue is resolved  ✓
```

---

## features

- live terminal UI — probes cluster every 30 seconds
- complete cluster snapshot — nodes, namespaces, pods, deployments, services, ingresses, pvcs
- health percentage — real time cluster health score
- must fix — critical issues with exact kubectl commands
- good practice — cost and stability recommendations
- security audit — privileged containers, missing network policies, exposed secrets, unpinned images
- cost signals — idle namespaces, unattached pvcs, missing resource limits
- ai powered analysis — press 'a' for guided fix with WHY, ACTION, RUN, RISK
- supports openai, anthropic, and local ollama
- air-gapped friendly — local ollama means zero data leaves your machine
- zero cluster footprint — no agent, no install, no SaaS
- works on any cluster — eks, gke, aks, kubeadm, k3s, minikube, orbstack

---

## install

**linux / mac**
```bash
curl -fsSL https://raw.githubusercontent.com/avvvet/aura/main/install.sh | sudo sh
```

**go install**
```bash
go install github.com/avvvet/aura@latest
```

---

## usage

```bash
aura                        # start live cluster monitoring
aura --context staging      # target different cluster
aura --kubeconfig ~/my.cfg  # explicit kubeconfig
aura --setup                # configure LLM provider
aura --clear                # clear saved config and API keys
```

**inside aura**
```
'a'      → open AI analysis view
'esc'    → return to main view
ctrl+c   → exit
```

---

## llm setup

on first run aura will ask you to configure a language model:

```
[1] ollama      local, free, private, recommended
[2] openai      best quality, requires API key
[3] anthropic   best reasoning, requires API key
[4] skip        snapshot only, configure later
```

API keys are stored locally with a 24 hour expiry and never leave your machine.

reconfigure anytime:
```bash
aura --setup
```

---

## how it connects

aura follows the standard kubeconfig precedence — exactly like kubectl:

```
1. --kubeconfig flag    explicit override
2. KUBECONFIG env var   ci/cd systems
3. ~/.kube/config       default fallback
```

no configuration needed. if kubectl works, aura works.

aura talks directly to the Kubernetes API server using client-go — the same library kubectl uses. it is not a kubectl wrapper.

---

## why aura?

| | aura | kubecost | cast.ai | k9s |
|---|---|---|---|---|
| install | single binary | helm chart | saas agent | binary |
| live monitoring | yes | no | yes | yes |
| ai guided fix | yes | no | no | no |
| data privacy | local only | cloud | cloud | local |
| air-gapped | yes | no | no | yes |
| security audit | yes | no | no | no |
| cost signals | yes | yes | yes | no |
| open source | yes | partial | no | yes |
| zero footprint | yes | no | no | yes |

---

## author

built by [@avvvet](https://github.com/avvvet) — Senior Golang Engineer & CKA Certified

> your existence alone reveals all.

---

## license

MIT