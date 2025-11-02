package resources

import (
	"fmt"
	"strconv"
	"strings"
)

// WorkloadProfile represents different workload types
type WorkloadProfile string

const (
	ProfileDevelopment WorkloadProfile = "development"
	ProfileTesting     WorkloadProfile = "testing"
	ProfileProduction  WorkloadProfile = "production"
	ProfileCustom      WorkloadProfile = "custom"
)

// PlatformCapabilities represents the resource capabilities of a platform
type PlatformCapabilities struct {
	Platform       string
	TotalCPU       int     // Total CPU cores available
	TotalMemoryGB  float64 // Total memory in GB
	ReservedCPU    float64 // CPU reserved for system
	ReservedMemGB  float64 // Memory reserved for system in GB
	MaxNodesLimit  int     // Maximum number of nodes recommended
}

// ResourceRecommendation represents recommended resources for a cluster
type ResourceRecommendation struct {
	ControlPlaneCPU    string
	ControlPlaneMemory string
	ControlPlaneDisk   string
	WorkerCPU          string
	WorkerMemory       string
	WorkerDisk         string
	MaxWorkers         int
	RecommendedWorkers int
}

// ResourceCalculator calculates optimal resource allocations
type ResourceCalculator struct {
	capabilities PlatformCapabilities
}

// NewResourceCalculator creates a new resource calculator
func NewResourceCalculator(platform string) *ResourceCalculator {
	return &ResourceCalculator{
		capabilities: getPlatformCapabilities(platform),
	}
}

// getPlatformCapabilities returns the capabilities for a given platform
func getPlatformCapabilities(platform string) PlatformCapabilities {
	switch {
	case strings.Contains(platform, "darwin-arm64"):
		// macOS M2 MacBook Pro - example with 16GB RAM, 8 cores
		return PlatformCapabilities{
			Platform:       platform,
			TotalCPU:       8,
			TotalMemoryGB:  16,
			ReservedCPU:    2,   // Reserve 2 cores for macOS
			ReservedMemGB:  4,   // Reserve 4GB for macOS
			MaxNodesLimit:  5,   // VM overhead considerations
		}
	case strings.Contains(platform, "darwin-amd64"):
		// macOS Intel - example with 16GB RAM, 4 cores
		return PlatformCapabilities{
			Platform:       platform,
			TotalCPU:       4,
			TotalMemoryGB:  16,
			ReservedCPU:    1,
			ReservedMemGB:  4,
			MaxNodesLimit:  4,
		}
	case strings.Contains(platform, "linux-arm64-pi"):
		// Raspberry Pi 5 - 8GB model
		return PlatformCapabilities{
			Platform:       platform,
			TotalCPU:       4,
			TotalMemoryGB:  8,
			ReservedCPU:    0.5, // Minimal OS overhead
			ReservedMemGB:  1,   // Reserve 1GB for OS
			MaxNodesLimit:  1,   // Single node for Pi
		}
	case strings.Contains(platform, "wsl2"):
		// WSL2 - conservative defaults
		return PlatformCapabilities{
			Platform:       platform,
			TotalCPU:       4,
			TotalMemoryGB:  8,
			ReservedCPU:    1,
			ReservedMemGB:  2,
			MaxNodesLimit:  4,
		}
	default:
		// Generic Linux
		return PlatformCapabilities{
			Platform:       platform,
			TotalCPU:       4,
			TotalMemoryGB:  8,
			ReservedCPU:    1,
			ReservedMemGB:  2,
			MaxNodesLimit:  5,
		}
	}
}

// CalculateRecommendation calculates resource recommendations based on profile
func (rc *ResourceCalculator) CalculateRecommendation(profile WorkloadProfile, requestedWorkers int) (*ResourceRecommendation, error) {
	cap := rc.capabilities

	// Calculate available resources
	availableCPU := float64(cap.TotalCPU) - cap.ReservedCPU
	availableMemGB := cap.TotalMemoryGB - cap.ReservedMemGB

	// Determine resource allocation based on profile
	var cpRatio float64
	var cpDisk, workerDisk string

	switch profile {
	case ProfileDevelopment:
		cpRatio = 0.3 // 30% of available for control plane, 70% for workers
		cpDisk = "20G"
		workerDisk = "20G"
	case ProfileTesting:
		cpRatio = 0.25 // 25% for control plane, 75% for workers
		cpDisk = "30G"
		workerDisk = "30G"
	case ProfileProduction:
		cpRatio = 0.2 // 20% for control plane, 80% for workers (more resources for workloads)
		cpDisk = "50G"
		workerDisk = "50G"
	default:
		cpRatio = 0.3
		cpDisk = "20G"
		workerDisk = "20G"
	}

	// Calculate control plane resources
	cpCPU := availableCPU * cpRatio
	cpMemGB := availableMemGB * cpRatio

	// Ensure minimum control plane requirements
	if cpCPU < 2 {
		cpCPU = 2
	}
	if cpMemGB < 2 {
		cpMemGB = 2
	}

	// Calculate per-worker resources
	remainingCPU := availableCPU - cpCPU
	remainingMemGB := availableMemGB - cpMemGB

	// Determine maximum workers based on platform and resources
	maxWorkers := cap.MaxNodesLimit - 1 // Subtract control plane

	// Calculate how many workers we can support with available resources
	if requestedWorkers > 0 {
		workerCPU := remainingCPU / float64(requestedWorkers)
		workerMemGB := remainingMemGB / float64(requestedWorkers)

		// Ensure minimum worker requirements
		if workerCPU < 1 {
			// Not enough CPU, reduce workers
			requestedWorkers = int(remainingCPU)
			if requestedWorkers < 1 {
				requestedWorkers = 1
			}
			workerCPU = remainingCPU / float64(requestedWorkers)
		}
		if workerMemGB < 1 {
			// Not enough memory, reduce workers
			requestedWorkers = int(remainingMemGB)
			if requestedWorkers < 1 {
				requestedWorkers = 1
			}
			workerMemGB = remainingMemGB / float64(requestedWorkers)
		}

		if requestedWorkers > maxWorkers {
			requestedWorkers = maxWorkers
			workerCPU = remainingCPU / float64(requestedWorkers)
			workerMemGB = remainingMemGB / float64(requestedWorkers)
		}

		recommendation := &ResourceRecommendation{
			ControlPlaneCPU:    formatCPU(cpCPU),
			ControlPlaneMemory: formatMemory(cpMemGB),
			ControlPlaneDisk:   cpDisk,
			WorkerCPU:          formatCPU(workerCPU),
			WorkerMemory:       formatMemory(workerMemGB),
			WorkerDisk:         workerDisk,
			MaxWorkers:         maxWorkers,
			RecommendedWorkers: requestedWorkers,
		}

		return recommendation, nil
	}

	// Auto-calculate optimal worker count
	optimalWorkers := 2 // Default
	if maxWorkers < 2 {
		optimalWorkers = maxWorkers
	} else if maxWorkers > 3 {
		optimalWorkers = 3 // Sweet spot for most dev environments
	}

	workerCPU := remainingCPU / float64(optimalWorkers)
	workerMemGB := remainingMemGB / float64(optimalWorkers)

	recommendation := &ResourceRecommendation{
		ControlPlaneCPU:    formatCPU(cpCPU),
		ControlPlaneMemory: formatMemory(cpMemGB),
		ControlPlaneDisk:   cpDisk,
		WorkerCPU:          formatCPU(workerCPU),
		WorkerMemory:       formatMemory(workerMemGB),
		WorkerDisk:         workerDisk,
		MaxWorkers:         maxWorkers,
		RecommendedWorkers: optimalWorkers,
	}

	return recommendation, nil
}

// formatCPU formats CPU value for Kubernetes
func formatCPU(cpu float64) string {
	// Round to 1 decimal place
	rounded := float64(int(cpu*10)) / 10
	if rounded >= 1 {
		return fmt.Sprintf("%.0f", rounded)
	}
	// Use millicores for values < 1
	return fmt.Sprintf("%dm", int(rounded*1000))
}

// formatMemory formats memory value for Kubernetes
func formatMemory(memGB float64) string {
	// Round to nearest 256MB increment
	memMB := int(memGB * 1024)
	memMB = ((memMB + 127) / 256) * 256

	if memMB >= 1024 {
		gb := float64(memMB) / 1024
		return fmt.Sprintf("%.0fG", gb)
	}
	return fmt.Sprintf("%dM", memMB)
}

// ParseCPU parses a CPU string (e.g., "2", "500m") to float64
func ParseCPU(cpu string) (float64, error) {
	if strings.HasSuffix(cpu, "m") {
		// Millicores
		val, err := strconv.ParseFloat(strings.TrimSuffix(cpu, "m"), 64)
		if err != nil {
			return 0, err
		}
		return val / 1000, nil
	}
	return strconv.ParseFloat(cpu, 64)
}

// ParseMemory parses a memory string (e.g., "2G", "512M") to GB
func ParseMemory(mem string) (float64, error) {
	mem = strings.TrimSpace(mem)
	if strings.HasSuffix(mem, "G") || strings.HasSuffix(mem, "g") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(mem, "G"), "g"), 64)
		return val, err
	}
	if strings.HasSuffix(mem, "M") || strings.HasSuffix(mem, "m") {
		val, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(mem, "M"), "m"), 64)
		if err != nil {
			return 0, err
		}
		return val / 1024, nil
	}
	// Assume GB if no suffix
	return strconv.ParseFloat(mem, 64)
}

// ValidateResources checks if requested resources are within platform limits
func (rc *ResourceCalculator) ValidateResources(cpuStr, memStr string, nodeCount int) error {
	cpu, err := ParseCPU(cpuStr)
	if err != nil {
		return fmt.Errorf("invalid CPU format: %w", err)
	}

	memGB, err := ParseMemory(memStr)
	if err != nil {
		return fmt.Errorf("invalid memory format: %w", err)
	}

	totalCPU := cpu * float64(nodeCount)
	totalMemGB := memGB * float64(nodeCount)

	availableCPU := float64(rc.capabilities.TotalCPU) - rc.capabilities.ReservedCPU
	availableMemGB := rc.capabilities.TotalMemoryGB - rc.capabilities.ReservedMemGB

	if totalCPU > availableCPU {
		return fmt.Errorf("requested CPU (%.1f) exceeds available CPU (%.1f)", totalCPU, availableCPU)
	}

	if totalMemGB > availableMemGB {
		return fmt.Errorf("requested memory (%.1fG) exceeds available memory (%.1fG)", totalMemGB, availableMemGB)
	}

	if nodeCount > rc.capabilities.MaxNodesLimit {
		return fmt.Errorf("requested nodes (%d) exceeds platform limit (%d)", nodeCount, rc.capabilities.MaxNodesLimit)
	}

	return nil
}

// GetPlatformCapabilities returns the platform capabilities
func (rc *ResourceCalculator) GetPlatformCapabilities() PlatformCapabilities {
	return rc.capabilities
}
