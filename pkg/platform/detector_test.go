package platform

import (
	"testing"
)

func TestDetect(t *testing.T) {
	p, err := Detect()
	if err != nil {
		t.Fatalf("Detect() failed: %v", err)
	}

	if p.OS == "" {
		t.Error("OS should not be empty")
	}
	if p.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if p.DeployMethod == "" {
		t.Error("DeployMethod should not be empty")
	}
	if p.Key == "" {
		t.Error("Key should not be empty")
	}

	t.Logf("Detected platform: %s", p.String())
	t.Logf("Platform key: %s", p.Key)
}

func TestBuildKey(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		expected string
	}{
		{
			name: "macOS ARM64",
			platform: Platform{
				OS:     "darwin",
				Arch:   "arm64",
				IsWSL:  false,
				IsPi:   false,
			},
			expected: "darwin-arm64",
		},
		{
			name: "WSL2 AMD64",
			platform: Platform{
				OS:     "linux",
				Arch:   "amd64",
				IsWSL:  true,
				IsPi:   false,
			},
			expected: "linux-amd64-wsl2",
		},
		{
			name: "Raspberry Pi",
			platform: Platform{
				OS:     "linux",
				Arch:   "arm64",
				IsWSL:  false,
				IsPi:   true,
			},
			expected: "linux-arm64-pi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := tt.platform.buildKey()
			if key != tt.expected {
				t.Errorf("buildKey() = %s, want %s", key, tt.expected)
			}
		})
	}
}

func TestSupportsMultipass(t *testing.T) {
	tests := []struct {
		name     string
		platform Platform
		expected bool
	}{
		{
			name: "Multipass platform",
			platform: Platform{
				DeployMethod: "multipass",
			},
			expected: true,
		},
		{
			name: "Native platform",
			platform: Platform{
				DeployMethod: "native",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.platform.SupportsMultipass()
			if result != tt.expected {
				t.Errorf("SupportsMultipass() = %v, want %v", result, tt.expected)
			}
		})
	}
}
