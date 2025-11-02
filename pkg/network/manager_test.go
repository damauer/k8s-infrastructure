package network

import (
	"strings"
	"testing"
)

func TestNewNetworkManager(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")
	if manager == nil {
		t.Fatal("NewNetworkManager() returned nil")
	}

	if manager.platform != "darwin-arm64" {
		t.Errorf("Expected platform 'darwin-arm64', got '%s'", manager.platform)
	}

	if manager.deployMethod != "multipass" {
		t.Errorf("Expected deployMethod 'multipass', got '%s'", manager.deployMethod)
	}
}

func TestGetDefaultNetworkConfig(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")

	tests := []struct {
		name        string
		env         string
		expectedPod string
		expectedSvc string
	}{
		{"dev environment", "dev", "10.244.0.0/16", "10.96.0.0/12"},
		{"prd environment", "prd", "10.245.0.0/16", "10.97.0.0/12"},
		{"default environment", "test", "10.244.0.0/16", "10.96.0.0/12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := manager.GetDefaultNetworkConfig(tt.env)

			if config.PodCIDR != tt.expectedPod {
				t.Errorf("Expected PodCIDR '%s', got '%s'", tt.expectedPod, config.PodCIDR)
			}

			if config.ServiceCIDR != tt.expectedSvc {
				t.Errorf("Expected ServiceCIDR '%s', got '%s'", tt.expectedSvc, config.ServiceCIDR)
			}

			if config.ClusterDomain != "cluster.local" {
				t.Errorf("Expected ClusterDomain 'cluster.local', got '%s'", config.ClusterDomain)
			}

			if config.DNSServiceIP == "" {
				t.Error("DNSServiceIP should not be empty")
			}
		})
	}
}

func TestGetPlatformMTU(t *testing.T) {
	tests := []struct {
		name         string
		deployMethod string
		expectedMTU  int
	}{
		{"Multipass deployment", "multipass", 1450},
		{"Native deployment", "native", 1500},
		{"Unknown deployment", "unknown", 1450},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewNetworkManager("linux-arm64", tt.deployMethod)
			mtu := manager.getPlatformMTU()

			if mtu != tt.expectedMTU {
				t.Errorf("Expected MTU %d, got %d", tt.expectedMTU, mtu)
			}
		})
	}
}

func TestCalculateDNSServiceIP(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")

	tests := []struct {
		name        string
		serviceCIDR string
		expectedIP  string
	}{
		{"Standard service CIDR", "10.96.0.0/12", "10.96.0.10"},
		{"Alternate service CIDR", "10.97.0.0/12", "10.97.0.10"},
		{"Custom service CIDR", "172.16.0.0/16", "172.16.0.10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := manager.calculateDNSServiceIP(tt.serviceCIDR)

			if ip != tt.expectedIP {
				t.Errorf("Expected DNS IP '%s', got '%s'", tt.expectedIP, ip)
			}
		})
	}
}

func TestCalculateDNSServiceIPInvalid(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")
	ip := manager.calculateDNSServiceIP("invalid-cidr")

	// Should return fallback default
	if ip != "10.96.0.10" {
		t.Errorf("Expected fallback IP '10.96.0.10', got '%s'", ip)
	}
}

func TestGetCalicoConfig(t *testing.T) {
	manager := NewNetworkManager("linux-arm64", "multipass")
	netConfig := &NetworkConfig{
		PodCIDR:  "10.244.0.0/16",
		MTU:      1450,
		IPFamily: "ipv4",
	}

	calicoConfig := manager.GetCalicoConfig(netConfig)

	if calicoConfig.Name != "calico" {
		t.Errorf("Expected name 'calico', got '%s'", calicoConfig.Name)
	}

	if calicoConfig.Type != "calico" {
		t.Errorf("Expected type 'calico', got '%s'", calicoConfig.Type)
	}

	if calicoConfig.Config["pod_cidr"] != netConfig.PodCIDR {
		t.Error("pod_cidr not set correctly in config")
	}

	if calicoConfig.Config["mtu"] != netConfig.MTU {
		t.Error("MTU not set correctly in config")
	}

	// Multipass should use vxlan backend
	if calicoConfig.Config["backend"] != "vxlan" {
		t.Errorf("Expected backend 'vxlan' for multipass, got '%v'", calicoConfig.Config["backend"])
	}
}

func TestGetCalicoConfigNative(t *testing.T) {
	manager := NewNetworkManager("linux-arm64-pi", "native")
	netConfig := &NetworkConfig{
		PodCIDR:  "10.244.0.0/16",
		MTU:      1500,
		IPFamily: "ipv4",
	}

	calicoConfig := manager.GetCalicoConfig(netConfig)

	// Native should use bird backend
	if calicoConfig.Config["backend"] != "bird" {
		t.Errorf("Expected backend 'bird' for native, got '%v'", calicoConfig.Config["backend"])
	}
}

func TestGetFlannelConfig(t *testing.T) {
	manager := NewNetworkManager("linux-amd64", "multipass")
	netConfig := &NetworkConfig{
		PodCIDR:  "10.244.0.0/16",
		MTU:      1450,
		IPFamily: "ipv4",
	}

	flannelConfig := manager.GetFlannelConfig(netConfig)

	if flannelConfig.Name != "flannel" {
		t.Errorf("Expected name 'flannel', got '%s'", flannelConfig.Name)
	}

	if flannelConfig.Config["network"] != netConfig.PodCIDR {
		t.Error("network not set correctly in config")
	}

	if flannelConfig.Config["backend"] != "vxlan" {
		t.Error("backend should be vxlan for flannel")
	}
}

func TestValidateNetworkConfig(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")

	tests := []struct {
		name      string
		config    *NetworkConfig
		shouldErr bool
	}{
		{
			name: "Valid config",
			config: &NetworkConfig{
				PodCIDR:      "10.244.0.0/16",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "10.96.0.10",
				MTU:          1450,
			},
			shouldErr: false,
		},
		{
			name: "Invalid pod CIDR",
			config: &NetworkConfig{
				PodCIDR:      "invalid",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "10.96.0.10",
				MTU:          1450,
			},
			shouldErr: true,
		},
		{
			name: "Invalid service CIDR",
			config: &NetworkConfig{
				PodCIDR:      "10.244.0.0/16",
				ServiceCIDR:  "invalid",
				DNSServiceIP: "10.96.0.10",
				MTU:          1450,
			},
			shouldErr: true,
		},
		{
			name: "Invalid DNS IP",
			config: &NetworkConfig{
				PodCIDR:      "10.244.0.0/16",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "invalid",
				MTU:          1450,
			},
			shouldErr: true,
		},
		{
			name: "Overlapping CIDRs",
			config: &NetworkConfig{
				PodCIDR:      "10.96.0.0/16",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "10.96.0.10",
				MTU:          1450,
			},
			shouldErr: true,
		},
		{
			name: "MTU too low",
			config: &NetworkConfig{
				PodCIDR:      "10.244.0.0/16",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "10.96.0.10",
				MTU:          1000,
			},
			shouldErr: true,
		},
		{
			name: "MTU too high",
			config: &NetworkConfig{
				PodCIDR:      "10.244.0.0/16",
				ServiceCIDR:  "10.96.0.0/12",
				DNSServiceIP: "10.96.0.10",
				MTU:          10000,
			},
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateNetworkConfig(tt.config)

			if tt.shouldErr && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestCIDRsOverlap(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")

	tests := []struct {
		name     string
		cidr1    string
		cidr2    string
		overlaps bool
	}{
		{"No overlap", "10.244.0.0/16", "10.96.0.0/12", false},
		{"Complete overlap", "10.96.0.0/16", "10.96.0.0/12", true},
		{"Partial overlap", "10.96.0.0/16", "10.96.128.0/17", true},
		{"Different networks", "192.168.0.0/16", "172.16.0.0/16", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.cidrsOverlap(tt.cidr1, tt.cidr2)

			if result != tt.overlaps {
				t.Errorf("Expected overlap=%v for %s and %s, got %v",
					tt.overlaps, tt.cidr1, tt.cidr2, result)
			}
		})
	}
}

func TestGetKubeProxyConfig(t *testing.T) {
	tests := []struct {
		name     string
		platform string
	}{
		{"macOS platform", "darwin-arm64"},
		{"Raspberry Pi platform", "linux-arm64-pi"},
		{"WSL2 platform", "linux-amd64-wsl2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewNetworkManager(tt.platform, "multipass")
			netConfig := &NetworkConfig{PodCIDR: "10.244.0.0/16"}

			config := manager.GetKubeProxyConfig(netConfig)

			if config["cluster_cidr"] != netConfig.PodCIDR {
				t.Error("cluster_cidr not set correctly")
			}

			if config["mode"] != "iptables" {
				t.Error("mode should be iptables")
			}

			// Raspberry Pi should have conntrack_max_per_core
			if strings.Contains(tt.platform, "pi") {
				if config["conntrack_max_per_core"] == nil {
					t.Error("Pi platform should have conntrack_max_per_core set")
				}
			}
		})
	}
}

func TestGenerateKubeadmNetworkConfig(t *testing.T) {
	manager := NewNetworkManager("linux-arm64", "multipass")
	netConfig := &NetworkConfig{
		PodCIDR:       "10.244.0.0/16",
		ServiceCIDR:   "10.96.0.0/12",
		ClusterDomain: "cluster.local",
	}

	config := manager.GenerateKubeadmNetworkConfig(netConfig)

	networking, ok := config["networking"].(map[string]interface{})
	if !ok {
		t.Fatal("networking section not found or wrong type")
	}

	if networking["podSubnet"] != netConfig.PodCIDR {
		t.Error("podSubnet not set correctly")
	}

	if networking["serviceSubnet"] != netConfig.ServiceCIDR {
		t.Error("serviceSubnet not set correctly")
	}

	if networking["dnsDomain"] != netConfig.ClusterDomain {
		t.Error("dnsDomain not set correctly")
	}
}

func TestGetNodePortRange(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")
	portRange := manager.GetNodePortRange()

	if portRange != "30000-32767" {
		t.Errorf("Expected NodePort range '30000-32767', got '%s'", portRange)
	}
}

func TestSupportsIPv6(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		expected bool
	}{
		{"macOS supports IPv6", "darwin-arm64", true},
		{"Pi supports IPv6", "linux-arm64-pi", true},
		{"WSL2 AMD64 conservative", "linux-amd64-wsl2", false},
		{"WSL2 ARM64 conservative", "linux-arm64-wsl2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewNetworkManager(tt.platform, "multipass")
			result := manager.SupportsIPv6()

			if result != tt.expected {
				t.Errorf("Expected IPv6 support %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSupportsDualStack(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		expected bool
	}{
		{"macOS supports dual-stack", "darwin-arm64", true},
		{"Pi supports dual-stack", "linux-arm64-pi", true},
		{"WSL2 AMD64 no dual-stack", "linux-amd64-wsl2", false},
		{"WSL2 ARM64 no dual-stack", "linux-arm64-wsl2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewNetworkManager(tt.platform, "multipass")
			result := manager.SupportsDualStack()

			if result != tt.expected {
				t.Errorf("Expected dual-stack support %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetNetworkPolicyConfig(t *testing.T) {
	manager := NewNetworkManager("darwin-arm64", "multipass")
	config := manager.GetNetworkPolicyConfig()

	if config["enabled"] != true {
		t.Error("Network policy should be enabled")
	}

	if config["default_policy"] != "allow" {
		t.Error("Default policy should be 'allow'")
	}
}
