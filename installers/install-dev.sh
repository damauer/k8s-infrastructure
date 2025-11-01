#!/bin/bash

set -e

echo "🚀 Installing Kubernetes DEV cluster..."
echo ""

# Change to k8s-dev directory
cd "$(dirname "$0")/k8s-dev"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    echo "Please install Go from https://go.dev/doc/install"
    exit 1
fi

# Build the installer
echo "📦 Building k8s-dev installer..."
go build -o k8s-dev main.go

# Run the installer
echo "🎯 Running k8s-dev installer..."
echo ""
./k8s-dev

echo ""
echo "✅ DEV cluster installation complete!"
echo "Use 'kubectl config use-context k8s-dev' to switch to this cluster"
