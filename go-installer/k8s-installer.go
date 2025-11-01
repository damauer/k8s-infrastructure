package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// Kubernetes version to install
	K8S_VERSION = "1.30"

	// Pod network CIDR
	POD_NETWORK_CIDR = "10.244.0.0/16"

	// Default VM resources
	DEFAULT_CPUS   = "2"
	DEFAULT_MEMORY = "4G"
	DEFAULT_DISK   = "20G"

	// Ubuntu release
	UBUNTU_RELEASE = "noble" // Ubuntu 24.04 LTS Noble Numbat

	// Node names
	CONTROL_PLANE_NAME = "k8s-control-plane"
	WORKER_PREFIX      = "k8s-worker"
)

type ClusterConfig struct {
	ControlPlane NodeConfig
	Workers      []NodeConfig
	PodCIDR      string
	K8sVersion   string
}

type NodeConfig struct {
	Name   string
	CPUs   string
	Memory string
	Disk   string
}

type MultipassInfo struct {
	Errors []interface{} `json:"errors"`
	Info   map[string]struct {
		IPv4 []string `json:"ipv4"`
		State string  `json:"state"`
	} `json:"info"`
}

func main() {
	log.Println("🚀 Starting Kubernetes cluster installation with Multipass")

	// Check prerequisites
	if err := checkPrerequisites(); err != nil {
		log.Fatalf("❌ Prerequisites check failed: %v", err)
	}

	// Create cluster configuration
	config := ClusterConfig{
		ControlPlane: NodeConfig{
			Name:   CONTROL_PLANE_NAME,
			CPUs:   DEFAULT_CPUS,
			Memory: DEFAULT_MEMORY,
			Disk:   DEFAULT_DISK,
		},
		Workers: []NodeConfig{
			{
				Name:   WORKER_PREFIX + "-1",
				CPUs:   DEFAULT_CPUS,
				Memory: DEFAULT_MEMORY,
				Disk:   DEFAULT_DISK,
			},
			{
				Name:   WORKER_PREFIX + "-2",
				CPUs:   DEFAULT_CPUS,
				Memory: DEFAULT_MEMORY,
				Disk:   DEFAULT_DISK,
			},
		},
		PodCIDR:    POD_NETWORK_CIDR,
		K8sVersion: K8S_VERSION,
	}

	// Create cloud-init files
	if err := createCloudInitFiles(config); err != nil {
		log.Fatalf("❌ Failed to create cloud-init files: %v", err)
	}

	// Launch control plane
	log.Println("🎯 Launching control plane node...")
	if err := launchNode(config.ControlPlane, "control-plane-init.yaml"); err != nil {
		log.Fatalf("❌ Failed to launch control plane: %v", err)
	}

	// Wait for control plane to be ready
	log.Println("⏳ Waiting for control plane to initialize...")
	if err := waitForCloudInit(config.ControlPlane.Name); err != nil {
		log.Fatalf("❌ Control plane initialization failed: %v", err)
	}

	// Initialize Kubernetes cluster
	log.Println("🔧 Initializing Kubernetes cluster...")
	joinCommand, err := initializeCluster(config)
	if err != nil {
		log.Fatalf("❌ Failed to initialize cluster: %v", err)
	}

	// Save kubeconfig
	log.Println("💾 Saving kubeconfig...")
	if err := saveKubeconfig(config.ControlPlane.Name); err != nil {
		log.Fatalf("❌ Failed to save kubeconfig: %v", err)
	}

	// Install CNI (Calico)
	log.Println("🌐 Installing Calico CNI...")
	if err := installCalico(config); err != nil {
		log.Fatalf("❌ Failed to install Calico: %v", err)
	}

	// Launch worker nodes in parallel (OPTIMIZED)
	log.Println("🎯 Launching worker nodes in parallel...")
	var wg sync.WaitGroup
	errChan := make(chan error, len(config.Workers))

	for _, worker := range config.Workers {
		wg.Add(1)
		go func(w NodeConfig) {
			defer wg.Done()
			log.Printf("  • Launching %s...\n", w.Name)
			if err := launchNode(w, "worker-init.yaml"); err != nil {
				errChan <- fmt.Errorf("failed to launch %s: %v", w.Name, err)
				return
			}
			if err := waitForCloudInit(w.Name); err != nil {
				errChan <- fmt.Errorf("failed to initialize %s: %v", w.Name, err)
				return
			}
		}(worker)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		log.Fatalf("❌ Worker launch failed: %v", err)
	}
	log.Println("  ✓ All workers launched")

	// Join worker nodes to cluster in parallel (OPTIMIZED)
	log.Println("🔗 Joining workers to cluster in parallel...")
	wg = sync.WaitGroup{}
	errChan = make(chan error, len(config.Workers))

	for _, worker := range config.Workers {
		wg.Add(1)
		go func(w NodeConfig) {
			defer wg.Done()
			log.Printf("  • Joining %s...\n", w.Name)
			if err := joinWorker(w.Name, joinCommand); err != nil {
				errChan <- fmt.Errorf("failed to join %s: %v", w.Name, err)
			}
		}(worker)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		log.Fatalf("❌ Worker join failed: %v", err)
	}
	log.Println("  ✓ All workers joined")

	// Wait for all nodes to be ready with smart polling (OPTIMIZED)
	log.Println("⏳ Waiting for all nodes to be ready (smart polling)...")
	allNodes := []string{config.ControlPlane.Name}
	for _, w := range config.Workers {
		allNodes = append(allNodes, w.Name)
	}
	if err := waitForNodesReady(config.ControlPlane.Name, allNodes, 300); err != nil {
		log.Fatalf("❌ Nodes failed to become ready: %v", err)
	}
	log.Println("  ✓ All nodes ready")

	// Install metrics-server, Helm, and Ingress in parallel (OPTIMIZED)
	log.Println("📊📦🌐 Installing metrics-server, Helm, and Ingress in parallel...")
	wg = sync.WaitGroup{}
	errChan = make(chan error, 3)

	wg.Add(3)
	go func() {
		defer wg.Done()
		log.Println("  • Installing metrics-server...")
		if err := installMetricsServer(config.ControlPlane.Name); err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		log.Println("  • Installing Helm...")
		if err := installHelm(config.ControlPlane.Name); err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		log.Println("  • Installing Ingress NGINX...")
		if err := installIngressNginx(config.ControlPlane.Name); err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		log.Fatalf("❌ Component installation failed: %v", err)
	}
	log.Println("  ✓ metrics-server, Helm, and Ingress installed")

	// Install kube-prometheus-stack (requires Helm, so must be sequential)
	log.Println("🔭 Installing kube-prometheus-stack (Prometheus + Grafana)...")
	if err := installKubePrometheusStack(config.ControlPlane.Name); err != nil {
		log.Fatalf("❌ Failed to install kube-prometheus-stack: %v", err)
	}

	// Expose Grafana and Prometheus in parallel (OPTIMIZED)
	log.Println("🌐 Exposing Grafana and Prometheus...")
	wg = sync.WaitGroup{}
	errChan = make(chan error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := exposeGrafana(config.ControlPlane.Name); err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer wg.Done()
		if err := exposePrometheus(config.ControlPlane.Name); err != nil {
			errChan <- err
		}
	}()

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		log.Fatalf("❌ Service exposure failed: %v", err)
	}

	// Get Grafana password
	grafanaPassword, err := getGrafanaPassword(config.ControlPlane.Name)
	if err != nil {
		log.Fatalf("❌ Failed to get Grafana password: %v", err)
	}

	// Get control plane IP for monitoring URLs
	controlPlaneIP, err := getNodeIP(config.ControlPlane.Name)
	if err != nil {
		log.Fatalf("❌ Failed to get control plane IP: %v", err)
	}

	// Display cluster status
	log.Println("\n✅ Cluster installation complete!")
	displayClusterInfo(config, controlPlaneIP, grafanaPassword)
}

func checkPrerequisites() error {
	log.Println("🔍 Checking prerequisites...")

	// Check if multipass is installed
	if _, err := exec.LookPath("multipass"); err != nil {
		return fmt.Errorf("multipass is not installed. Install it from https://multipass.run/")
	}

	// Check if kubectl is installed
	if _, err := exec.LookPath("kubectl"); err != nil {
		log.Println("⚠️  kubectl not found. You'll need to install it to manage the cluster.")
	}

	log.Println("✓ Prerequisites check passed")
	return nil
}

func createCloudInitFiles(config ClusterConfig) error {
	log.Println("📝 Creating cloud-init configuration files...")

	controlPlaneInit := getControlPlaneCloudInit(config.K8sVersion)
	if err := os.WriteFile("control-plane-init.yaml", []byte(controlPlaneInit), 0644); err != nil {
		return err
	}

	workerInit := getWorkerCloudInit(config.K8sVersion)
	if err := os.WriteFile("worker-init.yaml", []byte(workerInit), 0644); err != nil {
		return err
	}

	log.Println("✓ Cloud-init files created")
	return nil
}

func getControlPlaneCloudInit(k8sVersion string) string {
	sshKey := getSSHPublicKey()
	sshKeyConfig := ""
	if sshKey != "" {
		sshKeyConfig = fmt.Sprintf(`
ssh_authorized_keys:
  - %s
`, sshKey)
	}

	return fmt.Sprintf(`#cloud-config

package_update: true
package_upgrade: true
%s
bootcmd:
  - modprobe overlay
  - modprobe br_netfilter

write_files:
  - path: /etc/modules-load.d/k8s.conf
    content: |
      overlay
      br_netfilter
  - path: /etc/sysctl.d/k8s.conf
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward                 = 1
  - path: /etc/apt/keyrings/.placeholder
    content: ""

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - socat
  - conntrack
  - ipset

runcmd:
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab

  # Apply sysctl settings
  - sysctl --system

  # Install containerd
  - |
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update
    apt-get install -y containerd.io

  # Configure containerd
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd

  # Install Kubernetes packages
  - |
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v%s/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v%s/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Control plane node initialization complete after $UPTIME seconds"
`, sshKeyConfig, k8sVersion, k8sVersion)
}

func getWorkerCloudInit(k8sVersion string) string {
	sshKey := getSSHPublicKey()
	sshKeyConfig := ""
	if sshKey != "" {
		sshKeyConfig = fmt.Sprintf(`
ssh_authorized_keys:
  - %s
`, sshKey)
	}

	return fmt.Sprintf(`#cloud-config

package_update: true
package_upgrade: true
%s
bootcmd:
  - modprobe overlay
  - modprobe br_netfilter

write_files:
  - path: /etc/modules-load.d/k8s.conf
    content: |
      overlay
      br_netfilter
  - path: /etc/sysctl.d/k8s.conf
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward                 = 1
  - path: /etc/apt/keyrings/.placeholder
    content: ""

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - socat
  - conntrack
  - ipset

runcmd:
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab

  # Apply sysctl settings
  - sysctl --system

  # Install containerd
  - |
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update
    apt-get install -y containerd.io

  # Configure containerd
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd

  # Install Kubernetes packages
  - |
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v%s/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v%s/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Worker node initialization complete after $UPTIME seconds"
`, sshKeyConfig, k8sVersion, k8sVersion)
}

func launchNode(node NodeConfig, cloudInitFile string) error {
	args := []string{
		"launch",
		"--name", node.Name,
		"--cpus", node.CPUs,
		"--memory", node.Memory,
		"--disk", node.Disk,
		"--network", "en0",
		"--cloud-init", cloudInitFile,
		UBUNTU_RELEASE,
	}

	cmd := exec.Command("multipass", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func waitForCloudInit(nodeName string) error {
	maxAttempts := 60
	for i := 0; i < maxAttempts; i++ {
		cmd := exec.Command("multipass", "exec", nodeName, "--", "cloud-init", "status")
		output, err := cmd.Output()
		if err == nil {
			status := strings.TrimSpace(string(output))
			if strings.Contains(status, "status: done") {
				log.Printf("  ✓ %s is ready\n", nodeName)
				return nil
			}
		}
		time.Sleep(5 * time.Second)
		log.Printf("  Attempt %d/%d...", i+1, maxAttempts)
	}
	return fmt.Errorf("timeout waiting for cloud-init on %s", nodeName)
}

// isNodeReady checks if a node is ready in Kubernetes
func isNodeReady(controlPlane, nodeName string) bool {
	cmd := exec.Command("multipass", "exec", controlPlane, "--", "kubectl", "get", "node", nodeName,
		"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "True"
}

// waitForNodesReady waits for all nodes to be ready with smart polling
func waitForNodesReady(controlPlane string, nodeNames []string, timeoutSeconds int) error {
	start := time.Now()
	timeout := time.Duration(timeoutSeconds) * time.Second

	for {
		allReady := true
		for _, nodeName := range nodeNames {
			if !isNodeReady(controlPlane, nodeName) {
				allReady = false
				break
			}
		}

		if allReady {
			return nil
		}

		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for nodes to be ready after %d seconds", timeoutSeconds)
		}

		time.Sleep(2 * time.Second)
	}
}

func getNodeIP(nodeName string) (string, error) {
	cmd := exec.Command("multipass", "info", nodeName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get node info: %w", err)
	}

	var info MultipassInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return "", fmt.Errorf("failed to parse multipass info: %w", err)
	}

	nodeInfo, exists := info.Info[nodeName]
	if !exists || len(nodeInfo.IPv4) == 0 {
		return "", fmt.Errorf("no IP address found for node %s", nodeName)
	}

	return nodeInfo.IPv4[0], nil
}

func initializeCluster(config ClusterConfig) (string, error) {
	// Get control plane IP
	controlPlaneIP, err := getNodeIP(config.ControlPlane.Name)
	if err != nil {
		return "", err
	}

	log.Printf("  Control plane IP: %s\n", controlPlaneIP)

	// Initialize cluster
	initCmd := fmt.Sprintf(
		"sudo kubeadm init --pod-network-cidr=%s --apiserver-advertise-address=%s --control-plane-endpoint=%s",
		config.PodCIDR,
		controlPlaneIP,
		controlPlaneIP,
	)

	cmd := exec.Command("multipass", "exec", config.ControlPlane.Name, "--", "bash", "-c", initCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Init output: %s\n", string(output))
		return "", fmt.Errorf("kubeadm init failed: %w", err)
	}

	// Extract join command
	joinRegex := regexp.MustCompile(`kubeadm join [^\n]+\n[^\n]+discovery-token-ca-cert-hash[^\n]+`)
	joinMatches := joinRegex.FindString(string(output))
	if joinMatches == "" {
		return "", fmt.Errorf("could not find join command in kubeadm output")
	}

	// Clean up join command (remove line continuations)
	joinCommand := strings.ReplaceAll(joinMatches, "\\\n", "")
	joinCommand = strings.ReplaceAll(joinCommand, "\\", "")
	joinCommand = strings.TrimSpace(joinCommand)

	// Setup kubeconfig on control plane
	kubeconfigSetup := `
		mkdir -p $HOME/.kube
		sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
		sudo chown $(id -u):$(id -g) $HOME/.kube/config
	`
	cmd = exec.Command("multipass", "exec", config.ControlPlane.Name, "--", "bash", "-c", kubeconfigSetup)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to setup kubeconfig: %w", err)
	}

	log.Println("  ✓ Cluster initialized")
	return joinCommand, nil
}

func saveKubeconfig(controlPlaneName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .kube directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeDir, "config")

	// Backup existing kubeconfig if it exists
	if _, err := os.Stat(kubeconfigPath); err == nil {
		backupPath := kubeconfigPath + ".backup." + time.Now().Format("20060102-150405")
		if err := os.Rename(kubeconfigPath, backupPath); err != nil {
			log.Printf("⚠️  Failed to backup existing kubeconfig: %v", err)
		} else {
			log.Printf("  📦 Existing kubeconfig backed up to: %s\n", backupPath)
		}
	}

	// Get kubeconfig from control plane
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "sudo", "cat", "/etc/kubernetes/admin.conf")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	// Get control plane IP
	controlPlaneIP, err := getNodeIP(controlPlaneName)
	if err != nil {
		return err
	}

	// Replace internal IP with multipass IP
	kubeconfig := string(output)
	// Match server: https://<ip>:<port> and replace with control plane IP
	serverRegex := regexp.MustCompile(`server: https://[^:]+:`)
	kubeconfig = serverRegex.ReplaceAllString(kubeconfig, fmt.Sprintf("server: https://%s:", controlPlaneIP))

	if err := os.WriteFile(kubeconfigPath, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	log.Printf("  ✓ Kubeconfig saved to: %s\n", kubeconfigPath)
	return nil
}

func installCalico(config ClusterConfig) error {
	// Install Tigera operator and custom resources in one command to avoid hanging
	log.Println("  Installing Calico CNI...")
	installScript := fmt.Sprintf(`
		kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/tigera-operator.yaml
		sleep 10
		curl -sS -O https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/custom-resources.yaml
		sed -i 's#cidr: 192\.168\.0\.0/16#cidr: %s#g' custom-resources.yaml
		kubectl create -f custom-resources.yaml
	`, config.PodCIDR)

	cmd := exec.Command("multipass", "exec", config.ControlPlane.Name, "--", "bash", "-c", installScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Calico installation output: %s\n", string(output))
		return fmt.Errorf("failed to install Calico: %w", err)
	}

	log.Println("  Waiting for Calico to be ready...")
	time.Sleep(30 * time.Second)

	log.Println("  ✓ Calico CNI installed")
	return nil
}

func joinWorker(workerName, joinCommand string) error {
	cmd := exec.Command("multipass", "exec", workerName, "--", "sudo", "bash", "-c", joinCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Join output: %s\n", string(output))
		return fmt.Errorf("failed to join worker: %w", err)
	}

	log.Printf("  ✓ %s joined successfully\n", workerName)
	return nil
}

func installMetricsServer(controlPlaneName string) error {
	log.Println("  Installing metrics-server...")
	installCmd := `kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install metrics-server: %w", err)
	}

	// Wait for metrics-server to be ready
	log.Println("  Waiting for metrics-server to be ready...")
	waitCmd := `kubectl wait --for=condition=ready pod -l k8s-app=metrics-server -n kube-system --timeout=300s`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("metrics-server not ready: %w", err)
	}

	log.Println("  ✓ metrics-server installed")
	return nil
}

func installHelm(controlPlaneName string) error {
	log.Println("  Installing Helm...")
	installScript := `
		curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
	`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install Helm: %w", err)
	}

	log.Println("  ✓ Helm installed")
	return nil
}

func installKubePrometheusStack(controlPlaneName string) error {
	log.Println("  Installing kube-prometheus-stack...")
	installScript := `
		helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
		helm repo update
		helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
			--namespace monitoring \
			--create-namespace \
			--set prometheus.prometheusSpec.retention=7d \
			--set prometheus.prometheusSpec.storageSpec.volumeClaimTemplate.spec.resources.requests.storage=10Gi
	`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Prometheus stack output: %s\n", string(output))
		return fmt.Errorf("failed to install kube-prometheus-stack: %w", err)
	}

	// Wait for monitoring stack to be ready
	log.Println("  Waiting for monitoring stack to be ready...")
	waitCmd := `kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n monitoring --timeout=300s`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("monitoring stack not ready: %w", err)
	}

	log.Println("  ✓ kube-prometheus-stack installed")
	return nil
}

func exposeGrafana(controlPlaneName string) error {
	log.Println("  Exposing Grafana as NodePort...")
	exposeScript := `
		kubectl patch svc kube-prometheus-stack-grafana -n monitoring \
			-p '{"spec": {"type": "NodePort", "ports": [{"port": 80, "nodePort": 30080, "targetPort": 3000}]}}'
	`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", exposeScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to expose Grafana: %w", err)
	}

	log.Println("  ✓ Grafana exposed on port 30080")
	return nil
}

func exposePrometheus(controlPlaneName string) error {
	exposeScript := `
		kubectl patch svc kube-prometheus-stack-prometheus -n monitoring \
			-p '{"spec": {"type": "NodePort", "ports": [{"port": 9090, "nodePort": 30090, "targetPort": 9090}]}}'
	`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", exposeScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to expose Prometheus: %w", err)
	}

	log.Println("  ✓ Prometheus exposed on port 30090")
	return nil
}

func getGrafanaPassword(controlPlaneName string) (string, error) {
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c",
		"kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Grafana password: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func installIngressNginx(controlPlaneName string) error {
	log.Println("  Installing Ingress NGINX...")
	installCmd := `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install ingress-nginx: %w", err)
	}

	// Wait for ingress controller to be ready
	log.Println("  Waiting for ingress controller to be ready...")
	waitCmd := `kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=300s`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ingress controller not ready: %w", err)
	}

	log.Println("  ✓ Ingress NGINX installed")
	return nil
}

func getSSHPublicKey() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Try Ed25519 key first (more modern)
	keyPath := filepath.Join(homeDir, ".ssh", "id_ed25519.pub")
	if key, err := os.ReadFile(keyPath); err == nil {
		return strings.TrimSpace(string(key))
	}

	// Fall back to RSA key
	keyPath = filepath.Join(homeDir, ".ssh", "id_rsa.pub")
	if key, err := os.ReadFile(keyPath); err == nil {
		return strings.TrimSpace(string(key))
	}

	return ""
}

func displayClusterInfo(config ClusterConfig, controlPlaneIP, grafanaPassword string) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎉 Kubernetes Cluster Ready!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\n📦 Cluster Configuration:")
	fmt.Printf("  • Kubernetes Version: %s\n", config.K8sVersion)
	fmt.Printf("  • Pod Network CIDR:   %s\n", config.PodCIDR)
	fmt.Printf("  • Ubuntu Release:     24.04 LTS (Noble Numbat)\n")
	fmt.Printf("  • Container Runtime:  containerd\n")
	fmt.Printf("  • CNI Plugin:         Calico\n")
	fmt.Printf("  • Monitoring:         kube-prometheus-stack\n")
	fmt.Printf("  • Ingress:            NGINX Ingress Controller\n")

	fmt.Println("\n🖥️  Cluster Nodes:")
	fmt.Printf("  • Control Plane: %s\n", config.ControlPlane.Name)
	for _, worker := range config.Workers {
		fmt.Printf("  • Worker: %s\n", worker.Name)
	}

	fmt.Println("\n📊 Monitoring Stack:")
	fmt.Printf("  • Prometheus:  http://%s:30090\n", controlPlaneIP)
	fmt.Printf("  • Grafana:     http://%s:30080\n", controlPlaneIP)
	fmt.Printf("  • Username:    admin\n")
	fmt.Printf("  • Password:    %s\n", grafanaPassword)
	fmt.Println()
	fmt.Println("  28 Pre-configured Grafana dashboards including:")
	fmt.Println("  • Kubernetes / Compute Resources / Cluster")
	fmt.Println("  • Kubernetes / Networking / Cluster")
	fmt.Println("  • Node Exporter / Nodes")
	fmt.Println("  • And many more...")

	fmt.Println("\n📋 Useful Commands:")
	fmt.Println("  • Check nodes:          kubectl get nodes")
	fmt.Println("  • Check pods:           kubectl get pods -A")
	fmt.Println("  • View metrics:         kubectl top nodes")
	fmt.Println("  • Shell to control:     multipass shell", config.ControlPlane.Name)
	fmt.Println("  • Stop cluster:         multipass stop --all")
	fmt.Println("  • Start cluster:        multipass start --all")
	fmt.Println("  • Delete cluster:       multipass delete --all --purge")

	fmt.Println("\n🔧 Testing the cluster:")
	fmt.Println("  kubectl create deployment nginx --image=nginx")
	fmt.Println("  kubectl expose deployment nginx --port=80 --type=NodePort")
	fmt.Println("  kubectl get svc nginx")

	fmt.Println("\n📁 Kubeconfig: ~/.kube/config")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	// Display actual node status
	log.Println("📊 Current cluster status:")
	cmd := exec.Command("kubectl", "get", "nodes", "-o", "wide")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
