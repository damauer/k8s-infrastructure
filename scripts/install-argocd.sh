#!/bin/bash
set -e

echo "🚀 Installing ArgoCD on DEV cluster..."
echo ""

# Color codes
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if kubectl is configured for DEV cluster
echo -n "Checking kubectl configuration... "
if kubectl cluster-info &> /dev/null; then
    echo -e "${GREEN}✓${NC}"
else
    echo "❌ kubectl is not configured or cluster is not accessible"
    exit 1
fi

# Create argocd namespace
echo -n "Creating argocd namespace... "
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f - > /dev/null 2>&1
echo -e "${GREEN}✓${NC}"

# Install ArgoCD
echo "Installing ArgoCD..."
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

echo ""
echo "⏳ Waiting for ArgoCD to be ready..."
kubectl wait --for=condition=available --timeout=600s \
    deployment/argocd-server \
    deployment/argocd-repo-server \
    deployment/argocd-applicationset-controller \
    -n argocd

echo ""
echo -e "${GREEN}✅ ArgoCD installed successfully!${NC}"
echo ""

# Expose ArgoCD server as NodePort
echo "🌐 Exposing ArgoCD server as NodePort..."
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "NodePort", "ports": [{"port": 443, "nodePort": 30443, "name": "https"}]}}'
echo -e "${GREEN}✓${NC} ArgoCD server exposed on port 30443"

echo ""
echo "📋 Getting access information..."

# Get control plane IP
CONTROL_PLANE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')

# Get initial admin password
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)

echo ""
echo "============================================================"
echo "🎉 ArgoCD Ready!"
echo "============================================================"
echo ""
echo "📍 Access ArgoCD UI:"
echo "  URL:      https://${CONTROL_PLANE_IP}:30443"
echo "  Username: admin"
echo "  Password: ${ARGOCD_PASSWORD}"
echo ""
echo -e "${YELLOW}⚠  Note: You'll need to accept the self-signed certificate${NC}"
echo ""
echo "🔧 CLI Login:"
echo "  argocd login ${CONTROL_PLANE_IP}:30443 --username admin --password ${ARGOCD_PASSWORD} --insecure"
echo ""
echo "📚 Next Steps:"
echo "  1. Apply ArgoCD projects:"
echo "     kubectl apply -f ~/k8s-app-manifests/argocd/projects/"
echo ""
echo "  2. Apply ArgoCD applications:"
echo "     kubectl apply -f ~/k8s-app-manifests/argocd/applications/dev/"
echo ""
echo "  3. Access the UI and monitor deployments"
echo ""
echo "============================================================"
