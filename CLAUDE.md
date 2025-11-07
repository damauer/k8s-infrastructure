# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Cross-platform Kubernetes cluster deployment tool supporting macOS (Multipass VMs), WSL2 (Multipass VMs), and Raspberry Pi 5 (bare metal). The system automatically detects platform capabilities and optimizes resource allocation based on workload profiles.

## Building and Running

### Unified CLI (In Development)
```bash
# Build the new CLI
go build -o bin/k8s-deploy ./cmd/k8s-deploy/

# Create dev cluster with auto-optimized resources
./bin/k8s-deploy create --env dev

# Create production cluster
./bin/k8s-deploy create --env prd --profile production

# List all nodes
./bin/k8s-deploy list

# Check cluster status
./bin/k8s-deploy status --env dev

# Destroy cluster
./bin/k8s-deploy destroy --env dev
```

### Legacy Installers (Currently Active)

**k8s-dev (Go)** - Located in `installers/k8s-dev/`:
```bash
cd installers/k8s-dev
go build -o k8s-dev .
./k8s-dev
```

**k8s-prd (Rust)** - Located in `installers/k8s-prd/`:
```bash
cd installers/k8s-prd
cargo build --release
./target/release/k8s-prd
```

**Note**: The legacy installers remain the stable implementation until the unified CLI is fully validated.

### Testing
```bash
# Run all tests
go test ./...

# Test specific packages
go test ./pkg/platform/...
go test ./pkg/provider/...
go test ./pkg/resources/...
```

## Architecture

### Core Abstraction Pattern

The codebase follows a **provider pattern** to abstract deployment across different platforms:

1. **Platform Detection** (`pkg/platform/`) - Automatically detects OS, architecture, and deployment method (multipass vs native)
2. **Provider Interface** (`pkg/provider/interface.go`) - Defines contract that all deployment providers must implement
3. **Provider Implementations**:
   - `pkg/provider/multipass/` - VM-based deployment for macOS and WSL2
   - `pkg/provider/native/` - Bare metal deployment for Raspberry Pi

### Configuration System

Configuration is **two-layered**:

- **Platform Config** (`config/platform-config.yaml`) - Platform-specific defaults (CPUs, memory, deployment method)
- **Environment Config** (same file, `environments` section) - Environment-specific settings (dev/prd) for pod/service CIDRs, node counts, context names

The `pkg/config/config.go` loader merges these based on detected platform key (e.g., `darwin-arm64`, `linux-arm64-pi`, `linux-amd64-wsl2`).

### Resource Optimization

The **resource calculator** (`pkg/resources/calculator.go`) is central to the system's intelligence:

- Queries platform capabilities (total CPU/memory, OS reservations, max nodes)
- Calculates optimal allocations based on **workload profile** (development/testing/production)
- Validates requested resources against platform limits
- Uses allocation ratios: development (30% control plane, 70% workers), production (20% control plane, 80% workers)

**Key insight**: The calculator accounts for OS overhead (e.g., 2 cores + 4GB reserved on macOS M2, 0.5 core + 1GB on Pi).

### Cloud-Init Integration

Node provisioning uses **cloud-init templates** (`pkg/cloudinit/`):

- Separate templates for control-plane and worker roles
- Configured with Kubernetes version, pod/service CIDRs
- Generated dynamically and passed to providers via `NodeConfig.CloudInitPath`
- Templates handle containerd installation, kubelet setup, and system configuration

### Add-ons Management

The `pkg/addons/addons.go` manager orchestrates installation of cluster components:

- **Phase 1**: Parallel installation of metrics-server, Helm, and Ingress NGINX
- **Phase 2**: Prometheus Stack (requires Helm from Phase 1)
- **Phase 3**: Parallel exposure of Grafana (30080) and Prometheus (30090)
- **Phase 4**: ArgoCD installation and exposure (30081 HTTP, 30443 HTTPS)

Add-ons execute commands via provider abstraction (multipass exec vs native bash).

### Network Configuration

The `pkg/network/manager.go` handles platform-specific networking:

- Returns environment-specific pod/service CIDRs from config
- Calculates DNS service IP (10th IP in service CIDR)
- Sets MTU based on deployment method (1450 for multipass VMs to account for encapsulation, 1500 for native)
- Validates CIDR formats and IP calculations

## Key Files

- `cmd/k8s-deploy/main.go` - Unified CLI entry point (in development)
- `installers/k8s-dev/` - Legacy Go-based dev cluster installer (currently active)
- `installers/k8s-prd/` - Legacy Rust-based production cluster installer (currently active)
- `pkg/provider/interface.go` - Provider contract (CreateNode, DeployCluster, etc.)
- `pkg/resources/calculator.go` - Resource optimization logic
- `pkg/platform/detector.go` - Platform detection (WSL, Pi, macOS)
- `pkg/addons/addons.go` - Cluster add-on orchestration
- `config/platform-config.yaml` - Platform and environment configuration

## Important Patterns

### Platform Keys
Platform detection builds keys like `darwin-arm64`, `linux-arm64-pi`, `linux-arm64-wsl2` which map to configuration sections. The key construction logic in `platform.buildKey()` determines which config gets loaded.

### Provider Initialization
Providers are initialized with platform-specific config maps from `config.GetPlatformConfig(platform.String())`. Each provider extracts relevant fields (e.g., multipass_path for WSL2, ssh_key_path for Pi).

### Resource Validation Flow
1. User specifies or requests auto-sized resources
2. Calculator generates recommendations based on profile + platform capabilities
3. CLI rounds fractional CPUs for Multipass (requires integers)
4. Validation checks total resources don't exceed available (after OS reservation)

### Command Execution Pattern
Add-on manager wraps commands differently per provider:
- Multipass: `multipass exec <node> -- bash -c "export KUBECONFIG=/etc/kubernetes/admin.conf; <cmd>"`
- Native: `bash -c "<cmd>"`

## ArgoCD Integration

The `argocd/` directory contains GitOps application manifests:
- `nginx/` - Kustomize-based nginx deployment with dev/prd overlays
- `nginx-gateway/` - Gateway API fabric configuration
- `test-nginx-app/` - Test application with ArgoCD application definition

Applications reference this repo's GitHub URL and are designed for GitOps workflows.

## Module Structure

Root Go module is `k8s-infrastructure` (see root `go.mod`). The legacy `installers/k8s-dev/` has its own `go.mod` as it operates independently until migration to unified CLI is complete.
