package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the main configuration file structure
type Config struct {
	Platforms    map[string]PlatformConfig    `yaml:"platforms"`
	Environments map[string]EnvironmentConfig `yaml:"environments"`
	Kubernetes   KubernetesConfig             `yaml:"kubernetes"`
	Networking   NetworkingConfig             `yaml:"networking"`
}

// PlatformConfig represents platform-specific configuration
type PlatformConfig struct {
	DeploymentMethod string `yaml:"deployment_method"`
	DefaultCPUs      string `yaml:"default_cpus"`
	DefaultMemory    string `yaml:"default_memory"`
	DefaultDisk      string `yaml:"default_disk"`
	VMDriver         string `yaml:"vm_driver"`
	UbuntuRelease    string `yaml:"ubuntu_release"`
	MultipassNetwork string `yaml:"multipass_network"`
	MultipassPath    string `yaml:"multipass_path"`
	SSHUser          string `yaml:"ssh_user"`
	SSHKeyPath       string `yaml:"ssh_key_path"`
	MaxNodes         int    `yaml:"max_nodes"`
}

// EnvironmentConfig represents environment-specific configuration
type EnvironmentConfig struct {
	ControlPlanes int    `yaml:"control_planes"`
	Workers       int    `yaml:"workers"`
	PodCIDR       string `yaml:"pod_cidr"`
	ServiceCIDR   string `yaml:"service_cidr"`
	ContextName   string `yaml:"context_name"`
	ClusterName   string `yaml:"cluster_name"`
}

// KubernetesConfig represents Kubernetes version configuration
type KubernetesConfig struct {
	Version string `yaml:"version"`
}

// NetworkingConfig represents networking configuration
type NetworkingConfig struct {
	CNI           string `yaml:"cni"`
	CalicoVersion string `yaml:"calico_version"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// GetPlatformConfig returns platform-specific configuration
func (c *Config) GetPlatformConfig(platform string) map[string]interface{} {
	platformCfg, exists := c.Platforms[platform]
	if !exists {
		return make(map[string]interface{})
	}

	// Convert struct to map for provider initialization
	return map[string]interface{}{
		"deployment_method": platformCfg.DeploymentMethod,
		"default_cpus":      platformCfg.DefaultCPUs,
		"default_memory":    platformCfg.DefaultMemory,
		"default_disk":      platformCfg.DefaultDisk,
		"vm_driver":         platformCfg.VMDriver,
		"ubuntu_release":    platformCfg.UbuntuRelease,
		"multipass_network": platformCfg.MultipassNetwork,
		"multipass_path":    platformCfg.MultipassPath,
		"ssh_user":          platformCfg.SSHUser,
		"ssh_key_path":      platformCfg.SSHKeyPath,
		"max_nodes":         platformCfg.MaxNodes,
	}
}

// GetEnvironmentConfig returns environment-specific configuration
func (c *Config) GetEnvironmentConfig(env string) (*EnvironmentConfig, error) {
	envCfg, exists := c.Environments[env]
	if !exists {
		return nil, fmt.Errorf("environment '%s' not found in configuration", env)
	}
	return &envCfg, nil
}

// GetKubernetesVersion returns the configured Kubernetes version
func (c *Config) GetKubernetesVersion() string {
	return c.Kubernetes.Version
}

// GetCNIConfig returns the CNI configuration
func (c *Config) GetCNIConfig() (string, string) {
	return c.Networking.CNI, c.Networking.CalicoVersion
}
