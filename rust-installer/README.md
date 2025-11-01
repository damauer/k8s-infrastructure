# Rust Kubernetes Installer (PROD Environment)

Production-grade Kubernetes cluster installer optimized for stability and performance.

## Features

- **High-performance concurrency**: Rayon and crossbeam for parallel operations
- **Smart polling**: Efficient 2-second polling with minimal overhead
- **Complete stack**: Full monitoring, ingress, and Helm
- **Robust deployment**: ~10 minutes for fully validated production cluster

## Requirements

- Rust 1.70+ installed
- Multipass installed and running
- kubectl installed
- 16GB+ RAM available
- 40GB+ disk space

## Usage

### Development Build
```bash
cargo run
```

### Production Build
```bash
cargo build --release
./target/release/k8s-installer
```

### With Timing
```bash
time cargo run --release
```

## What Gets Installed

### Cluster Configuration
- **Kubernetes**: v1.30.14
- **CNI**: Calico v3.28.0
- **Container Runtime**: containerd
- **Pod Network CIDR**: 10.244.0.0/16

### Nodes
- 1x Control Plane: `rust-k8s-control-plane` (4 CPU, 8GB RAM)
- 2x Workers: `rust-k8s-worker-1`, `rust-k8s-worker-2` (4 CPU, 4GB RAM each)

### Components
- **metrics-server**: Resource metrics API
- **Helm 3**: Package manager
- **kube-prometheus-stack**:
  - Prometheus (metrics collection)
  - Grafana (visualization with 28+ pre-configured dashboards)
  - Alertmanager (alerting)
  - Node Exporter (node metrics)
  - kube-state-metrics (Kubernetes metrics)
- **NGINX Ingress Controller**: v1.9.4

## Post-Installation

After successful installation, you'll receive:
- Kubeconfig automatically saved to `~/.kube/config`
- Previous kubeconfig backed up with timestamp
- Grafana URL and auto-generated password
- Prometheus URL
- Complete cluster status

### Access Monitoring
```bash
# Grafana (default: admin / <displayed-password>)
http://<control-plane-ip>:30080

# Prometheus
http://<control-plane-ip>:30090
```

### Pre-configured Grafana Dashboards
The kube-prometheus-stack includes 28 built-in dashboards:
- Kubernetes / Compute Resources / Cluster
- Kubernetes / Networking / Cluster
- Kubernetes / API Server
- Node Exporter / Nodes
- Persistent Volumes
- StatefulSets
- And many more...

### Verify Installation
```bash
kubectl get nodes
kubectl get pods -A
kubectl top nodes
kubectl top pods -A
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
multipass shell rust-k8s-control-plane
multipass shell rust-k8s-worker-1
multipass shell rust-k8s-worker-2
```

## Optimization Features

### Parallel Worker Launch (Rayon)
Workers are launched concurrently using rayon's parallel iterators, reducing deployment time significantly.

### Smart Polling
Instead of fixed sleeps, checks actual Kubernetes node status every 2 seconds and continues as soon as ready.

### Parallel Component Installation (Crossbeam)
metrics-server, Helm, and Ingress NGINX install concurrently using scoped threads with guaranteed lifetime safety.

### Thread-Safe Error Handling
Robust error propagation across parallel operations with detailed error messages.

## Configuration

Edit the `ClusterConfig` struct in `src/main.rs` to customize:
- Node names and resources
- Kubernetes version
- Pod network CIDR
- Calico version
- Worker count

## Troubleshooting

### Build Issues
```bash
# Clean and rebuild
cargo clean
cargo build --release

# Check Rust version
rustc --version  # Should be 1.70+
```

### Multipass Issues
```bash
# Check Multipass status
multipass list

# View VM logs
multipass exec <node-name> -- journalctl -xeu kubelet

# Restart Multipass daemon (macOS)
sudo launchctl stop com.canonical.multipassd
sudo launchctl start com.canonical.multipassd
```

### Kubernetes Issues
```bash
# Check cluster status
kubectl cluster-info
kubectl get componentstatuses

# Check Calico
kubectl get pods -n kube-system -l k8s-app=calico-node

# Check monitoring stack
kubectl get pods -n monitoring
kubectl get svc -n monitoring
```

### Monitoring Issues
```bash
# Check Prometheus
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090

# Check Grafana
kubectl port-forward -n monitoring svc/grafana 3000:80

# Get Grafana password again
kubectl get secret -n monitoring grafana -o jsonpath="{.data.admin-password}" | base64 --decode
```

## Performance

Typical deployment timeline:
- Prerequisites check: 1s
- Cloud-init files: 1s
- Control plane launch: ~66s
- Cluster initialization: ~30s
- Calico CNI install: ~30s
- Workers launch (parallel): ~67s
- Workers join (parallel): instant
- Nodes ready: ~19s
- Components install (parallel): ~30s
- Prometheus stack: ~90s
- Service exposure: ~1s

**Total**: ~10 minutes 20 seconds

## Dependencies

Defined in `Cargo.toml`:
- `rayon`: Data parallelism
- `crossbeam`: Scoped threads
- `tokio`: Async runtime
- `serde`, `serde_json`: Configuration parsing
- `regex`: Output parsing
- `colored`: Terminal colors
- `chrono`: Timestamps

## GitOps Integration

This PROD cluster is managed by ArgoCD running on the DEV cluster. Applications are defined in the [k8s-app-manifests](https://github.com/damauer/k8s-app-manifests) repository and deployed via GitOps workflow.

## Production Readiness

This installer is designed for production use with:
- Comprehensive error handling
- Automatic retries for transient failures
- Smart polling to minimize unnecessary waits
- Complete monitoring stack out of the box
- Backup of existing kubeconfig
- Detailed status reporting
