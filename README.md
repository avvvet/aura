# aura

> the light that pushes darkness.

`aura` is a lightweight Kubernetes cluster intelligence tool for engineers who need the complete picture fast. No agents, no SaaS, no cloud dependency. Just run `aura` and everything becomes clear.

---

## what is aura?

You just joined a team, landed a contract, inherited an unknown cluster, or need to troubleshoot fast. One command. Zero setup. aura tells you everything you need to know.

![aura screenshot](docs/screenshot.png)

---

## features

- complete cluster snapshot in one command
- nodes, namespaces, pods, deployments, services, ingresses, pvcs
- health status at a glance
- cost signals — idle namespaces, unattached pvcs, missing resource limits
- ai powered analysis via openai, anthropic or local ollama
- zero cluster footprint — no agent, no install, no SaaS
- works on any cluster — eks, gke, aks, kubeadm, k3s, minikube
- air-gapped friendly — data never leaves your machine

---

## install

```bash
curl -L https://github.com/avvvet/aura/releases/latest/download/aura-linux-amd64 -o /usr/local/bin/aura
chmod +x /usr/local/bin/aura
```

---

## usage

```bash
aura                        # full cluster snapshot
aura --namespace production # scoped to namespace
aura --context staging      # different cluster
aura --output json          # machine readable
aura --analyze              # ai powered analysis
aura --help                 # all options
```

---

## how it connects

aura follows the standard kubeconfig precedence — exactly like kubectl: