package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Platform represents the detected platform information
type Platform struct {
	OS           string // darwin, linux, windows
	Arch         string // amd64, arm64
	DeployMethod string // multipass, native
	IsWSL        bool
	IsPi         bool
	Key          string // Combined key for config lookup
}

// Detect automatically detects the current platform
func Detect() (*Platform, error) {
	p := &Platform{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}

	// Detect WSL
	if p.OS == "linux" {
		if isWSL() {
			p.IsWSL = true
		}
	}

	// Detect Raspberry Pi
	if p.OS == "linux" && p.Arch == "arm64" {
		if isPi() {
			p.IsPi = true
		}
	}

	// Determine deployment method
	if p.IsPi {
		p.DeployMethod = "native"
	} else {
		p.DeployMethod = "multipass"
	}

	// Build platform key for config lookup
	p.Key = p.buildKey()

	return p, nil
}

// isWSL checks if we're running in WSL
func isWSL() bool {
	// Check for WSL interop file
	if _, err := os.Stat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
		return true
	}

	// Check /proc/version for WSL/Microsoft
	if data, err := os.ReadFile("/proc/version"); err == nil {
		content := strings.ToLower(string(data))
		if strings.Contains(content, "microsoft") || strings.Contains(content, "wsl") {
			return true
		}
	}

	return false
}

// isPi checks if we're running on a Raspberry Pi
func isPi() bool {
	// Check /proc/cpuinfo for Raspberry Pi
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		content := string(data)
		// Pi 5 has BCM2712, Pi 4 has BCM2711
		if strings.Contains(content, "BCM2712") || strings.Contains(content, "BCM2711") {
			return true
		}
		// Also check for "Raspberry Pi" in model
		if strings.Contains(content, "Raspberry Pi") {
			return true
		}
	}

	// Check /proc/device-tree/model
	if data, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		if strings.Contains(string(data), "Raspberry Pi") {
			return true
		}
	}

	return false
}

// buildKey creates a configuration lookup key
func (p *Platform) buildKey() string {
	if p.IsPi {
		return fmt.Sprintf("%s-%s-pi", p.OS, p.Arch)
	}
	if p.IsWSL {
		return fmt.Sprintf("%s-%s-wsl2", p.OS, p.Arch)
	}
	return fmt.Sprintf("%s-%s", p.OS, p.Arch)
}

// String returns a human-readable platform description
func (p *Platform) String() string {
	details := []string{fmt.Sprintf("%s/%s", p.OS, p.Arch)}

	if p.IsWSL {
		details = append(details, "WSL2")
	}
	if p.IsPi {
		details = append(details, "Raspberry Pi")
	}
	details = append(details, fmt.Sprintf("deploy=%s", p.DeployMethod))

	return strings.Join(details, ", ")
}

// SupportsMultipass returns true if the platform supports Multipass VMs
func (p *Platform) SupportsMultipass() bool {
	return p.DeployMethod == "multipass"
}

// SupportsNative returns true if the platform supports native/bare metal deployment
func (p *Platform) SupportsNative() bool {
	return p.DeployMethod == "native"
}
