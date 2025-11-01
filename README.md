# Kubernetes Infrastructure

This repository contains automated Kubernetes cluster installers for both DEV and PROD environments using Multipass VMs.

## Architecture

- **DEV Environment**: Go-based installer (`go-installer/`) - Fast iterations and development
- **PROD Environment**: Rust-based installer (`rust-installer/`) - Optimized performance and stability

Both installers create a 3-node Kubernetes cluster with:
- 1 control plane node
- 2 worker nodes
- Calico CNI
- metrics-server
- Helm 3
- kube-prometheus-stack (Prometheus + Grafana)
- NGINX Ingress Controller

## Quick Start

### DEV Cluster (Go)
```bash
cd go-installer
go run k8s-installer.go
```

### PROD Cluster (Rust)
```bash
cd rust-installer
cargo build --release
./target/release/k8s-installer
```

## Prerequisites

Both installers require:
- [Multipass](https://multipass.run/) installed and running
- kubectl installed locally
- Sufficient system resources (recommend 16GB+ RAM, 50GB+ disk)

## Features

### Parallel Operations
Both installers leverage concurrency for optimal performance:
- Parallel worker node launch
- Parallel worker join operations
- Parallel component installation
- Smart polling instead of fixed sleeps

### Monitoring Stack
- **Prometheus**: Metrics collection and alerting
- **Grafana**: Pre-configured dashboards (28+ built-in)
- **metrics-server**: Resource metrics API

### Access Information
After deployment, access monitoring at:
- Grafana: `http://<control-plane-ip>:30080`
- Prometheus: `http://<control-plane-ip>:30090`
- Default credentials displayed in installer output

## Management

### Common Commands
```bash
# Check cluster status
kubectl get nodes
kubectl get pods -A

# View metrics
kubectl top nodes
kubectl top pods -A

# Shell access
multipass shell <node-name>

# Stop cluster
multipass stop --all

# Start cluster
multipass start --all

# Delete cluster
multipass delete --all --purge
```

## GitOps Integration

These clusters are designed to work with ArgoCD for GitOps-based application deployment. See [k8s-app-manifests](https://github.com/damauer/k8s-app-manifests) repository for application definitions.

## Deployment Times

- **Go (DEV)**: ~3-4 minutes (optimized for fast iterations)
- **Rust (PROD)**: ~10 minutes (includes full stack with monitoring)
