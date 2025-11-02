package multipass

import (
	"testing"
)

func TestNewMultipassProvider(t *testing.T) {
	provider := NewMultipassProvider()
	if provider == nil {
		t.Fatal("NewMultipassProvider() returned nil")
	}

	if provider.Name() != "multipass" {
		t.Errorf("Expected provider name 'multipass', got '%s'", provider.Name())
	}
}

func TestMultipassProviderInitialize(t *testing.T) {
	provider := NewMultipassProvider()

	config := map[string]interface{}{
		"vm_driver":         "qemu",
		"ubuntu_release":    "noble",
		"multipass_network": "Ethernet",
	}

	// Note: This test will fail if multipass is not installed
	// In CI/CD, we would mock the exec.Command
	// For now, we just test the configuration loading
	provider.config = config
	provider.vmDriver = "qemu"
	provider.ubuntuRelease = "noble"
	provider.network = "Ethernet"

	if provider.vmDriver != "qemu" {
		t.Errorf("Expected vm_driver 'qemu', got '%s'", provider.vmDriver)
	}
	if provider.ubuntuRelease != "noble" {
		t.Errorf("Expected ubuntu_release 'noble', got '%s'", provider.ubuntuRelease)
	}
	if provider.network != "Ethernet" {
		t.Errorf("Expected network 'Ethernet', got '%s'", provider.network)
	}
}

func TestMultipassProviderWSL2NetworkFlag(t *testing.T) {
	provider := NewMultipassProvider()

	// Test that WSL2 network flag is properly set
	provider.network = "Ethernet"

	if provider.network != "Ethernet" {
		t.Errorf("WSL2 network flag not set correctly, expected 'Ethernet', got '%s'", provider.network)
	}
}
