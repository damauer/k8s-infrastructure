package addons

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Addon represents a Kubernetes add-on component
type Addon struct {
	Name        string
	Description string
	InstallFunc func(ctx context.Context, controlPlane string) error
}

// AddonManager manages the installation of Kubernetes add-ons
type AddonManager struct {
	provider     string // multipass or native
	controlPlane string
}

// NewAddonManager creates a new addon manager
func NewAddonManager(provider, controlPlane string) *AddonManager {
	return &AddonManager{
		provider:     provider,
		controlPlane: controlPlane,
	}
}

// execCommand executes a command on the control plane node
func (m *AddonManager) execCommand(ctx context.Context, command string) error {
	var cmd *exec.Cmd
	if m.provider == "multipass" {
		// Set KUBECONFIG for kubectl to work properly
		wrappedCmd := fmt.Sprintf("export KUBECONFIG=/etc/kubernetes/admin.conf; %s", command)
		cmd = exec.CommandContext(ctx, "multipass", "exec", m.controlPlane, "--", "bash", "-c", wrappedCmd)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// execCommandOutput executes a command and returns its output
func (m *AddonManager) execCommandOutput(ctx context.Context, command string) (string, error) {
	var cmd *exec.Cmd
	if m.provider == "multipass" {
		// Set KUBECONFIG for kubectl to work properly
		wrappedCmd := fmt.Sprintf("export KUBECONFIG=/etc/kubernetes/admin.conf; %s", command)
		cmd = exec.CommandContext(ctx, "multipass", "exec", m.controlPlane, "--", "bash", "-c", wrappedCmd)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-c", command)
	}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// InstallMetricsServer installs the Kubernetes metrics-server
func (m *AddonManager) InstallMetricsServer(ctx context.Context) error {
	installCmd := `kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml`
	if err := m.execCommand(ctx, installCmd); err != nil {
		return fmt.Errorf("failed to install metrics-server: %w", err)
	}

	// Wait for metrics-server to be ready
	waitCmd := `kubectl wait --for=condition=ready pod -l k8s-app=metrics-server -n kube-system --timeout=120s || true`
	if err := m.execCommand(ctx, waitCmd); err != nil {
		return fmt.Errorf("metrics-server not ready: %w", err)
	}

	return nil
}

// InstallHelm installs Helm package manager
func (m *AddonManager) InstallHelm(ctx context.Context) error {
	installScript := `curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash`
	if err := m.execCommand(ctx, installScript); err != nil {
		return fmt.Errorf("failed to install Helm: %w", err)
	}
	return nil
}

// InstallIngressNginx installs the NGINX Ingress Controller
func (m *AddonManager) InstallIngressNginx(ctx context.Context) error {
	installCmd := `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml`
	if err := m.execCommand(ctx, installCmd); err != nil {
		return fmt.Errorf("failed to install ingress-nginx: %w", err)
	}

	// Wait for ingress controller to be ready
	waitCmd := `kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=120s || true`
	if err := m.execCommand(ctx, waitCmd); err != nil {
		return fmt.Errorf("ingress controller not ready: %w", err)
	}

	return nil
}

// InstallPrometheusStack installs kube-prometheus-stack (Prometheus + Grafana)
func (m *AddonManager) InstallPrometheusStack(ctx context.Context) error {
	installScript := `
		helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
		helm repo update
		helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
			--namespace monitoring \
			--create-namespace \
			--set prometheus.prometheusSpec.retention=7d \
			--set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=10Gi
	`
	if err := m.execCommand(ctx, installScript); err != nil {
		return fmt.Errorf("failed to install kube-prometheus-stack: %w", err)
	}

	// Wait for monitoring stack to be ready
	waitCmd := `kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n monitoring --timeout=300s`
	if err := m.execCommand(ctx, waitCmd); err != nil {
		return fmt.Errorf("monitoring stack not ready: %w", err)
	}

	return nil
}

// ExposeGrafana exposes Grafana as a NodePort service
func (m *AddonManager) ExposeGrafana(ctx context.Context, nodePort int) error {
	exposeScript := fmt.Sprintf(`
		kubectl patch svc kube-prometheus-stack-grafana -n monitoring \
			-p '{"spec": {"type": "NodePort", "ports": [{"port": 80, "nodePort": %d, "targetPort": 3000}]}}'
	`, nodePort)
	if err := m.execCommand(ctx, exposeScript); err != nil {
		return fmt.Errorf("failed to expose Grafana: %w", err)
	}
	return nil
}

// ExposePrometheus exposes Prometheus as a NodePort service
func (m *AddonManager) ExposePrometheus(ctx context.Context, nodePort int) error {
	exposeScript := fmt.Sprintf(`
		kubectl patch svc kube-prometheus-stack-prometheus -n monitoring \
			-p '{"spec": {"type": "NodePort", "ports": [{"port": 9090, "nodePort": %d, "targetPort": 9090}]}}'
	`, nodePort)
	if err := m.execCommand(ctx, exposeScript); err != nil {
		return fmt.Errorf("failed to expose Prometheus: %w", err)
	}
	return nil
}

// GetGrafanaPassword retrieves the Grafana admin password
func (m *AddonManager) GetGrafanaPassword(ctx context.Context) (string, error) {
	cmd := `kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d`
	password, err := m.execCommandOutput(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get Grafana password: %w", err)
	}
	return password, nil
}

// InstallArgoCD installs ArgoCD GitOps tool
func (m *AddonManager) InstallArgoCD(ctx context.Context) error {
	installScript := `
		kubectl create namespace argocd
		kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	`
	if err := m.execCommand(ctx, installScript); err != nil {
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}

	// Wait for ArgoCD server to be ready
	waitCmd := `kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s`
	if err := m.execCommand(ctx, waitCmd); err != nil {
		return fmt.Errorf("ArgoCD server not ready: %w", err)
	}

	return nil
}

// ExposeArgoCD exposes ArgoCD server as a NodePort service
func (m *AddonManager) ExposeArgoCD(ctx context.Context, nodePort int) error {
	exposeScript := fmt.Sprintf(`
		kubectl patch svc argocd-server -n argocd \
			-p '{"spec": {"type": "NodePort", "ports": [{"name": "http", "port": 80, "nodePort": %d, "targetPort": 8080}, {"name": "https", "port": 443, "nodePort": %d, "targetPort": 8080}]}}'
	`, nodePort, nodePort+362) // 30081 for http, 30443 for https
	if err := m.execCommand(ctx, exposeScript); err != nil {
		return fmt.Errorf("failed to expose ArgoCD: %w", err)
	}
	return nil
}

// GetArgoPassword retrieves the ArgoCD admin password
func (m *AddonManager) GetArgoPassword(ctx context.Context) (string, error) {
	cmd := `kubectl get secret -n argocd argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d`
	password, err := m.execCommandOutput(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to get ArgoCD password: %w", err)
	}
	return password, nil
}

// InstallAll installs all add-ons in parallel where possible
func (m *AddonManager) InstallAll(ctx context.Context) error {
	fmt.Println("\n📦 Installing Kubernetes add-ons...")

	// Phase 1: Install metrics-server, Helm, and Ingress in parallel
	fmt.Println("📊 Installing core components in parallel...")
	var wg sync.WaitGroup
	errChan := make(chan error, 3)

	wg.Add(3)
	go func() {
		defer wg.Done()
		fmt.Println("  • Installing metrics-server...")
		if err := m.InstallMetricsServer(ctx); err != nil {
			errChan <- err
		} else {
			fmt.Println("  ✓ metrics-server installed")
		}
	}()

	go func() {
		defer wg.Done()
		fmt.Println("  • Installing Helm...")
		if err := m.InstallHelm(ctx); err != nil {
			errChan <- err
		} else {
			fmt.Println("  ✓ Helm installed")
		}
	}()

	go func() {
		defer wg.Done()
		fmt.Println("  • Installing Ingress NGINX...")
		if err := m.InstallIngressNginx(ctx); err != nil {
			errChan <- err
		} else {
			fmt.Println("  ✓ Ingress NGINX installed")
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors in phase 1
	for err := range errChan {
		return fmt.Errorf("core component installation failed: %w", err)
	}

	// Phase 2: Install Prometheus Stack (requires Helm)
	fmt.Println("🔭 Installing Prometheus Stack (Prometheus + Grafana)...")
	if err := m.InstallPrometheusStack(ctx); err != nil {
		return err
	}
	fmt.Println("  ✓ Prometheus Stack installed")

	// Phase 3: Expose monitoring services in parallel
	fmt.Println("🌐 Exposing monitoring services...")
	wg = sync.WaitGroup{}
	errChan = make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := m.ExposeGrafana(ctx, 30080); err != nil {
			errChan <- err
		} else {
			fmt.Println("  ✓ Grafana exposed on port 30080")
		}
	}()

	go func() {
		defer wg.Done()
		if err := m.ExposePrometheus(ctx, 30090); err != nil {
			errChan <- err
		} else {
			fmt.Println("  ✓ Prometheus exposed on port 30090")
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors in phase 3
	for err := range errChan {
		return fmt.Errorf("service exposure failed: %w", err)
	}

	// Phase 4: Install ArgoCD
	fmt.Println("🚀 Installing ArgoCD...")
	if err := m.InstallArgoCD(ctx); err != nil {
		return err
	}
	fmt.Println("  ✓ ArgoCD installed")

	// Expose ArgoCD
	if err := m.ExposeArgoCD(ctx, 30081); err != nil {
		return err
	}
	fmt.Println("  ✓ ArgoCD exposed on port 30081 (HTTP) and 30443 (HTTPS)")

	// Small delay to ensure services are ready
	time.Sleep(5 * time.Second)

	return nil
}

// GetAccessInfo retrieves access information for all services
func (m *AddonManager) GetAccessInfo(ctx context.Context, controlPlaneIP string) (map[string]string, error) {
	info := make(map[string]string)

	// Get Grafana password
	grafanaPassword, err := m.GetGrafanaPassword(ctx)
	if err != nil {
		return nil, err
	}

	// Get ArgoCD password
	argoPassword, err := m.GetArgoPassword(ctx)
	if err != nil {
		return nil, err
	}

	info["grafana_url"] = fmt.Sprintf("http://%s:30080", controlPlaneIP)
	info["grafana_username"] = "admin"
	info["grafana_password"] = grafanaPassword

	info["prometheus_url"] = fmt.Sprintf("http://%s:30090", controlPlaneIP)

	info["argocd_url_http"] = fmt.Sprintf("http://%s:30081", controlPlaneIP)
	info["argocd_url_https"] = fmt.Sprintf("https://%s:30443", controlPlaneIP)
	info["argocd_username"] = "admin"
	info["argocd_password"] = argoPassword

	return info, nil
}
