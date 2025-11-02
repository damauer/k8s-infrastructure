package multipass

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

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

	// Add network for WSL2
	if p.network != "" {
		args = append(args, "--network", p.network)
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

// DeployCluster creates and configures an entire cluster
func (p *MultipassProvider) DeployCluster(ctx context.Context, config provider.ClusterConfig) error {
	// Create all nodes
	for _, nodeConfig := range config.Nodes {
		if err := p.CreateNode(ctx, nodeConfig); err != nil {
			return fmt.Errorf("failed to create node %s: %w", nodeConfig.Name, err)
		}
	}

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
