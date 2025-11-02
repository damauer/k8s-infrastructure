package provider

import (
	"context"
)

// NodeConfig represents the configuration for a single node
type NodeConfig struct {
	Name         string
	Role         string // control-plane or worker
	CPUs         string
	Memory       string
	Disk         string
	Platform     string
	Environment  string
	KubeVersion  string
	CloudInitPath string // Path to cloud-init config file (optional)
}

// ClusterConfig represents the configuration for an entire cluster
type ClusterConfig struct {
	Name         string
	Environment  string
	Nodes        []NodeConfig
	PodCIDR      string
	ServiceCIDR  string
	CNI          string
	CNIVersion   string
}

// NodeStatus represents the current status of a node
type NodeStatus struct {
	Name    string
	State   string // Running, Stopped, Deleted, Unknown
	IP      string
	Role    string
	Ready   bool
}

// Provider defines the interface that all deployment providers must implement
type Provider interface {
	// Name returns the provider name (e.g., "multipass", "native")
	Name() string

	// Initialize sets up the provider with platform-specific configuration
	Initialize(ctx context.Context, config map[string]interface{}) error

	// CreateNode creates a single node with the given configuration
	CreateNode(ctx context.Context, config NodeConfig) error

	// DeleteNode removes a node
	DeleteNode(ctx context.Context, nodeName string) error

	// StartNode starts a stopped node
	StartNode(ctx context.Context, nodeName string) error

	// StopNode stops a running node
	StopNode(ctx context.Context, nodeName string) error

	// GetNodeStatus returns the current status of a node
	GetNodeStatus(ctx context.Context, nodeName string) (*NodeStatus, error)

	// ListNodes returns the status of all nodes
	ListNodes(ctx context.Context) ([]NodeStatus, error)

	// ExecuteCommand runs a command on a node
	ExecuteCommand(ctx context.Context, nodeName string, command string) (string, error)

	// CopyFile copies a file to a node
	CopyFile(ctx context.Context, nodeName string, localPath string, remotePath string) error

	// GetNodeIP returns the IP address of a node
	GetNodeIP(ctx context.Context, nodeName string) (string, error)

	// DeployCluster creates and configures an entire cluster
	DeployCluster(ctx context.Context, config ClusterConfig) error

	// DestroyCluster removes an entire cluster
	DestroyCluster(ctx context.Context, clusterName string) error
}
