# Kubernetes Infrastructure

Cross-platform Kubernetes cluster deployment tool supporting:
- **macOS** (M2 MacBook Pro, Intel) - Multipass VMs
- **WSL2** (Windows 11, AMD64/ARM64) - Multipass VMs
- **Raspberry Pi 5** (ARM64) - Bare metal

## Features

- 🔍 Automatic platform detection
- ⚙️ YAML-based configuration
- 🚀 Multi-environment support (dev, prd)
- 📦 GitOps-ready with ArgoCD
- 🌐 Gateway API + Legacy Ingress support
- 📊 Integrated monitoring (Prometheus/Grafana)
- 🎯 Intelligent resource optimization based on platform capabilities
- 📏 Workload profiles for development, testing, and production

## Quick Start

### Current Status (Phase 5 Complete)

✅ Configuration system
✅ Platform detection
✅ Deployment abstraction layer
✅ Image & binary management
✅ Platform-specific networking
✅ Resource optimization
✅ Unified CLI tool

### Prerequisites

**All Platforms:**
- kubectl
- Git

**macOS:**
- Multipass

**WSL2:**
- Multipass for Windows

**Raspberry Pi 5:**
- Ubuntu 24.04 LTS
- SSH access

### CLI Tool

Build the CLI tool:
```bash
go build -o bin/k8s-deploy ./cmd/k8s-deploy/
```

**Basic Usage:**
```bash
# Create a dev cluster with auto-optimized resources (recommended)
./bin/k8s-deploy create --env dev

# Create a production cluster with optimized resources
./bin/k8s-deploy create --env prd --profile production

# Create a cluster with custom settings
./bin/k8s-deploy create --env dev --nodes 3 --cpus 2 --memory 4G

# List all nodes
./bin/k8s-deploy list

# Check cluster status
./bin/k8s-deploy status --env dev

# Destroy a cluster
./bin/k8s-deploy destroy --env dev
```

**Resource Optimization:**

The CLI automatically calculates optimal resource allocations based on:
- Platform capabilities (CPU, memory, max nodes)
- Workload profile (development, testing, production)
- Available system resources after OS reservation

```bash
# Let the tool optimize everything (recommended)
./bin/k8s-deploy create --env dev

# Use production profile for larger allocations
./bin/k8s-deploy create --env prd --profile production

# Override specific resources
./bin/k8s-deploy create --env dev --cpus 4 --memory 8G
```

**Available Commands:**
- `create` - Create a new Kubernetes cluster
- `destroy` - Destroy an existing cluster
- `list` - List all nodes across clusters
- `status` - Show cluster status and health
- `version` - Show version information
- `help` - Show help message

**Create Flags:**
- `--env` - Environment (dev, prd) [default: dev]
- `--platform` - Platform override [default: auto]
- `--k8s-version` - Kubernetes version [default: 1.30.0]
- `--cni` - CNI type (calico, flannel) [default: calico]
- `--cni-version` - CNI version [default: 3.28.0]
- `--nodes` - Number of worker nodes [default: 3]
- `--cpus` - CPUs per node [default: 2]
- `--memory` - Memory per node [default: 2G]
- `--disk` - Disk per node [default: 20G]

### Configuration

See `config/platform-config.yaml` for platform-specific settings.
See `config/cluster-inventory.yaml` for bare metal node inventory (Pi clusters).

## Platform Detection

The system automatically detects your platform:

```go
import "k8s-infrastructure/pkg/platform"

p, _ := platform.Detect()
fmt.Printf("Platform: %s\n", p.String())
// Output: darwin/arm64, deploy=multipass
```

### Supported Platforms

| Platform | OS | Arch | Deploy Method |
|----------|--------|--------|---------------|
| macOS M2 | darwin | arm64 | multipass |
| macOS Intel | darwin | amd64 | multipass |
| WSL2 AMD64 | linux | amd64 | multipass |
| WSL2 ARM64 | linux | arm64 | multipass |
| Raspberry Pi 5 | linux | arm64 | native |

## Project Structure

```
k8s-infrastructure/
├── argocd/                 # ArgoCD applications & manifests
│   ├── nginx/              # Example nginx deployment
│   └── nginx-gateway/      # Gateway API fabric
├── cmd/                    # Command-line tools
│   └── k8s-deploy/         # Main CLI tool
├── config/                 # Platform configurations
│   ├── platform-config.yaml
│   └── cluster-inventory.yaml
├── installers/             # Legacy installers (being phased out)
│   ├── k8s-dev/
│   └── k8s-prd/
└── pkg/                    # Go packages
    ├── binaries/           # Binary management
    ├── config/             # Configuration loading
    ├── images/             # Container image management
    ├── network/            # Network configuration
    ├── platform/           # Platform detection
    ├── resources/          # Resource optimization & calculation
    └── provider/           # Deployment providers
        ├── multipass/      # Multipass VM provider
        └── native/         # Bare metal provider
```

## Development

### Testing

```bash
# Test platform detection
go test ./pkg/platform/...

# Test provider implementations
go test ./pkg/provider/...

# Test image and binary management
go test ./pkg/images/... ./pkg/binaries/...

# Test network configuration
go test ./pkg/network/...

# Test resource optimization
go test ./pkg/resources/...

# Run all tests
go test ./...
```

## Roadmap

- [x] Phase 1: Configuration & Platform Detection
- [x] Phase 2: Deployment Abstraction Layer
- [x] Phase 3: Image & Binary Management
- [x] Phase 4: Platform-Specific Networking
- [x] Phase 5: Resource Optimization
- [x] Phase 6: Unified CLI Tool
- [ ] Phase 7: Testing & CI/CD
- [ ] Phase 8: Documentation

## License

MIT
