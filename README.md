# Kubernetes Infrastructure

Multi-cluster Kubernetes deployment system for DEV and PROD environments using Multipass VMs.

## Overview

This repository provides automated installers for deploying Kubernetes clusters in DEV and PROD environments. Each cluster includes:

- **Kubernetes 1.30** - Latest stable release
- **Calico CNI** - Container networking
- **kube-prometheus-stack** - Monitoring with Prometheus + Grafana
- **Ingress NGINX** - Ingress controller
- **ArgoCD** (DEV only) - GitOps continuous delivery
- **Multi-cluster management** - Merged kubeconfig for easy context switching

## Quick Start

### Prerequisites

- [Multipass](https://multipass.run/) - VM manager
- [kubectl](https://kubernetes.io/docs/tasks/tools/) - Kubernetes CLI
- [Go 1.21+](https://go.dev/doc/install) - For DEV cluster
- [Rust](https://rustup.rs/) - For PROD cluster

### Install DEV Cluster

```bash
cd installers
./install-dev.sh
```

The DEV cluster will be deployed with these nodes:
- `k8s-dev-c1` - Control plane
- `k8s-dev-w1` - Worker 1
- `k8s-dev-w2` - Worker 2

### Install PROD Cluster

```bash
cd installers
./install-prd.sh
```

The PROD cluster will be deployed with these nodes:
- `k8s-prd-c1` - Control plane
- `k8s-prd-w1` - Worker 1
- `k8s-prd-w2` - Worker 2

## Multi-Cluster Management

Both clusters are automatically added to your `~/.kube/config` with distinct context names.

### Switch between clusters

```bash
# Switch to DEV
kubectl config use-context k8s-dev

# Switch to PROD
kubectl config use-context k8s-prd

# View all contexts
kubectl config get-contexts

# Check current context
kubectl config current-context
```

## Accessing Services

### Monitoring (Both Clusters)

```bash
# Get Grafana password (DEV)
kubectl --context k8s-dev get secret -n monitoring kube-prometheus-stack-grafana \
  -o jsonpath='{.data.admin-password}' | base64 -d

# Get Grafana password (PROD)
kubectl --context k8s-prd get secret -n monitoring kube-prometheus-stack-grafana \
  -o jsonpath='{.data.admin-password}' | base64 -d

# Access Grafana: http://<control-plane-ip>:30080
# Username: admin
```

### ArgoCD (DEV Only)

```bash
# Get ArgoCD password
kubectl --context k8s-dev get secret -n argocd argocd-initial-admin-secret \
  -o jsonpath='{.data.password}' | base64 -d

# Access ArgoCD: http://<control-plane-ip>:30443
# Username: admin
```

## Common Operations

### Cluster Management

```bash
# Check nodes
kubectl --context k8s-dev get nodes
kubectl --context k8s-prd get nodes

# View all pods
kubectl --context k8s-dev get pods -A
kubectl --context k8s-prd get pods -A

# SSH into nodes
multipass shell k8s-dev-c1
multipass shell k8s-prd-c1

# Stop clusters
multipass stop k8s-dev-c1 k8s-dev-w1 k8s-dev-w2
multipass stop k8s-prd-c1 k8s-prd-w1 k8s-prd-w2

# Delete all clusters
cd installers
./cleanup.sh
```

## Directory Structure

```
k8s-infrastructure/
├── installers/
│   ├── k8s-dev/           # DEV cluster installer (Go)
│   │   ├── main.go
│   │   └── go.mod
│   ├── k8s-prd/           # PROD cluster installer (Rust)
│   │   ├── src/main.rs
│   │   ├── Cargo.toml
│   │   └── Cargo.lock
│   ├── install-dev.sh
│   ├── install-prd.sh
│   └── cleanup.sh
├── argocd/                # ArgoCD configurations
└── README.md
```

## Troubleshooting

### Clean up and retry

```bash
cd installers
./cleanup.sh
./install-dev.sh  # or ./install-prd.sh
```

### Check component status

```bash
# Check Calico
kubectl get pods -n calico-system

# Check monitoring
kubectl get pods -n monitoring

# Check ArgoCD (DEV)
kubectl --context k8s-dev get pods -n argocd
```
