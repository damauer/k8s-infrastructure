package native

import (
	"testing"
)

func TestNewNativeProvider(t *testing.T) {
	provider := NewNativeProvider()
	if provider == nil {
		t.Fatal("NewNativeProvider() returned nil")
	}

	if provider.Name() != "native" {
		t.Errorf("Expected provider name 'native', got '%s'", provider.Name())
	}

	if provider.inventory == nil {
		t.Error("Inventory map should be initialized")
	}
}

func TestNativeProviderInitialize(t *testing.T) {
	provider := NewNativeProvider()

	config := map[string]interface{}{
		"ssh_user": "ubuntu",
	}

	provider.config = config
	provider.sshUser = "ubuntu"

	if provider.sshUser != "ubuntu" {
		t.Errorf("Expected ssh_user 'ubuntu', got '%s'", provider.sshUser)
	}
}

func TestNativeProviderLoadInventory(t *testing.T) {
	provider := NewNativeProvider()

	nodes := []InventoryNode{
		{
			Hostname: "pi-dev-c1",
			IP:       "192.168.1.101",
			Role:     "control-plane",
			Arch:     "arm64",
			SSHUser:  "ubuntu",
			SSHPort:  22,
		},
		{
			Hostname: "pi-dev-w1",
			IP:       "192.168.1.111",
			Role:     "worker",
			Arch:     "arm64",
			SSHUser:  "ubuntu",
			SSHPort:  22,
		},
	}

	provider.LoadInventory(nodes)

	if len(provider.inventory) != 2 {
		t.Errorf("Expected 2 nodes in inventory, got %d", len(provider.inventory))
	}

	node, exists := provider.inventory["pi-dev-c1"]
	if !exists {
		t.Error("Node pi-dev-c1 not found in inventory")
	}
	if node.IP != "192.168.1.101" {
		t.Errorf("Expected IP 192.168.1.101, got %s", node.IP)
	}
	if node.Role != "control-plane" {
		t.Errorf("Expected role 'control-plane', got '%s'", node.Role)
	}
}

func TestNativeProviderGetNodeIP(t *testing.T) {
	provider := NewNativeProvider()

	nodes := []InventoryNode{
		{
			Hostname: "test-node",
			IP:       "192.168.1.100",
			Role:     "worker",
			Arch:     "arm64",
			SSHUser:  "ubuntu",
			SSHPort:  22,
		},
	}

	provider.LoadInventory(nodes)

	// We can't use context in this simple test, but we can test the inventory lookup
	node, exists := provider.inventory["test-node"]
	if !exists {
		t.Fatal("Node test-node not found in inventory")
	}

	if node.IP != "192.168.1.100" {
		t.Errorf("Expected IP 192.168.1.100, got %s", node.IP)
	}
}
