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

## Quick Start

### Current Status (Phase 2 Complete)

✅ Configuration system
✅ Platform detection
✅ Deployment abstraction layer

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
├── config/                 # Platform configurations
│   ├── platform-config.yaml
│   └── cluster-inventory.yaml
├── installers/             # Legacy installers (being phased out)
│   ├── k8s-dev/
│   └── k8s-prd/
└── pkg/                    # Go packages
    ├── platform/           # Platform detection
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

# Run all tests
go test ./...
```

## Roadmap

- [x] Phase 1: Configuration & Platform Detection
- [x] Phase 2: Deployment Abstraction Layer
- [ ] Phase 3: Image & Binary Management
- [ ] Phase 4: Platform-Specific Networking
- [ ] Phase 5: Resource Optimization
- [ ] Phase 6: Unified CLI Tool
- [ ] Phase 7: Testing & CI/CD
- [ ] Phase 8: Documentation

## License

MIT
