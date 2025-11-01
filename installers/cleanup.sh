#!/bin/bash

echo "🧹 Cleaning up Kubernetes clusters..."
echo ""

# List of all possible nodes
DEV_NODES="k8s-dev-c1 k8s-dev-w1 k8s-dev-w2"
PRD_NODES="k8s-prd-c1 k8s-prd-w1 k8s-prd-w2"

# Function to check if a VM exists
vm_exists() {
    multipass list | grep -q "^$1 "
}

# Delete DEV cluster nodes
echo "Checking for DEV cluster nodes..."
for node in $DEV_NODES; do
    if vm_exists "$node"; then
        echo "  Deleting $node..."
        multipass delete "$node"
    else
        echo "  $node not found, skipping..."
    fi
done

# Delete PROD cluster nodes
echo ""
echo "Checking for PROD cluster nodes..."
for node in $PRD_NODES; do
    if vm_exists "$node"; then
        echo "  Deleting $node..."
        multipass delete "$node"
    else
        echo "  $node not found, skipping..."
    fi
done

# Purge deleted instances
echo ""
echo "Purging deleted instances..."
multipass purge

echo ""
echo "✅ Cleanup complete!"
echo ""
echo "Note: Your kubeconfig (~/.kube/config) has not been modified."
echo "You may want to manually remove the k8s-dev and k8s-prd contexts if needed:"
echo "  kubectl config delete-context k8s-dev"
echo "  kubectl config delete-context k8s-prd"
