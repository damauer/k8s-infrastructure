# Go Kubernetes Installer (DEV Environment)

Fast Kubernetes cluster installer optimized for development and rapid iterations.

## Features

- **Parallel operations**: Goroutines for concurrent worker launch and component installation
- **Smart polling**: 2-second polling intervals instead of fixed sleeps
- **Full stack**: Kubernetes 1.30 + Calico + monitoring + ingress
- **Fast deployment**: ~3-4 minutes for complete cluster

## Requirements

- Go 1.21+ installed
- Multipass installed and running
- kubectl installed
- 12GB+ RAM available
- 30GB+ disk space

## Usage

### Quick Start
```bash
go run k8s-installer.go
```

### Build and Run
```bash
go build -o k8s-installer k8s-installer.go
./k8s-installer
```

## What Gets Installed

### Cluster Configuration
- **Kubernetes**: v1.30.14
- **CNI**: Calico v3.28.0
- **Container Runtime**: containerd
- **Pod Network CIDR**: 10.244.0.0/16

### Nodes
- 1x Control Plane: `k8s-control-plane` (4 CPU, 8GB RAM)
- 2x Workers: `k8s-worker-1`, `k8s-worker-2` (4 CPU, 4GB RAM each)

### Components
- **metrics-server**: Resource metrics API
- **Helm 3**: Package manager
- **kube-prometheus-stack**: Prometheus + Grafana + Alertmanager
- **NGINX Ingress Controller**: v1.9.4

## Post-Installation

After successful installation, you'll receive:
- Kubeconfig automatically saved to `~/.kube/config`
- Grafana URL and credentials
- Prometheus URL
- Cluster status summary

### Access Monitoring
```bash
# Grafana (default: admin / <displayed-password>)
http://<control-plane-ip>:30080

# Prometheus
http://<control-plane-ip>:30090
```

### Verify Installation
```bash
kubectl get nodes
kubectl get pods -A
kubectl top nodes
```

## Management

### Stop Cluster
```bash
multipass stop --all
```

### Start Cluster
```bash
multipass start --all
```

### Delete Cluster
```bash
multipass delete --all --purge
```

### Shell Access
```bash
multipass shell k8s-control-plane
multipass shell k8s-worker-1
multipass shell k8s-worker-2
```

## Optimization Features

### Parallel Worker Launch
Workers are launched concurrently using goroutines, reducing deployment time by ~50%.

### Smart Polling
Instead of fixed 30-second sleeps, the installer checks node readiness every 2 seconds, completing as soon as nodes are ready.

### Parallel Component Installation
metrics-server, Helm, and Ingress NGINX are installed concurrently, saving ~2-3 minutes.

## Configuration

Edit the `ClusterConfig` struct in `k8s-installer.go` to customize:
- Node names and resources
- Kubernetes version
- Pod network CIDR
- Worker count

## Troubleshooting

### Multipass Issues
```bash
# Check Multipass status
multipass list

# View logs
multipass exec <node-name> -- journalctl -xeu kubelet
```

### Network Issues
```bash
# Check Calico pods
kubectl get pods -n kube-system -l k8s-app=calico-node

# Check pod network
kubectl run test --image=nginx
kubectl exec test -- curl kubernetes.default
```

### Monitoring Issues
```bash
# Check Prometheus stack
kubectl get pods -n monitoring

# Check Grafana service
kubectl get svc -n monitoring grafana
```

## GitOps Integration

This DEV cluster hosts ArgoCD for managing applications in both DEV and PROD environments. See the [k8s-app-manifests](https://github.com/damauer/k8s-app-manifests) repository for application definitions.
