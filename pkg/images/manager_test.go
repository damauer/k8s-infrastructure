package images

import (
	"strings"
	"testing"
)

func TestNewImageManager(t *testing.T) {
	manager := NewImageManager("arm64", "")
	if manager == nil {
		t.Fatal("NewImageManager() returned nil")
	}

	if manager.arch != "arm64" {
		t.Errorf("Expected arch 'arm64', got '%s'", manager.arch)
	}

	if manager.registry != "registry.k8s.io" {
		t.Errorf("Expected default registry 'registry.k8s.io', got '%s'", manager.registry)
	}
}

func TestNewImageManagerCustomRegistry(t *testing.T) {
	manager := NewImageManager("amd64", "my-registry.com")
	if manager.registry != "my-registry.com" {
		t.Errorf("Expected custom registry 'my-registry.com', got '%s'", manager.registry)
	}
}

func TestGetKubernetesImages(t *testing.T) {
	manager := NewImageManager("arm64", "")
	images := manager.GetKubernetesImages("1.30.0")

	if len(images) == 0 {
		t.Fatal("GetKubernetesImages() returned empty list")
	}

	// Check for essential components
	essentialComponents := []string{
		"kube-apiserver",
		"kube-controller-manager",
		"kube-scheduler",
		"kube-proxy",
		"etcd",
		"coredns",
	}

	for _, component := range essentialComponents {
		found := false
		for _, img := range images {
			if strings.Contains(img.Repository, component) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Essential component '%s' not found in image list", component)
		}
	}
}

func TestGetKubernetesImagesVersionHandling(t *testing.T) {
	manager := NewImageManager("arm64", "")

	// Test with 'v' prefix
	imagesWithV := manager.GetKubernetesImages("v1.30.0")
	// Test without 'v' prefix
	imagesWithoutV := manager.GetKubernetesImages("1.30.0")

	if len(imagesWithV) != len(imagesWithoutV) {
		t.Error("Version with and without 'v' prefix should return same number of images")
	}
}

func TestGetCNIImages(t *testing.T) {
	manager := NewImageManager("arm64", "")

	tests := []struct {
		name       string
		cniType    string
		cniVersion string
		minImages  int
	}{
		{"Calico", "calico", "3.28.0", 3},
		{"Flannel", "flannel", "v0.24.0", 1},
		{"Unknown", "unknown", "1.0.0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			images := manager.GetCNIImages(tt.cniType, tt.cniVersion)
			if len(images) < tt.minImages {
				t.Errorf("Expected at least %d images for %s, got %d", tt.minImages, tt.cniType, len(images))
			}
		})
	}
}

func TestImageRefGetImageURL(t *testing.T) {
	tests := []struct {
		name     string
		imageRef ImageRef
		expected string
	}{
		{
			name: "Standard image",
			imageRef: ImageRef{
				Registry:   "registry.k8s.io",
				Repository: "kube-apiserver",
				Tag:        "v1.30.0",
			},
			expected: "registry.k8s.io/kube-apiserver:v1.30.0",
		},
		{
			name: "Image with digest",
			imageRef: ImageRef{
				Registry:   "registry.k8s.io",
				Repository: "kube-proxy",
				Tag:        "v1.30.0",
				Digest:     "sha256:abc123",
			},
			expected: "registry.k8s.io/kube-proxy@sha256:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := tt.imageRef.GetImageURL()
			if url != tt.expected {
				t.Errorf("Expected URL '%s', got '%s'", tt.expected, url)
			}
		})
	}
}

func TestSupportsArchitecture(t *testing.T) {
	manager := NewImageManager("arm64", "")

	tests := []struct {
		name     string
		imageRef ImageRef
		expected bool
	}{
		{
			name: "Official k8s.io registry",
			imageRef: ImageRef{
				Registry:   "registry.k8s.io",
				Repository: "kube-apiserver",
			},
			expected: true,
		},
		{
			name: "Docker Hub",
			imageRef: ImageRef{
				Registry:   "docker.io",
				Repository: "calico/node",
			},
			expected: true,
		},
		{
			name: "Quay.io",
			imageRef: ImageRef{
				Registry:   "quay.io",
				Repository: "tigera/operator",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.SupportsArchitecture(tt.imageRef)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPrePullImages(t *testing.T) {
	manager := NewImageManager("arm64", "")
	images := manager.PrePullImages("v1.30.0", "calico", "3.28.0")

	if len(images) == 0 {
		t.Fatal("PrePullImages() returned empty list")
	}

	// Should contain at least Kubernetes core images and CNI images
	if len(images) < 5 {
		t.Errorf("Expected at least 5 images, got %d", len(images))
	}

	// All images should be valid URLs
	for _, img := range images {
		if !strings.Contains(img, ":") && !strings.Contains(img, "@") {
			t.Errorf("Invalid image URL format: %s", img)
		}
	}
}

func TestGetArchitectureTag(t *testing.T) {
	manager := NewImageManager("arm64", "")
	tag := manager.GetArchitectureTag("v1.30.0")

	// Modern images use multi-arch manifests, so tag should remain unchanged
	if tag != "v1.30.0" {
		t.Errorf("Expected tag 'v1.30.0', got '%s'", tag)
	}
}
