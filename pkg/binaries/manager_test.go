package binaries

import (
	"strings"
	"testing"
)

func TestNewBinaryManager(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "")
	if manager == nil {
		t.Fatal("NewBinaryManager() returned nil")
	}

	if manager.os != "linux" {
		t.Errorf("Expected os 'linux', got '%s'", manager.os)
	}

	if manager.arch != "arm64" {
		t.Errorf("Expected arch 'arm64', got '%s'", manager.arch)
	}

	if manager.cacheDir != "/tmp/k8s-binaries" {
		t.Errorf("Expected default cache dir '/tmp/k8s-binaries', got '%s'", manager.cacheDir)
	}
}

func TestNewBinaryManagerCustomCache(t *testing.T) {
	manager := NewBinaryManager("linux", "amd64", "/custom/cache")
	if manager.cacheDir != "/custom/cache" {
		t.Errorf("Expected custom cache dir '/custom/cache', got '%s'", manager.cacheDir)
	}
}

func TestGetBinaryInfo(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "")
	binary := manager.GetBinaryInfo("kubectl", "v1.30.0")

	if binary == nil {
		t.Fatal("GetBinaryInfo() returned nil")
	}

	if binary.Name != "kubectl" {
		t.Errorf("Expected binary name 'kubectl', got '%s'", binary.Name)
	}

	if binary.Version != "v1.30.0" {
		t.Errorf("Expected version 'v1.30.0', got '%s'", binary.Version)
	}

	if !strings.Contains(binary.URL, "kubectl") {
		t.Errorf("Binary URL should contain 'kubectl', got '%s'", binary.URL)
	}

	if !strings.Contains(binary.URL, "arm64") {
		t.Errorf("Binary URL should contain 'arm64', got '%s'", binary.URL)
	}
}

func TestGetKubernetesBinaries(t *testing.T) {
	manager := NewBinaryManager("linux", "amd64", "")
	binaries := manager.GetKubernetesBinaries("v1.30.0")

	if len(binaries) == 0 {
		t.Fatal("GetKubernetesBinaries() returned empty list")
	}

	// Check for essential binaries
	essentialBinaries := []string{"kubectl", "kubeadm", "kubelet"}
	for _, essential := range essentialBinaries {
		found := false
		for _, binary := range binaries {
			if binary.Name == essential {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Essential binary '%s' not found", essential)
		}
	}
}

func TestNormalizeArch(t *testing.T) {
	manager := NewBinaryManager("linux", "amd64", "")

	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "amd64"},
		{"amd64", "amd64"},
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
		{"armv7l", "arm"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := manager.normalizeArch(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeArch(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeOS(t *testing.T) {
	manager := NewBinaryManager("linux", "amd64", "")

	tests := []struct {
		input    string
		expected string
	}{
		{"darwin", "darwin"},
		{"linux", "linux"},
		{"windows", "windows"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := manager.normalizeOS(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeOS(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSupportsArchitecture(t *testing.T) {
	tests := []struct {
		name     string
		arch     string
		expected bool
	}{
		{"amd64 supported", "amd64", true},
		{"arm64 supported", "arm64", true},
		{"x86_64 supported", "x86_64", true},
		{"aarch64 supported", "aarch64", true},
		{"arm unsupported", "arm", false},
		{"unknown unsupported", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewBinaryManager("linux", tt.arch, "")
			result := manager.SupportsArchitecture()
			if result != tt.expected {
				t.Errorf("Expected %v for arch '%s', got %v", tt.expected, tt.arch, result)
			}
		})
	}
}

func TestGetCachedPath(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "/cache")
	binary := &BinaryInfo{
		Name:    "kubectl",
		Version: "v1.30.0",
		OS:      "linux",
		Arch:    "arm64",
	}

	path := manager.GetCachedPath(binary)
	expected := "/cache/v1.30.0/linux/arm64/kubectl"

	if path != expected {
		t.Errorf("Expected cached path '%s', got '%s'", expected, path)
	}
}

func TestGetDownloadCommand(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "/cache")
	binary := &BinaryInfo{
		Name:    "kubectl",
		Version: "v1.30.0",
		OS:      "linux",
		Arch:    "arm64",
		URL:     "https://example.com/kubectl",
	}

	cmd := manager.GetDownloadCommand(binary)

	if !strings.Contains(cmd, "curl") {
		t.Error("Download command should contain 'curl'")
	}

	if !strings.Contains(cmd, binary.URL) {
		t.Error("Download command should contain binary URL")
	}

	if !strings.Contains(cmd, "chmod +x") {
		t.Error("Download command should contain 'chmod +x'")
	}
}

func TestGetAllRequiredBinaries(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "")
	binaries := manager.GetAllRequiredBinaries("v1.30.0", "v1.4.0", "v1.29.0", "1.7.11", "v1.1.10")

	if len(binaries) == 0 {
		t.Fatal("GetAllRequiredBinaries() returned empty list")
	}

	// Should include kubectl, kubeadm, kubelet at minimum
	if len(binaries) < 3 {
		t.Errorf("Expected at least 3 binaries, got %d", len(binaries))
	}

	// For Linux, should include CNI plugins, containerd, etc.
	if manager.os == "linux" && len(binaries) < 7 {
		t.Errorf("Expected at least 7 binaries for Linux, got %d", len(binaries))
	}
}

func TestGetCNIPluginsBinaries(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "")
	binaries := manager.GetCNIPluginsBinaries("v1.4.0")

	if len(binaries) == 0 {
		t.Fatal("GetCNIPluginsBinaries() returned empty list")
	}

	if binaries[0].Name != "cni-plugins" {
		t.Errorf("Expected binary name 'cni-plugins', got '%s'", binaries[0].Name)
	}

	if !strings.Contains(binaries[0].URL, "cni-plugins") {
		t.Error("URL should contain 'cni-plugins'")
	}
}

func TestGetContainerdBinaries(t *testing.T) {
	manager := NewBinaryManager("linux", "amd64", "")
	binaries := manager.GetContainerdBinaries("1.7.11")

	if len(binaries) == 0 {
		t.Fatal("GetContainerdBinaries() returned empty list")
	}

	if binaries[0].Name != "containerd" {
		t.Errorf("Expected binary name 'containerd', got '%s'", binaries[0].Name)
	}

	if !strings.Contains(binaries[0].URL, "containerd") {
		t.Error("URL should contain 'containerd'")
	}
}

func TestGetRuncBinaries(t *testing.T) {
	manager := NewBinaryManager("linux", "arm64", "")
	binaries := manager.GetRuncBinaries("v1.1.10")

	if len(binaries) == 0 {
		t.Fatal("GetRuncBinaries() returned empty list")
	}

	if binaries[0].Name != "runc" {
		t.Errorf("Expected binary name 'runc', got '%s'", binaries[0].Name)
	}

	if !strings.Contains(binaries[0].URL, "runc") {
		t.Error("URL should contain 'runc'")
	}
}
