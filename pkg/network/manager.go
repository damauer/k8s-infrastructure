package network

import (
	"fmt"
	"net"
)

// NetworkManager handles network configuration for Kubernetes clusters
type NetworkManager struct {
	platform     string
	deployMethod string
}

// NetworkConfig represents network configuration for a cluster
type NetworkConfig struct {
	PodCIDR        string
	ServiceCIDR    string
	DNSServiceIP   string
	ClusterDomain  string
	CNIType        string
	CNIVersion     string
	MTU            int
	IPFamily       string // ipv4, ipv6, dual
}

// CNIConfig represents CNI-specific configuration
type CNIConfig struct {
	Name    string
	Type    string
	Version string
	Config  map[string]interface{}
}

// NewNetworkManager creates a new network manager
func NewNetworkManager(platform, deployMethod string) *NetworkManager {
	return &NetworkManager{
		platform:     platform,
		deployMethod: deployMethod,
	}
}

// GetDefaultNetworkConfig returns default network configuration for the platform
func (m *NetworkManager) GetDefaultNetworkConfig(env string) *NetworkConfig {
	config := &NetworkConfig{
		ClusterDomain: "cluster.local",
		IPFamily:      "ipv4",
		MTU:           1450, // Safe default for most environments
	}

	// Environment-specific CIDR ranges
	switch env {
	case "dev":
		config.PodCIDR = "10.244.0.0/16"
		config.ServiceCIDR = "10.96.0.0/12"
	case "prd":
		config.PodCIDR = "10.245.0.0/16"
		config.ServiceCIDR = "10.97.0.0/12"
	default:
		config.PodCIDR = "10.244.0.0/16"
		config.ServiceCIDR = "10.96.0.0/12"
	}

	// Calculate DNS service IP (typically .10 in the service CIDR)
	config.DNSServiceIP = m.calculateDNSServiceIP(config.ServiceCIDR)

	// Platform-specific MTU adjustments
	config.MTU = m.getPlatformMTU()

	return config
}

// getPlatformMTU returns the appropriate MTU for the platform
func (m *NetworkManager) getPlatformMTU() int {
	switch m.deployMethod {
	case "multipass":
		// Multipass VMs typically need slightly lower MTU
		return 1450
	case "native":
		// Bare metal can use standard MTU
		return 1500
	default:
		return 1450
	}
}

// calculateDNSServiceIP calculates the DNS service IP from the service CIDR
func (m *NetworkManager) calculateDNSServiceIP(serviceCIDR string) string {
	ip, _, err := net.ParseCIDR(serviceCIDR)
	if err != nil {
		return "10.96.0.10" // fallback default
	}

	// Work with the parsed IP directly (which is the network address in CIDR notation)
	// Add 10 to the last octet for DNS service
	dnsIP := make(net.IP, len(ip))
	copy(dnsIP, ip)
	dnsIP[len(dnsIP)-1] += 10

	return dnsIP.String()
}

// GetCalicoConfig returns Calico-specific CNI configuration
func (m *NetworkManager) GetCalicoConfig(netConfig *NetworkConfig) *CNIConfig {
	config := &CNIConfig{
		Name:    "calico",
		Type:    "calico",
		Version: "3.28.0",
		Config: map[string]interface{}{
			"pod_cidr":     netConfig.PodCIDR,
			"mtu":          netConfig.MTU,
			"ip_family":    netConfig.IPFamily,
			"nat_outgoing": true,
			"ipam": map[string]interface{}{
				"type": "calico-ipam",
			},
		},
	}

	// Platform-specific Calico settings
	if m.deployMethod == "multipass" {
		config.Config["backend"] = "vxlan"
	} else {
		config.Config["backend"] = "bird"
	}

	return config
}

// GetFlannelConfig returns Flannel-specific CNI configuration
func (m *NetworkManager) GetFlannelConfig(netConfig *NetworkConfig) *CNIConfig {
	config := &CNIConfig{
		Name:    "flannel",
		Type:    "flannel",
		Version: "v0.24.0",
		Config: map[string]interface{}{
			"network":  netConfig.PodCIDR,
			"backend":  "vxlan",
			"mtu":      netConfig.MTU,
			"ip_family": netConfig.IPFamily,
		},
	}

	return config
}

// ValidateNetworkConfig validates network configuration
func (m *NetworkManager) ValidateNetworkConfig(config *NetworkConfig) error {
	// Validate Pod CIDR
	if _, _, err := net.ParseCIDR(config.PodCIDR); err != nil {
		return fmt.Errorf("invalid pod CIDR %s: %w", config.PodCIDR, err)
	}

	// Validate Service CIDR
	if _, _, err := net.ParseCIDR(config.ServiceCIDR); err != nil {
		return fmt.Errorf("invalid service CIDR %s: %w", config.ServiceCIDR, err)
	}

	// Validate DNS Service IP
	if net.ParseIP(config.DNSServiceIP) == nil {
		return fmt.Errorf("invalid DNS service IP: %s", config.DNSServiceIP)
	}

	// Validate CIDRs don't overlap
	if m.cidrsOverlap(config.PodCIDR, config.ServiceCIDR) {
		return fmt.Errorf("pod CIDR and service CIDR overlap")
	}

	// Validate MTU
	if config.MTU < 1280 || config.MTU > 9000 {
		return fmt.Errorf("invalid MTU %d: must be between 1280 and 9000", config.MTU)
	}

	return nil
}

// cidrsOverlap checks if two CIDR ranges overlap
func (m *NetworkManager) cidrsOverlap(cidr1, cidr2 string) bool {
	_, net1, err1 := net.ParseCIDR(cidr1)
	_, net2, err2 := net.ParseCIDR(cidr2)

	if err1 != nil || err2 != nil {
		return false
	}

	// Check if net1 contains net2's network address
	if net1.Contains(net2.IP) {
		return true
	}

	// Check if net2 contains net1's network address
	if net2.Contains(net1.IP) {
		return true
	}

	return false
}

// GetKubeProxyConfig returns kube-proxy configuration
func (m *NetworkManager) GetKubeProxyConfig(netConfig *NetworkConfig) map[string]interface{} {
	config := map[string]interface{}{
		"cluster_cidr": netConfig.PodCIDR,
		"mode":         "iptables", // Default mode
	}

	// Platform-specific kube-proxy settings
	if m.platform == "linux-arm64-pi" {
		// Raspberry Pi might benefit from different settings
		config["conntrack_max_per_core"] = 32768
	}

	return config
}

// GetNetworkPolicyConfig returns network policy configuration
func (m *NetworkManager) GetNetworkPolicyConfig() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        true,
		"default_policy": "allow", // Can be changed to "deny" for stricter security
	}
}

// GenerateKubeadmNetworkConfig generates kubeadm-specific network configuration
func (m *NetworkManager) GenerateKubeadmNetworkConfig(netConfig *NetworkConfig) map[string]interface{} {
	return map[string]interface{}{
		"networking": map[string]interface{}{
			"podSubnet":     netConfig.PodCIDR,
			"serviceSubnet": netConfig.ServiceCIDR,
			"dnsDomain":     netConfig.ClusterDomain,
		},
	}
}

// GetNodePortRange returns the NodePort range for services
func (m *NetworkManager) GetNodePortRange() string {
	return "30000-32767" // Standard Kubernetes NodePort range
}

// SupportsIPv6 returns whether the platform supports IPv6
func (m *NetworkManager) SupportsIPv6() bool {
	// Most modern platforms support IPv6, but WSL2 can be tricky
	if m.platform == "linux-amd64-wsl2" || m.platform == "linux-arm64-wsl2" {
		return false // Conservative default for WSL2
	}
	return true
}

// SupportsDualStack returns whether the platform supports dual-stack networking
func (m *NetworkManager) SupportsDualStack() bool {
	// Dual-stack requires Kubernetes 1.23+
	// WSL2 and older platforms might have issues
	if m.platform == "linux-amd64-wsl2" || m.platform == "linux-arm64-wsl2" {
		return false
	}
	return true
}
