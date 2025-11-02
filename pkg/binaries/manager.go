package binaries

import (
	"fmt"
	"path/filepath"
)

// BinaryManager handles Kubernetes binary downloads and caching
type BinaryManager struct {
	arch      string
	os        string
	cacheDir  string
	baseURL   string
}

// BinaryInfo represents information about a Kubernetes binary
type BinaryInfo struct {
	Name     string
	Version  string
	OS       string
	Arch     string
	URL      string
	Checksum string
}

// NewBinaryManager creates a new binary manager
func NewBinaryManager(os, arch, cacheDir string) *BinaryManager {
	if cacheDir == "" {
		cacheDir = "/tmp/k8s-binaries"
	}
	return &BinaryManager{
		os:       os,
		arch:     arch,
		cacheDir: cacheDir,
		baseURL:  "https://dl.k8s.io/release",
	}
}

// GetBinaryInfo returns information about a specific binary
func (m *BinaryManager) GetBinaryInfo(name, version string) *BinaryInfo {
	// Normalize architecture naming for Kubernetes downloads
	arch := m.normalizeArch(m.arch)
	os := m.normalizeOS(m.os)

	url := fmt.Sprintf("%s/%s/bin/%s/%s/%s", m.baseURL, version, os, arch, name)

	return &BinaryInfo{
		Name:    name,
		Version: version,
		OS:      os,
		Arch:    arch,
		URL:     url,
	}
}

// GetKubernetesBinaries returns the list of core Kubernetes binaries
func (m *BinaryManager) GetKubernetesBinaries(version string) []*BinaryInfo {
	binaries := []string{
		"kubectl",
		"kubeadm",
		"kubelet",
	}

	var result []*BinaryInfo
	for _, binary := range binaries {
		result = append(result, m.GetBinaryInfo(binary, version))
	}

	return result
}

// GetCNIPluginsBinaries returns CNI plugin binaries
func (m *BinaryManager) GetCNIPluginsBinaries(version string) []*BinaryInfo {
	// CNI plugins use a different URL structure
	arch := m.normalizeArch(m.arch)
	os := m.normalizeOS(m.os)

	url := fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-%s-%s-%s.tgz",
		version, os, arch, version)

	return []*BinaryInfo{
		{
			Name:    "cni-plugins",
			Version: version,
			OS:      os,
			Arch:    arch,
			URL:     url,
		},
	}
}

// GetCRIToolsBinaries returns CRI tools (crictl)
func (m *BinaryManager) GetCRIToolsBinaries(version string) []*BinaryInfo {
	arch := m.normalizeArch(m.arch)
	os := m.normalizeOS(m.os)

	url := fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-%s-%s-%s.tar.gz",
		version, version, os, arch)

	return []*BinaryInfo{
		{
			Name:    "crictl",
			Version: version,
			OS:      os,
			Arch:    arch,
			URL:     url,
		},
	}
}

// GetContainerdBinaries returns containerd binaries
func (m *BinaryManager) GetContainerdBinaries(version string) []*BinaryInfo {
	arch := m.normalizeArch(m.arch)

	// Containerd uses different architecture naming
	containerdArch := arch
	if arch == "amd64" {
		containerdArch = "amd64"
	} else if arch == "arm64" {
		containerdArch = "arm64"
	}

	url := fmt.Sprintf("https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-%s.tar.gz",
		version, version, containerdArch)

	return []*BinaryInfo{
		{
			Name:    "containerd",
			Version: version,
			OS:      "linux",
			Arch:    arch,
			URL:     url,
		},
	}
}

// GetRuncBinaries returns runc binaries
func (m *BinaryManager) GetRuncBinaries(version string) []*BinaryInfo {
	arch := m.normalizeArch(m.arch)

	url := fmt.Sprintf("https://github.com/opencontainers/runc/releases/download/%s/runc.%s",
		version, arch)

	return []*BinaryInfo{
		{
			Name:    "runc",
			Version: version,
			OS:      "linux",
			Arch:    arch,
			URL:     url,
		},
	}
}

// GetCachedPath returns the local cached path for a binary
func (m *BinaryManager) GetCachedPath(binary *BinaryInfo) string {
	return filepath.Join(m.cacheDir, binary.Version, binary.OS, binary.Arch, binary.Name)
}

// normalizeArch normalizes architecture naming for different systems
func (m *BinaryManager) normalizeArch(arch string) string {
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "arm":
		return "arm"
	default:
		return arch
	}
}

// normalizeOS normalizes OS naming
func (m *BinaryManager) normalizeOS(os string) string {
	switch os {
	case "darwin":
		return "darwin"
	case "linux":
		return "linux"
	case "windows":
		return "windows"
	default:
		return os
	}
}

// GetDownloadCommand returns a shell command to download a binary
func (m *BinaryManager) GetDownloadCommand(binary *BinaryInfo) string {
	cachedPath := m.GetCachedPath(binary)
	return fmt.Sprintf("mkdir -p $(dirname %s) && curl -L -o %s %s && chmod +x %s",
		cachedPath, cachedPath, binary.URL, cachedPath)
}

// GetAllRequiredBinaries returns all binaries needed for a Kubernetes installation
func (m *BinaryManager) GetAllRequiredBinaries(kubeVersion, cniVersion, criVersion, containerdVersion, runcVersion string) []*BinaryInfo {
	var binaries []*BinaryInfo

	// Kubernetes core binaries
	binaries = append(binaries, m.GetKubernetesBinaries(kubeVersion)...)

	// CNI plugins (only for Linux)
	if m.os == "linux" {
		binaries = append(binaries, m.GetCNIPluginsBinaries(cniVersion)...)
		binaries = append(binaries, m.GetCRIToolsBinaries(criVersion)...)
		binaries = append(binaries, m.GetContainerdBinaries(containerdVersion)...)
		binaries = append(binaries, m.GetRuncBinaries(runcVersion)...)
	}

	return binaries
}

// SupportsArchitecture checks if binaries are available for the given architecture
func (m *BinaryManager) SupportsArchitecture() bool {
	// Kubernetes officially supports amd64 and arm64
	supportedArchs := []string{"amd64", "arm64"}
	normalizedArch := m.normalizeArch(m.arch)

	for _, supported := range supportedArchs {
		if normalizedArch == supported {
			return true
		}
	}

	return false
}
