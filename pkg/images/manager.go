package images

import (
	"fmt"
	"strings"
)

// ImageManager handles container image selection and management
type ImageManager struct {
	arch     string
	registry string
}

// ImageRef represents a container image reference
type ImageRef struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// NewImageManager creates a new image manager
func NewImageManager(arch string, registry string) *ImageManager {
	if registry == "" {
		registry = "registry.k8s.io"
	}
	return &ImageManager{
		arch:     arch,
		registry: registry,
	}
}

// GetKubernetesImages returns the list of core Kubernetes images for a specific version
func (m *ImageManager) GetKubernetesImages(version string) []ImageRef {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	images := []ImageRef{
		{
			Registry:   m.registry,
			Repository: "kube-apiserver",
			Tag:        version,
		},
		{
			Registry:   m.registry,
			Repository: "kube-controller-manager",
			Tag:        version,
		},
		{
			Registry:   m.registry,
			Repository: "kube-scheduler",
			Tag:        version,
		},
		{
			Registry:   m.registry,
			Repository: "kube-proxy",
			Tag:        version,
		},
		{
			Registry:   m.registry,
			Repository: "pause",
			Tag:        "3.9",
		},
		{
			Registry:   m.registry,
			Repository: "etcd",
			Tag:        "3.5.12-0",
		},
		{
			Registry:   m.registry,
			Repository: "coredns/coredns",
			Tag:        "v1.11.1",
		},
	}

	return images
}

// GetCNIImages returns the list of CNI images (Calico example)
func (m *ImageManager) GetCNIImages(cniType string, cniVersion string) []ImageRef {
	switch cniType {
	case "calico":
		return m.getCalicoImages(cniVersion)
	case "flannel":
		return m.getFlannelImages(cniVersion)
	default:
		return []ImageRef{}
	}
}

// getCalicoImages returns Calico-specific images
func (m *ImageManager) getCalicoImages(version string) []ImageRef {
	version = strings.TrimPrefix(version, "v")

	return []ImageRef{
		{
			Registry:   "quay.io",
			Repository: "tigera/operator",
			Tag:        fmt.Sprintf("v%s", version),
		},
		{
			Registry:   "docker.io",
			Repository: "calico/cni",
			Tag:        fmt.Sprintf("v%s", version),
		},
		{
			Registry:   "docker.io",
			Repository: "calico/node",
			Tag:        fmt.Sprintf("v%s", version),
		},
		{
			Registry:   "docker.io",
			Repository: "calico/kube-controllers",
			Tag:        fmt.Sprintf("v%s", version),
		},
	}
}

// getFlannelImages returns Flannel-specific images
func (m *ImageManager) getFlannelImages(version string) []ImageRef {
	return []ImageRef{
		{
			Registry:   "docker.io",
			Repository: "flannel/flannel",
			Tag:        version,
		},
		{
			Registry:   "docker.io",
			Repository: "flannel/flannel-cni-plugin",
			Tag:        "v1.2.0",
		},
	}
}

// GetImageURL returns the full image URL
func (ref *ImageRef) GetImageURL() string {
	if ref.Digest != "" {
		return fmt.Sprintf("%s/%s@%s", ref.Registry, ref.Repository, ref.Digest)
	}
	return fmt.Sprintf("%s/%s:%s", ref.Registry, ref.Repository, ref.Tag)
}

// String returns a human-readable representation
func (ref *ImageRef) String() string {
	return ref.GetImageURL()
}

// SupportsArchitecture checks if an image supports the given architecture
func (m *ImageManager) SupportsArchitecture(imageRef ImageRef) bool {
	// All official Kubernetes images support both amd64 and arm64
	// This is a placeholder for more sophisticated registry inspection
	officialRegistries := []string{
		"registry.k8s.io",
		"k8s.gcr.io",
		"docker.io",
		"quay.io",
	}

	for _, registry := range officialRegistries {
		if strings.Contains(imageRef.Registry, registry) {
			return true
		}
	}

	return true
}

// GetArchitectureTag returns architecture-specific tag if needed
func (m *ImageManager) GetArchitectureTag(baseTag string) string {
	// Most modern images use multi-arch manifests, so we don't need to append arch
	// This method is kept for compatibility with older images that might need it
	return baseTag
}

// PrePullImages generates a list of images to pre-pull on nodes
func (m *ImageManager) PrePullImages(kubeVersion string, cniType string, cniVersion string) []string {
	var images []string

	// Add Kubernetes core images
	for _, img := range m.GetKubernetesImages(kubeVersion) {
		images = append(images, img.GetImageURL())
	}

	// Add CNI images
	for _, img := range m.GetCNIImages(cniType, cniVersion) {
		images = append(images, img.GetImageURL())
	}

	return images
}
