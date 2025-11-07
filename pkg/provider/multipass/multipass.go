package multipass

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"k8s-infrastructure/pkg/provider"
)

// MultipassProvider implements the Provider interface for Multipass VMs
type MultipassProvider struct {
	config         map[string]interface{}
	vmDriver       string
	ubuntuRelease  string
	network        string // For WSL2: "Ethernet"
}

// NewMultipassProvider creates a new Multipass provider
func NewMultipassProvider() *MultipassProvider {
	return &MultipassProvider{}
}

// Name returns the provider name
func (p *MultipassProvider) Name() string {
	return "multipass"
}

// Initialize sets up the provider with platform-specific configuration
func (p *MultipassProvider) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config

	// Extract configuration values
	if driver, ok := config["vm_driver"].(string); ok {
		p.vmDriver = driver
	}
	if release, ok := config["ubuntu_release"].(string); ok {
		p.ubuntuRelease = release
	}
	if network, ok := config["multipass_network"].(string); ok {
		p.network = network
	}

	// Verify multipass is installed
	cmd := exec.CommandContext(ctx, "multipass", "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("multipass not found or not installed: %w", err)
	}

	return nil
}

// CreateNode creates a single node with the given configuration
func (p *MultipassProvider) CreateNode(ctx context.Context, config provider.NodeConfig) error {
	args := []string{"launch"}

	// Add node name
	args = append(args, "--name", config.Name)

	// Add resources
	if config.CPUs != "" {
		args = append(args, "--cpus", config.CPUs)
	}
	if config.Memory != "" {
		args = append(args, "--memory", config.Memory)
	}
	if config.Disk != "" {
		args = append(args, "--disk", config.Disk)
	}

	// Add Ubuntu release if specified
	if p.ubuntuRelease != "" {
		args = append(args, p.ubuntuRelease)
	}

	// Add network (en0 for macOS, or custom from config)
	network := p.network
	if network == "" {
		network = "en0" // Default to en0 for macOS
	}
	args = append(args, "--network", network)

	// Add cloud-init if provided
	if config.CloudInitPath != "" {
		args = append(args, "--cloud-init", config.CloudInitPath)
	}

	cmd := exec.CommandContext(ctx, "multipass", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create node %s: %w\nOutput: %s", config.Name, err, string(output))
	}

	return nil
}

// DeleteNode removes a node
func (p *MultipassProvider) DeleteNode(ctx context.Context, nodeName string) error {
	// Stop the node first
	stopCmd := exec.CommandContext(ctx, "multipass", "stop", nodeName)
	_ = stopCmd.Run() // Ignore errors if already stopped

	// Delete and purge
	deleteCmd := exec.CommandContext(ctx, "multipass", "delete", nodeName)
	if err := deleteCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete node %s: %w", nodeName, err)
	}

	purgeCmd := exec.CommandContext(ctx, "multipass", "purge")
	if err := purgeCmd.Run(); err != nil {
		return fmt.Errorf("failed to purge deleted nodes: %w", err)
	}

	return nil
}

// StartNode starts a stopped node
func (p *MultipassProvider) StartNode(ctx context.Context, nodeName string) error {
	cmd := exec.CommandContext(ctx, "multipass", "start", nodeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start node %s: %w", nodeName, err)
	}
	return nil
}

// StopNode stops a running node
func (p *MultipassProvider) StopNode(ctx context.Context, nodeName string) error {
	cmd := exec.CommandContext(ctx, "multipass", "stop", nodeName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop node %s: %w", nodeName, err)
	}
	return nil
}

// GetNodeStatus returns the current status of a node
func (p *MultipassProvider) GetNodeStatus(ctx context.Context, nodeName string) (*provider.NodeStatus, error) {
	cmd := exec.CommandContext(ctx, "multipass", "info", nodeName, "--format", "csv")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get node status for %s: %w", nodeName, err)
	}

	// Parse CSV output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("unexpected multipass info output")
	}

	// CSV format: Name,State,IPv4,IPv6,Release
	fields := strings.Split(lines[1], ",")
	if len(fields) < 3 {
		return nil, fmt.Errorf("unexpected multipass info CSV format")
	}

	status := &provider.NodeStatus{
		Name:  fields[0],
		State: fields[1],
		IP:    fields[2],
		Ready: fields[1] == "Running",
	}

	return status, nil
}

// ListNodes returns the status of all nodes
func (p *MultipassProvider) ListNodes(ctx context.Context) ([]provider.NodeStatus, error) {
	cmd := exec.CommandContext(ctx, "multipass", "list", "--format", "csv")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	var nodes []provider.NodeStatus
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Skip header line
	for i := 1; i < len(lines); i++ {
		fields := strings.Split(lines[i], ",")
		if len(fields) < 3 {
			continue
		}

		nodes = append(nodes, provider.NodeStatus{
			Name:  fields[0],
			State: fields[1],
			IP:    fields[2],
			Ready: fields[1] == "Running",
		})
	}

	return nodes, nil
}

// ExecuteCommand runs a command on a node
func (p *MultipassProvider) ExecuteCommand(ctx context.Context, nodeName string, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "multipass", "exec", nodeName, "--", "bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command on %s: %w", nodeName, err)
	}
	return string(output), nil
}

// CopyFile copies a file to a node
func (p *MultipassProvider) CopyFile(ctx context.Context, nodeName string, localPath string, remotePath string) error {
	source := localPath
	dest := fmt.Sprintf("%s:%s", nodeName, remotePath)

	cmd := exec.CommandContext(ctx, "multipass", "transfer", source, dest)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy file to %s: %w", nodeName, err)
	}
	return nil
}

// GetNodeIP returns the IP address of a node
func (p *MultipassProvider) GetNodeIP(ctx context.Context, nodeName string) (string, error) {
	status, err := p.GetNodeStatus(ctx, nodeName)
	if err != nil {
		return "", err
	}
	return status.IP, nil
}

// waitForCloudInit waits for cloud-init to complete on a node with exponential backoff
func (p *MultipassProvider) waitForCloudInit(ctx context.Context, nodeName string) error {
	maxAttempts := 60
	sleepDuration := 1 * time.Second // Start with 1s for faster detection
	maxSleep := 10 * time.Second

	for i := 0; i < maxAttempts; i++ {
		cmd := exec.CommandContext(ctx, "multipass", "exec", nodeName, "--", "cloud-init", "status")
		output, err := cmd.Output()

		if err == nil {
			status := strings.TrimSpace(string(output))

			// Check for running state (faster detection)
			if strings.Contains(status, "status: running") {
				sleepDuration = 2 * time.Second // Poll more frequently when running
			} else if strings.Contains(status, "status: done") {
				fmt.Printf("  ✓ %s is ready\n", nodeName)
				return nil
			}
		}

		if i > 0 && i%10 == 0 {
			fmt.Printf("  Waiting for cloud-init on %s... (%d/%d)\n", nodeName, i+1, maxAttempts)
		}

		time.Sleep(sleepDuration)

		// Exponential backoff with cap
		sleepDuration = time.Duration(float64(sleepDuration) * 1.5)
		if sleepDuration > maxSleep {
			sleepDuration = maxSleep
		}
	}
	return fmt.Errorf("timeout waiting for cloud-init on %s", nodeName)
}

// waitForPods waits for pods in a namespace with a specific label to be ready
func (p *MultipassProvider) waitForPods(ctx context.Context, controlPlane, namespace, labelSelector string, timeoutSeconds int) error {
	start := time.Now()
	timeout := time.Duration(timeoutSeconds) * time.Second

	for {
		cmd := exec.CommandContext(ctx, "multipass", "exec", controlPlane, "--",
			"kubectl", "get", "pods", "-n", namespace, "-l", labelSelector, "--no-headers")
		output, err := cmd.Output()

		if err == nil && len(strings.TrimSpace(string(output))) > 0 {
			lines := strings.Split(strings.TrimSpace(string(output)), "\n")
			allReady := true

			for _, line := range lines {
				fields := strings.Fields(line)
				if len(fields) < 3 {
					continue
				}
				// Check if pod is Running and ready (e.g., "1/1" or "2/2")
				status := fields[2]
				readyStatus := fields[1]
				if status != "Running" || !strings.Contains(readyStatus, "/") {
					allReady = false
					break
				}
				// Parse "1/1" format
				parts := strings.Split(readyStatus, "/")
				if len(parts) == 2 && parts[0] != parts[1] {
					allReady = false
					break
				}
			}

			if allReady && len(lines) > 0 {
				return nil
			}
		}

		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for pods in %s namespace with label %s", namespace, labelSelector)
		}

		time.Sleep(2 * time.Second)
	}
}

// getNodeIPFromMultipass retrieves node IP using multipass info JSON
func (p *MultipassProvider) getNodeIPFromMultipass(ctx context.Context, nodeName string) (string, error) {
	cmd := exec.CommandContext(ctx, "multipass", "info", nodeName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get node info: %w", err)
	}

	var info map[string]interface{}
	if err := json.Unmarshal(output, &info); err != nil {
		return "", fmt.Errorf("failed to parse multipass info: %w", err)
	}

	nodeInfo, ok := info["info"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid multipass info format")
	}

	nodeData, ok := nodeInfo[nodeName].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("node %s not found in info", nodeName)
	}

	ipv4List, ok := nodeData["ipv4"].([]interface{})
	if !ok || len(ipv4List) == 0 {
		return "", fmt.Errorf("no IPv4 address found for %s", nodeName)
	}

	ip, ok := ipv4List[0].(string)
	if !ok {
		return "", fmt.Errorf("invalid IP format for %s", nodeName)
	}

	return ip, nil
}

// initializeControlPlane runs kubeadm init on the control plane
func (p *MultipassProvider) initializeControlPlane(ctx context.Context, nodeName, podCIDR, controlPlaneIP string) (string, error) {
	fmt.Printf("  Initializing Kubernetes control plane...\n")

	initCmd := fmt.Sprintf(
		"sudo kubeadm init --pod-network-cidr=%s --apiserver-advertise-address=%s --control-plane-endpoint=%s",
		podCIDR, controlPlaneIP, controlPlaneIP,
	)

	cmd := exec.CommandContext(ctx, "multipass", "exec", nodeName, "--", "bash", "-c", initCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubeadm init failed: %w\nOutput: %s", err, string(output))
	}

	// Extract join command from output
	joinCommand, err := p.extractJoinCommand(string(output))
	if err != nil {
		return "", fmt.Errorf("failed to extract join command: %w", err)
	}

	// Setup kubeconfig on control plane
	kubeconfigSetup := `
		mkdir -p $HOME/.kube
		sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
		sudo chown $(id -u):$(id -g) $HOME/.kube/config
	`
	cmd = exec.CommandContext(ctx, "multipass", "exec", nodeName, "--", "bash", "-c", kubeconfigSetup)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to setup kubeconfig: %w", err)
	}

	fmt.Printf("  ✓ Control plane initialized\n")
	return joinCommand, nil
}

// extractJoinCommand extracts the kubeadm join command from kubeadm init output
func (p *MultipassProvider) extractJoinCommand(output string) (string, error) {
	joinRegex := regexp.MustCompile(`kubeadm join [^\n]+\n[^\n]+discovery-token-ca-cert-hash[^\n]+`)
	joinMatches := joinRegex.FindString(output)

	if joinMatches == "" {
		return "", fmt.Errorf("join command not found in kubeadm output")
	}

	// Clean up join command
	joinCommand := strings.ReplaceAll(joinMatches, "\\\n", "")
	joinCommand = strings.ReplaceAll(joinCommand, "\\", "")
	joinCommand = strings.TrimSpace(joinCommand)

	return joinCommand, nil
}

// setupLocalKubeconfig retrieves and configures local kubeconfig
func (p *MultipassProvider) setupLocalKubeconfig(ctx context.Context, controlPlane, clusterName, contextName, controlPlaneIP string) error {
	fmt.Printf("  Setting up local kubeconfig...\n")

	// Retrieve kubeconfig from control plane
	cmd := exec.CommandContext(ctx, "multipass", "exec", controlPlane, "--", "sudo", "cat", "/etc/kubernetes/admin.conf")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	kubeconfig := string(output)

	// Replace server address with control plane IP
	serverRegex := regexp.MustCompile(`server: https://[^:]+:`)
	kubeconfig = serverRegex.ReplaceAllString(kubeconfig, fmt.Sprintf("server: https://%s:", controlPlaneIP))

	// Replace cluster name
	kubeconfig = strings.ReplaceAll(kubeconfig, "name: kubernetes", fmt.Sprintf("name: %s", clusterName))
	kubeconfig = strings.ReplaceAll(kubeconfig, "cluster: kubernetes", fmt.Sprintf("cluster: %s", clusterName))

	// Replace context name
	kubeconfig = strings.ReplaceAll(kubeconfig, "name: kubernetes-admin@kubernetes", fmt.Sprintf("name: %s", contextName))
	kubeconfig = strings.ReplaceAll(kubeconfig, "context: kubernetes-admin@kubernetes", fmt.Sprintf("context: %s", contextName))
	kubeconfig = strings.ReplaceAll(kubeconfig, "current-context: kubernetes-admin@kubernetes", fmt.Sprintf("current-context: %s", contextName))

	// Write to temp file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	kubeconfigPath := filepath.Join(homeDir, ".kube", "config")
	tempKubeconfigPath := filepath.Join(homeDir, ".kube", fmt.Sprintf("config-%s", clusterName))

	if err := os.MkdirAll(filepath.Dir(kubeconfigPath), 0755); err != nil {
		return fmt.Errorf("failed to create .kube directory: %w", err)
	}

	if err := os.WriteFile(tempKubeconfigPath, []byte(kubeconfig), 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	// Merge with existing kubeconfig if it exists
	if _, err := os.Stat(kubeconfigPath); err == nil {
		// Backup existing kubeconfig
		backupPath := kubeconfigPath + ".backup." + time.Now().Format("20060102-150405")
		exec.Command("cp", kubeconfigPath, backupPath).Run()

		// Merge kubeconfigs
		mergeCmd := exec.Command("bash", "-c",
			fmt.Sprintf("KUBECONFIG=%s:%s kubectl config view --flatten > %s.merged && mv %s.merged %s",
				kubeconfigPath, tempKubeconfigPath, kubeconfigPath, kubeconfigPath, kubeconfigPath))
		if err := mergeCmd.Run(); err != nil {
			// If merge fails, just copy the new one
			exec.Command("cp", tempKubeconfigPath, kubeconfigPath).Run()
		}

		// Clean up temp file
		os.Remove(tempKubeconfigPath)
	} else {
		// No existing kubeconfig, just rename temp file
		if err := os.Rename(tempKubeconfigPath, kubeconfigPath); err != nil {
			return fmt.Errorf("failed to save kubeconfig: %w", err)
		}
	}

	// Set current context
	setContextCmd := exec.Command("kubectl", "config", "use-context", contextName)
	if err := setContextCmd.Run(); err != nil {
		fmt.Printf("  ⚠️  Warning: failed to set context: %v\n", err)
	}

	fmt.Printf("  ✓ Kubeconfig configured for context: %s\n", contextName)
	return nil
}

// installCNI installs the CNI network plugin
func (p *MultipassProvider) installCNI(ctx context.Context, controlPlane, cniType, cniVersion string) error {
	fmt.Printf("  Installing %s CNI...\n", cniType)

	var installCmd string
	switch strings.ToLower(cniType) {
	case "flannel":
		installCmd = "kubectl apply -f https://github.com/flannel-io/flannel/releases/latest/download/kube-flannel.yml"
	case "calico":
		installCmd = fmt.Sprintf("kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v%s/manifests/tigera-operator.yaml && "+
			"kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v%s/manifests/custom-resources.yaml", cniVersion, cniVersion)
	default:
		return fmt.Errorf("unsupported CNI type: %s", cniType)
	}

	cmd := exec.CommandContext(ctx, "multipass", "exec", controlPlane, "--", "bash", "-c", installCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install %s: %w\nOutput: %s", cniType, err, string(output))
	}

	// Wait for CNI to be ready using actual pod readiness checks
	fmt.Printf("  Waiting for %s pods to be ready...\n", cniType)

	var namespace, labelSelector string
	switch strings.ToLower(cniType) {
	case "flannel":
		namespace = "kube-flannel"
		labelSelector = "app=flannel"
	case "calico":
		namespace = "tigera-operator"
		labelSelector = "k8s-app=tigera-operator"
	}

	if err := p.waitForPods(ctx, controlPlane, namespace, labelSelector, 120); err != nil {
		return fmt.Errorf("CNI pods not ready: %w", err)
	}

	fmt.Printf("  ✓ %s installed\n", cniType)
	return nil
}

// joinWorker joins a worker node to the cluster
func (p *MultipassProvider) joinWorker(ctx context.Context, workerName, joinCommand string) error {
	cmd := exec.CommandContext(ctx, "multipass", "exec", workerName, "--", "sudo", "bash", "-c", joinCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to join worker %s: %w\nOutput: %s", workerName, err, string(output))
	}

	fmt.Printf("  ✓ %s joined successfully\n", workerName)
	return nil
}

// waitForNodeReady waits for a node to become Ready in Kubernetes with adaptive polling
func (p *MultipassProvider) waitForNodeReady(ctx context.Context, controlPlane, nodeName string, timeout time.Duration) error {
	start := time.Now()
	sleepDuration := 1 * time.Second // Start with faster polling

	for {
		cmd := exec.CommandContext(ctx, "multipass", "exec", controlPlane, "--",
			"kubectl", "get", "node", nodeName,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()

		if err == nil && strings.TrimSpace(string(output)) == "True" {
			return nil
		}

		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for node %s to be ready", nodeName)
		}

		time.Sleep(sleepDuration)

		// Adaptive polling: gradually increase interval
		if sleepDuration < 5*time.Second {
			sleepDuration += 500 * time.Millisecond
		}
	}
}

// DeployCluster creates and configures an entire cluster
func (p *MultipassProvider) DeployCluster(ctx context.Context, config provider.ClusterConfig) error {
	// Separate control plane and worker nodes
	var controlPlaneNode provider.NodeConfig
	var workerNodes []provider.NodeConfig

	for _, node := range config.Nodes {
		if node.Role == "control-plane" {
			controlPlaneNode = node
		} else {
			workerNodes = append(workerNodes, node)
		}
	}

	if controlPlaneNode.Name == "" {
		return fmt.Errorf("no control plane node found in cluster config")
	}

	// Step 1: Create control plane node
	fmt.Printf("Creating control plane: %s\n", controlPlaneNode.Name)
	if err := p.CreateNode(ctx, controlPlaneNode); err != nil {
		return fmt.Errorf("failed to create control plane: %w", err)
	}

	// Step 2: Wait for control plane cloud-init
	fmt.Printf("Waiting for control plane initialization...\n")
	if err := p.waitForCloudInit(ctx, controlPlaneNode.Name); err != nil {
		return err
	}

	// Step 3: Get control plane IP
	controlPlaneIP, err := p.getNodeIPFromMultipass(ctx, controlPlaneNode.Name)
	if err != nil {
		return fmt.Errorf("failed to get control plane IP: %w", err)
	}
	fmt.Printf("Control plane IP: %s\n", controlPlaneIP)

	// Step 4: Initialize Kubernetes on control plane
	joinCommand, err := p.initializeControlPlane(ctx, controlPlaneNode.Name, config.PodCIDR, controlPlaneIP)
	if err != nil {
		return err
	}

	// Step 5: Setup local kubeconfig
	if err := p.setupLocalKubeconfig(ctx, controlPlaneNode.Name, config.Name, config.Name, controlPlaneIP); err != nil {
		return err
	}

	// Step 6: Install CNI
	if err := p.installCNI(ctx, controlPlaneNode.Name, config.CNI, config.CNIVersion); err != nil {
		return err
	}

	// Step 7: Create worker nodes in parallel
	if len(workerNodes) > 0 {
		fmt.Printf("Creating worker nodes...\n")
		var wg sync.WaitGroup
		errChan := make(chan error, len(workerNodes))

		for _, worker := range workerNodes {
			wg.Add(1)
			go func(w provider.NodeConfig) {
				defer wg.Done()
				fmt.Printf("  Creating %s\n", w.Name)
				if err := p.CreateNode(ctx, w); err != nil {
					errChan <- fmt.Errorf("failed to create worker %s: %w", w.Name, err)
				}
			}(worker)
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			return err
		}

		// Step 8: Wait for worker cloud-init in parallel
		fmt.Printf("Waiting for workers to initialize...\n")
		errChan = make(chan error, len(workerNodes))

		for _, worker := range workerNodes {
			wg.Add(1)
			go func(w provider.NodeConfig) {
				defer wg.Done()
				if err := p.waitForCloudInit(ctx, w.Name); err != nil {
					errChan <- err
				}
			}(worker)
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			return err
		}

		// Step 9: Join workers in parallel
		fmt.Printf("Joining workers to cluster...\n")
		errChan = make(chan error, len(workerNodes))

		for _, worker := range workerNodes {
			wg.Add(1)
			go func(w provider.NodeConfig) {
				defer wg.Done()
				if err := p.joinWorker(ctx, w.Name, joinCommand); err != nil {
					errChan <- err
				}
			}(worker)
		}

		wg.Wait()
		close(errChan)

		for err := range errChan {
			return err
		}

		// Step 10: Wait for all nodes to be Ready
		fmt.Printf("Waiting for all nodes to be ready...\n")
		for _, worker := range workerNodes {
			if err := p.waitForNodeReady(ctx, controlPlaneNode.Name, worker.Name, 5*time.Minute); err != nil {
				return err
			}
			fmt.Printf("  ✓ %s is ready\n", worker.Name)
		}
	}

	fmt.Printf("✓ Cluster deployment complete\n")
	return nil
}

// DestroyCluster removes an entire cluster
func (p *MultipassProvider) DestroyCluster(ctx context.Context, clusterName string) error {
	// List all nodes
	nodes, err := p.ListNodes(ctx)
	if err != nil {
		return err
	}

	// Delete nodes that match the cluster prefix
	for _, node := range nodes {
		if strings.HasPrefix(node.Name, clusterName) {
			if err := p.DeleteNode(ctx, node.Name); err != nil {
				return err
			}
		}
	}

	return nil
}
