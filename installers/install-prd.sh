#!/bin/bash

set -e

echo "🚀 Installing Kubernetes PROD cluster..."
echo ""

# Change to k8s-prd directory
cd "$(dirname "$0")/k8s-prd"

# Check if Cargo is installed
if ! command -v cargo &> /dev/null; then
    echo "❌ Error: Rust/Cargo is not installed"
    echo "Please install Rust from https://rustup.rs/"
    exit 1
fi

# Build the installer
echo "📦 Building k8s-prd installer..."
cargo build --release

# Run the installer
echo "🎯 Running k8s-prd installer..."
echo ""
./target/release/k8s-prd

echo ""
echo "✅ PROD cluster installation complete!"
echo "Use 'kubectl config use-context k8s-prd' to switch to this cluster"
