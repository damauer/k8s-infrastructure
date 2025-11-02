package native

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"k8s-infrastructure/pkg/provider"
)

// NativeProvider implements the Provider interface for bare metal deployments
type NativeProvider struct {
	config    map[string]interface{}
	inventory map[string]InventoryNode
	sshUser   string
}

// InventoryNode represents a physical node in the inventory
type InventoryNode struct {
	Hostname string
	IP       string
	Role     string
	Arch     string
	SSHUser  string
	SSHPort  int
}

// NewNativeProvider creates a new Native provider
func NewNativeProvider() *NativeProvider {
	return &NativeProvider{
		inventory: make(map[string]InventoryNode),
	}
}

// Name returns the provider name
func (p *NativeProvider) Name() string {
	return "native"
}

// Initialize sets up the provider with platform-specific configuration
func (p *NativeProvider) Initialize(ctx context.Context, config map[string]interface{}) error {
	p.config = config

	// Extract SSH user
	if user, ok := config["ssh_user"].(string); ok {
		p.sshUser = user
	} else {
		p.sshUser = "ubuntu"
	}

	// TODO: Load inventory from cluster-inventory.yaml
	// For now, this is a placeholder implementation

	return nil
}

// LoadInventory loads node inventory from configuration
func (p *NativeProvider) LoadInventory(nodes []InventoryNode) {
	for _, node := range nodes {
		p.inventory[node.Hostname] = node
	}
}

// CreateNode creates a single node with the given configuration
// For native deployments, this is a no-op as nodes already exist
func (p *NativeProvider) CreateNode(ctx context.Context, config provider.NodeConfig) error {
	// Native nodes are pre-existing physical hardware
	// This method verifies the node is accessible
	node, exists := p.inventory[config.Name]
	if !exists {
		return fmt.Errorf("node %s not found in inventory", config.Name)
	}

	// Verify SSH connectivity
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", node.SSHUser, node.IP),
		"echo", "ok")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to connect to node %s at %s: %w", config.Name, node.IP, err)
	}

	return nil
}

// DeleteNode removes a node
// For native deployments, this is a no-op as we don't destroy physical hardware
func (p *NativeProvider) DeleteNode(ctx context.Context, nodeName string) error {
	// Native nodes are not deleted, but we can reset them
	// This is a placeholder for potential cleanup operations
	return nil
}

// StartNode starts a stopped node
func (p *NativeProvider) StartNode(ctx context.Context, nodeName string) error {
	// For bare metal, nodes are typically always on
	// This could trigger a wake-on-LAN in the future
	return fmt.Errorf("start operation not supported for native nodes")
}

// StopNode stops a running node
func (p *NativeProvider) StopNode(ctx context.Context, nodeName string) error {
	node, exists := p.inventory[nodeName]
	if !exists {
		return fmt.Errorf("node %s not found in inventory", nodeName)
	}

	// Graceful shutdown via SSH
	cmd := exec.CommandContext(ctx, "ssh",
		fmt.Sprintf("%s@%s", node.SSHUser, node.IP),
		"sudo", "shutdown", "-h", "now")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop node %s: %w", nodeName, err)
	}

	return nil
}

// GetNodeStatus returns the current status of a node
func (p *NativeProvider) GetNodeStatus(ctx context.Context, nodeName string) (*provider.NodeStatus, error) {
	node, exists := p.inventory[nodeName]
	if !exists {
		return nil, fmt.Errorf("node %s not found in inventory", nodeName)
	}

	// Check if node is reachable via SSH
	cmd := exec.CommandContext(ctx, "ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", node.SSHUser, node.IP),
		"echo", "ok")

	status := &provider.NodeStatus{
		Name: nodeName,
		IP:   node.IP,
		Role: node.Role,
	}

	if err := cmd.Run(); err != nil {
		status.State = "Unreachable"
		status.Ready = false
	} else {
		status.State = "Running"
		status.Ready = true
	}

	return status, nil
}

// ListNodes returns the status of all nodes
func (p *NativeProvider) ListNodes(ctx context.Context) ([]provider.NodeStatus, error) {
	var nodes []provider.NodeStatus

	for name := range p.inventory {
		status, err := p.GetNodeStatus(ctx, name)
		if err != nil {
			continue
		}
		nodes = append(nodes, *status)
	}

	return nodes, nil
}

// ExecuteCommand runs a command on a node
func (p *NativeProvider) ExecuteCommand(ctx context.Context, nodeName string, command string) (string, error) {
	node, exists := p.inventory[nodeName]
	if !exists {
		return "", fmt.Errorf("node %s not found in inventory", nodeName)
	}

	cmd := exec.CommandContext(ctx, "ssh",
		fmt.Sprintf("%s@%s", node.SSHUser, node.IP),
		"bash", "-c", command)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to execute command on %s: %w", nodeName, err)
	}

	return string(output), nil
}

// CopyFile copies a file to a node
func (p *NativeProvider) CopyFile(ctx context.Context, nodeName string, localPath string, remotePath string) error {
	node, exists := p.inventory[nodeName]
	if !exists {
		return fmt.Errorf("node %s not found in inventory", nodeName)
	}

	dest := fmt.Sprintf("%s@%s:%s", node.SSHUser, node.IP, remotePath)

	cmd := exec.CommandContext(ctx, "scp",
		"-o", "StrictHostKeyChecking=no",
		localPath, dest)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy file to %s: %w", nodeName, err)
	}

	return nil
}

// GetNodeIP returns the IP address of a node
func (p *NativeProvider) GetNodeIP(ctx context.Context, nodeName string) (string, error) {
	node, exists := p.inventory[nodeName]
	if !exists {
		return "", fmt.Errorf("node %s not found in inventory", nodeName)
	}
	return node.IP, nil
}

// DeployCluster creates and configures an entire cluster
func (p *NativeProvider) DeployCluster(ctx context.Context, config provider.ClusterConfig) error {
	// Verify all nodes are accessible
	for _, nodeConfig := range config.Nodes {
		if err := p.CreateNode(ctx, nodeConfig); err != nil {
			return fmt.Errorf("failed to verify node %s: %w", nodeConfig.Name, err)
		}
	}

	return nil
}

// DestroyCluster removes an entire cluster
// For native deployments, this cleans up Kubernetes but doesn't destroy hardware
func (p *NativeProvider) DestroyCluster(ctx context.Context, clusterName string) error {
	// List all nodes that match the cluster prefix
	for name, node := range p.inventory {
		if strings.HasPrefix(name, clusterName) {
			// Run kubeadm reset on the node
			_, err := p.ExecuteCommand(ctx, name,
				"sudo kubeadm reset -f && sudo rm -rf /etc/kubernetes /var/lib/kubelet /var/lib/etcd")
			if err != nil {
				return fmt.Errorf("failed to reset node %s: %w", name, err)
			}

			// Clean up network interfaces
			_, _ = p.ExecuteCommand(ctx, name,
				"sudo ip link delete cni0 2>/dev/null || true")
			_, _ = p.ExecuteCommand(ctx, name,
				"sudo ip link delete flannel.1 2>/dev/null || true")

			fmt.Printf("Reset node %s (%s)\n", name, node.IP)
		}
	}

	return nil
}
