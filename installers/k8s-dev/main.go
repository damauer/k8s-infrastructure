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

	// Node names - DEV environment
	CONTROL_PLANE_NAME = "k8s-dev-c1"
	WORKER_PREFIX      = "k8s-dev-w"

	// Context and cluster name
	CONTEXT_NAME = "k8s-dev"
	CLUSTER_NAME = "k8s-dev"
)

type ClusterConfig struct {
	ControlPlane NodeConfig
	Workers      []NodeConfig
	PodCIDR      string
	K8sVersion   string
	ContextName  string
	ClusterName  string
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
	log.Println("🚀 Starting Kubernetes DEV cluster installation with Multipass")

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
				Name:   WORKER_PREFIX + "1",
				CPUs:   DEFAULT_CPUS,
				Memory: DEFAULT_MEMORY,
				Disk:   DEFAULT_DISK,
			},
			{
				Name:   WORKER_PREFIX + "2",
				CPUs:   DEFAULT_CPUS,
				Memory: DEFAULT_MEMORY,
				Disk:   DEFAULT_DISK,
			},
		},
		PodCIDR:     POD_NETWORK_CIDR,
		K8sVersion:  K8S_VERSION,
		ContextName: CONTEXT_NAME,
		ClusterName: CLUSTER_NAME,
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

	// Save kubeconfig (with merging support)
	log.Println("💾 Saving kubeconfig...")
	if err := saveKubeconfig(config); err != nil {
		log.Fatalf("❌ Failed to save kubeconfig: %v", err)
	}

	// Install CNI (Calico)
	log.Println("🌐 Installing Flannel CNI...")
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

	// Install ArgoCD
	log.Println("🚀 Installing ArgoCD...")
	if err := installArgoCD(config.ControlPlane.Name); err != nil {
		log.Fatalf("❌ Failed to install ArgoCD: %v", err)
	}

	// Get Grafana password
	grafanaPassword, err := getGrafanaPassword(config.ControlPlane.Name)
	if err != nil {
		log.Fatalf("❌ Failed to get Grafana password: %v", err)
	}

	// Get ArgoCD password
	argocdPassword, err := getArgocdPassword(config.ControlPlane.Name)
	if err != nil {
		log.Fatalf("❌ Failed to get ArgoCD password: %v", err)
	}

	// Get control plane IP for monitoring URLs
	controlPlaneIP, err := getNodeIP(config.ControlPlane.Name)
	if err != nil {
		log.Fatalf("❌ Failed to get control plane IP: %v", err)
	}

	// Display cluster status
	log.Println("\n✅ DEV Cluster installation complete!")
	displayClusterInfo(config, controlPlaneIP, grafanaPassword, argocdPassword)
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

func saveKubeconfig(config ClusterConfig) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeDir := filepath.Join(homeDir, ".kube")
	if err := os.MkdirAll(kubeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .kube directory: %w", err)
	}

	kubeconfigPath := filepath.Join(kubeDir, "config")
	tempKubeconfigPath := filepath.Join(kubeDir, fmt.Sprintf("config.%s.tmp", config.ContextName))

	// Get kubeconfig from control plane
	cmd := exec.Command("multipass", "exec", config.ControlPlane.Name, "--", "sudo", "cat", "/etc/kubernetes/admin.conf")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	// Get control plane IP
	controlPlaneIP, err := getNodeIP(config.ControlPlane.Name)
	if err != nil {
		return err
	}

	// Replace internal IP with multipass IP and update context/cluster names
	kubeconfig := string(output)

	// Replace server address
	serverRegex := regexp.MustCompile(`server: https://[^:]+:`)
	kubeconfig = serverRegex.ReplaceAllString(kubeconfig, fmt.Sprintf("server: https://%s:", controlPlaneIP))

	// Replace cluster name
	kubeconfig = strings.ReplaceAll(kubeconfig, "name: kubernetes", fmt.Sprintf("name: %s", config.ClusterName))
	kubeconfig = strings.ReplaceAll(kubeconfig, "cluster: kubernetes", fmt.Sprintf("cluster: %s", config.ClusterName))

	// Replace context name
	kubeconfig = strings.ReplaceAll(kubeconfig, "name: kubernetes-admin@kubernetes", fmt.Sprintf("name: %s", config.ContextName))
	kubeconfig = strings.ReplaceAll(kubeconfig, "current-context: kubernetes-admin@kubernetes", fmt.Sprintf("current-context: %s", config.ContextName))

	// Write temporary kubeconfig
	if err := os.WriteFile(tempKubeconfigPath, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write temporary kubeconfig: %w", err)
	}

	// Merge with existing kubeconfig if it exists
	if _, err := os.Stat(kubeconfigPath); err == nil {
		// Existing kubeconfig exists, merge them
		log.Println("  📦 Merging with existing kubeconfig...")

		// Backup existing kubeconfig
		backupPath := kubeconfigPath + ".backup." + time.Now().Format("20060102-150405")
		if err := exec.Command("cp", kubeconfigPath, backupPath).Run(); err != nil {
			log.Printf("⚠️  Failed to backup existing kubeconfig: %v", err)
		} else {
			log.Printf("  📦 Existing kubeconfig backed up to: %s\n", backupPath)
		}

		// Merge kubeconfigs using kubectl config view --flatten
		mergeCmd := exec.Command("bash", "-c",
			fmt.Sprintf("KUBECONFIG=%s:%s kubectl config view --flatten > %s.merged && mv %s.merged %s",
				kubeconfigPath, tempKubeconfigPath, kubeconfigPath, kubeconfigPath, kubeconfigPath))
		if err := mergeCmd.Run(); err != nil {
			return fmt.Errorf("failed to merge kubeconfigs: %w", err)
		}

		// Rename the context if it has the wrong name (e.g., kubernetes-admin@kubernetes -> k8s-dev)
		renameCmd := exec.Command("bash", "-c",
			fmt.Sprintf("kubectl config rename-context %s-admin@%s %s 2>/dev/null || kubectl config rename-context kubernetes-admin@%s %s 2>/dev/null || true",
				config.ClusterName, config.ClusterName, config.ContextName, config.ClusterName, config.ContextName))
		renameCmd.Run() // Ignore errors as context might already be named correctly

		// Remove temporary file
		os.Remove(tempKubeconfigPath)

		log.Printf("  ✓ Kubeconfig merged with context: %s\n", config.ContextName)
	} else {
		// No existing kubeconfig, just rename temp to config
		if err := os.Rename(tempKubeconfigPath, kubeconfigPath); err != nil {
			return fmt.Errorf("failed to save kubeconfig: %w", err)
		}
		log.Printf("  ✓ Kubeconfig saved to: %s\n", kubeconfigPath)
	}

	// Set current context to the new cluster
	setContextCmd := exec.Command("kubectl", "config", "use-context", config.ContextName)
	if err := setContextCmd.Run(); err != nil {
		log.Printf("⚠️  Failed to set current context to %s: %v", config.ContextName, err)
	} else {
		log.Printf("  ✓ Current context set to: %s\n", config.ContextName)
	}

	return nil
}

func installCalico(config ClusterConfig) error {
	// Install Flannel CNI (uses quay.io, avoids Docker Hub rate limiting)
	log.Println("  Installing Flannel CNI...")
	installCmd := `kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml`

	cmd := exec.Command("multipass", "exec", config.ControlPlane.Name, "--", "bash", "-c", installCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Flannel installation output: %s\n", string(output))
		return fmt.Errorf("failed to install Flannel: %w", err)
	}

	log.Println("  Waiting for Flannel to be ready...")
	time.Sleep(30 * time.Second)

	log.Println("  ✓ Flannel CNI installed")
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install metrics-server: %w\nOutput: %s", err, string(output))
	}

	// Patch metrics-server deployment to add --kubelet-insecure-tls flag for dev environments
	log.Println("  Patching metrics-server for insecure TLS (dev environment)...")
	patchCmd := `kubectl patch deployment metrics-server -n kube-system --type='json' -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", patchCmd)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to patch metrics-server: %w\nOutput: %s", err, string(output))
	}

	// Wait for metrics-server to be ready (reduced timeout)
	log.Println("  Waiting for metrics-server to be ready...")
	waitCmd := `kubectl wait --for=condition=ready pod -l k8s-app=metrics-server -n kube-system --timeout=120s || true`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("  ⚠️  Warning: metrics-server wait command failed (pod may still be starting): %v\nOutput: %s", err, string(output))
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
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Helm installation output: %s\n", string(output))
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
			--set prometheus.prometheusSpec.storageSpec=null
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

func installArgoCD(controlPlaneName string) error {
	log.Println("  Installing ArgoCD...")
	installScript := `
		kubectl create namespace argocd
		kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("ArgoCD installation output: %s\n", string(output))
		return fmt.Errorf("failed to install ArgoCD: %w", err)
	}

	// Wait for ArgoCD server to be ready
	log.Println("  Waiting for ArgoCD server to be ready...")
	waitCmd := `kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ArgoCD server not ready: %w", err)
	}

	// Expose ArgoCD server as NodePort
	log.Println("  Exposing ArgoCD server as NodePort...")
	exposeScript := `
		kubectl patch svc argocd-server -n argocd \
			-p '{"spec": {"type": "NodePort", "ports": [{"port": 443, "nodePort": 30443, "targetPort": 8080}]}}'
	`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", exposeScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to expose ArgoCD: %w", err)
	}

	log.Println("  ✓ ArgoCD installed and exposed on port 30443")
	return nil
}

func getArgocdPassword(controlPlaneName string) (string, error) {
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c",
		"kubectl get secret -n argocd argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get ArgoCD password: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func installIngressNginx(controlPlaneName string) error {
	log.Println("  Installing Ingress NGINX...")
	installCmd := `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml`
	cmd := exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", installCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install ingress-nginx: %w\nOutput: %s", err, string(output))
	}

	// Wait for ingress controller to be ready (reduced timeout)
	log.Println("  Waiting for ingress controller to be ready...")
	waitCmd := `kubectl wait --namespace ingress-nginx --for=condition=ready pod --selector=app.kubernetes.io/component=controller --timeout=120s || true`
	cmd = exec.Command("multipass", "exec", controlPlaneName, "--", "bash", "-c", waitCmd)
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Printf("  ⚠️  Warning: ingress controller wait command failed (pod may still be starting): %v\nOutput: %s", err, string(output))
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

func displayClusterInfo(config ClusterConfig, controlPlaneIP, grafanaPassword, argocdPassword string) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🎉 Kubernetes DEV Cluster Ready!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\n📦 Cluster Configuration:")
	fmt.Printf("  • Environment:        DEV\n")
	fmt.Printf("  • Context Name:       %s\n", config.ContextName)
	fmt.Printf("  • Cluster Name:       %s\n", config.ClusterName)
	fmt.Printf("  • Kubernetes Version: %s\n", config.K8sVersion)
	fmt.Printf("  • Pod Network CIDR:   %s\n", config.PodCIDR)
	fmt.Printf("  • Ubuntu Release:     24.04 LTS (Noble Numbat)\n")
	fmt.Printf("  • Container Runtime:  containerd\n")
	fmt.Printf("  • CNI Plugin:         Calico\n")
	fmt.Printf("  • Monitoring:         kube-prometheus-stack\n")
	fmt.Printf("  • GitOps:             ArgoCD\n")
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

	fmt.Println("\n🚀 ArgoCD (GitOps):")
	fmt.Printf("  • ArgoCD UI:   http://%s:30443\n", controlPlaneIP)
	fmt.Printf("  • Username:    admin\n")
	fmt.Printf("  • Password:    %s\n", argocdPassword)

	fmt.Println("\n🔧 Multi-Cluster Management:")
	fmt.Println("  • Switch to DEV:  kubectl config use-context", config.ContextName)
	fmt.Println("  • List contexts:  kubectl config get-contexts")
	fmt.Println("  • Current context: kubectl config current-context")

	fmt.Println("\n📋 Useful Commands:")
	fmt.Println("  • Check nodes:       kubectl get nodes")
	fmt.Println("  • Check pods:        kubectl get pods -A")
	fmt.Println("  • View metrics:      kubectl top nodes")
	fmt.Println("  • Shell to control:  multipass shell", config.ControlPlane.Name)
	fmt.Println("  • Stop cluster:      multipass stop", config.ControlPlane.Name, config.Workers[0].Name, config.Workers[1].Name)
	fmt.Println("  • Start cluster:     multipass start", config.ControlPlane.Name, config.Workers[0].Name, config.Workers[1].Name)
	fmt.Println("  • Delete cluster:    multipass delete", config.ControlPlane.Name, config.Workers[0].Name, config.Workers[1].Name, "--purge")

	fmt.Println("\n📁 Kubeconfig: ~/.kube/config")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	// Display actual node status
	log.Println("📊 Current cluster status:")
	cmd := exec.Command("kubectl", "get", "nodes", "-o", "wide")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
