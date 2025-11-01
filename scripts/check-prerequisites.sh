#!/bin/bash
set -e

echo "🔍 Checking prerequisites for Kubernetes cluster installation..."
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

errors=0

# Check Multipass
echo -n "Checking Multipass... "
if command -v multipass &> /dev/null; then
    version=$(multipass version | grep multipass | awk '{print $2}')
    echo -e "${GREEN}✓${NC} (version $version)"
else
    echo -e "${RED}✗ Not found${NC}"
    echo "  Install from: https://multipass.run/"
    ((errors++))
fi

# Check kubectl
echo -n "Checking kubectl... "
if command -v kubectl &> /dev/null; then
    version=$(kubectl version --client -o json 2>/dev/null | grep -o '"gitVersion":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo -e "${GREEN}✓${NC} ($version)"
else
    echo -e "${RED}✗ Not found${NC}"
    echo "  Install from: https://kubernetes.io/docs/tasks/tools/"
    ((errors++))
fi

# Check Go (for go-installer)
echo -n "Checking Go... "
if command -v go &> /dev/null; then
    version=$(go version | awk '{print $3}')
    echo -e "${GREEN}✓${NC} ($version)"
else
    echo -e "${YELLOW}⚠${NC} Not found (required for go-installer)"
    echo "  Install from: https://go.dev/doc/install"
fi

# Check Rust (for rust-installer)
echo -n "Checking Rust... "
if command -v cargo &> /dev/null; then
    version=$(cargo --version | awk '{print $2}')
    echo -e "${GREEN}✓${NC} (cargo $version)"
else
    echo -e "${YELLOW}⚠${NC} Not found (required for rust-installer)"
    echo "  Install from: https://rustup.rs/"
fi

echo ""

# Check system resources
echo "📊 System Resources:"
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    total_mem=$(sysctl -n hw.memsize | awk '{print int($1/1024/1024/1024)}')
    echo "  • Total RAM: ${total_mem}GB"
    if [ $total_mem -lt 12 ]; then
        echo -e "    ${YELLOW}⚠ Warning: At least 12GB RAM recommended${NC}"
    fi
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    total_mem=$(free -g | awk '/^Mem:/{print $2}')
    echo "  • Total RAM: ${total_mem}GB"
    if [ $total_mem -lt 12 ]; then
        echo -e "    ${YELLOW}⚠ Warning: At least 12GB RAM recommended${NC}"
    fi
fi

# Check disk space
if command -v df &> /dev/null; then
    available=$(df -h . | awk 'NR==2 {print $4}')
    echo "  • Available disk space: $available"
fi

echo ""

# Summary
if [ $errors -eq 0 ]; then
    echo -e "${GREEN}✅ All required prerequisites are installed!${NC}"
    echo ""
    echo "You can now proceed with cluster installation:"
    echo "  • DEV cluster (Go):  cd go-installer && go run k8s-installer.go"
    echo "  • PROD cluster (Rust): cd rust-installer && cargo run --release"
    exit 0
else
    echo -e "${RED}❌ Missing $errors required prerequisite(s)${NC}"
    echo "Please install the missing tools and try again."
    exit 1
fi
