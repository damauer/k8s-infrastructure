package resources

import (
	"testing"
)

func TestNewResourceCalculator(t *testing.T) {
	calc := NewResourceCalculator("darwin-arm64")
	if calc == nil {
		t.Fatal("NewResourceCalculator returned nil")
	}

	if calc.capabilities.Platform != "darwin-arm64" {
		t.Errorf("Expected platform 'darwin-arm64', got '%s'", calc.capabilities.Platform)
	}
}

func TestGetPlatformCapabilities(t *testing.T) {
	tests := []struct {
		name              string
		platform          string
		expectedTotalCPU  int
		expectedMaxNodes  int
	}{
		{"macOS M2", "darwin-arm64", 8, 5},
		{"macOS Intel", "darwin-amd64", 4, 4},
		{"Raspberry Pi", "linux-arm64-pi", 4, 1},
		{"WSL2", "linux-amd64-wsl2", 4, 4},
		{"Generic", "linux-amd64", 4, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap := getPlatformCapabilities(tt.platform)

			if cap.TotalCPU != tt.expectedTotalCPU {
				t.Errorf("Expected TotalCPU %d, got %d", tt.expectedTotalCPU, cap.TotalCPU)
			}

			if cap.MaxNodesLimit != tt.expectedMaxNodes {
				t.Errorf("Expected MaxNodesLimit %d, got %d", tt.expectedMaxNodes, cap.MaxNodesLimit)
			}

			if cap.ReservedCPU <= 0 {
				t.Error("ReservedCPU should be greater than 0")
			}

			if cap.ReservedMemGB <= 0 {
				t.Error("ReservedMemGB should be greater than 0")
			}
		})
	}
}

func TestCalculateRecommendation(t *testing.T) {
	tests := []struct {
		name             string
		platform         string
		profile          WorkloadProfile
		requestedWorkers int
		minWorkers       int
		maxWorkers       int
	}{
		{"macOS Development", "darwin-arm64", ProfileDevelopment, 2, 1, 4},
		{"macOS Production", "darwin-arm64", ProfileProduction, 3, 2, 4},
		{"Pi Development", "linux-arm64-pi", ProfileDevelopment, 0, 0, 0},
		{"WSL2 Testing", "linux-amd64-wsl2", ProfileTesting, 2, 1, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calc := NewResourceCalculator(tt.platform)
			rec, err := calc.CalculateRecommendation(tt.profile, tt.requestedWorkers)

			if err != nil {
				t.Fatalf("CalculateRecommendation failed: %v", err)
			}

			if rec == nil {
				t.Fatal("Recommendation is nil")
			}

			// Validate control plane resources
			if rec.ControlPlaneCPU == "" {
				t.Error("ControlPlaneCPU is empty")
			}
			if rec.ControlPlaneMemory == "" {
				t.Error("ControlPlaneMemory is empty")
			}
			if rec.ControlPlaneDisk == "" {
				t.Error("ControlPlaneDisk is empty")
			}

			// Validate worker resources
			if rec.WorkerCPU == "" {
				t.Error("WorkerCPU is empty")
			}
			if rec.WorkerMemory == "" {
				t.Error("WorkerMemory is empty")
			}
			if rec.WorkerDisk == "" {
				t.Error("WorkerDisk is empty")
			}

			// Validate worker counts
			if tt.requestedWorkers > 0 {
				if rec.RecommendedWorkers < tt.minWorkers {
					t.Errorf("RecommendedWorkers %d is less than min %d", rec.RecommendedWorkers, tt.minWorkers)
				}
				if rec.RecommendedWorkers > tt.maxWorkers {
					t.Errorf("RecommendedWorkers %d exceeds max %d", rec.RecommendedWorkers, tt.maxWorkers)
				}
			}

			t.Logf("Recommendation: CP=%s/%s, Worker=%s/%s, Workers=%d/%d",
				rec.ControlPlaneCPU, rec.ControlPlaneMemory,
				rec.WorkerCPU, rec.WorkerMemory,
				rec.RecommendedWorkers, rec.MaxWorkers)
		})
	}
}

func TestCalculateRecommendationProfiles(t *testing.T) {
	calc := NewResourceCalculator("darwin-arm64")

	profiles := []WorkloadProfile{
		ProfileDevelopment,
		ProfileTesting,
		ProfileProduction,
	}

	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			rec, err := calc.CalculateRecommendation(profile, 2)
			if err != nil {
				t.Fatalf("Failed to calculate recommendation for %s: %v", profile, err)
			}

			// Production should have larger disks
			if profile == ProfileProduction {
				if rec.ControlPlaneDisk != "50G" {
					t.Errorf("Expected 50G disk for production, got %s", rec.ControlPlaneDisk)
				}
			}

			// Development should have smaller disks
			if profile == ProfileDevelopment {
				if rec.ControlPlaneDisk != "20G" {
					t.Errorf("Expected 20G disk for development, got %s", rec.ControlPlaneDisk)
				}
			}
		})
	}
}

func TestFormatCPU(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{2.5, "2"},
		{1.0, "1"},
		{0.5, "500m"},
		{0.25, "200m"}, // Rounds to 1 decimal: 0.25 -> 0.2
		{4.0, "4"},
		{0.1, "100m"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatCPU(tt.input)
			if result != tt.expected {
				t.Errorf("formatCPU(%.2f) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{2.5, "2G"},     // Rounds to nearest: 2560MB -> 2.5GB -> 2G (banker's rounding)
		{1.0, "1G"},
		{0.5, "512M"},
		{0.75, "768M"},
		{4.0, "4G"},
		{1.5, "2G"},     // Rounds to nearest: 1536MB -> 1.5GB -> 2G
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatMemory(tt.input)
			if result != tt.expected {
				t.Errorf("formatMemory(%.2f) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseCPU(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"2", 2.0, false},
		{"500m", 0.5, false},
		{"1000m", 1.0, false},
		{"250m", 0.25, false},
		{"4", 4.0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseCPU(tt.input)

			if tt.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("ParseCPU(%s) = %.2f, want %.2f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
		hasError bool
	}{
		{"2G", 2.0, false},
		{"2g", 2.0, false},
		{"1024M", 1.0, false},
		{"512M", 0.5, false},
		{"4G", 4.0, false},
		{"2048m", 2.0, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseMemory(tt.input)

			if tt.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("ParseMemory(%s) = %.2f, want %.2f", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateResources(t *testing.T) {
	calc := NewResourceCalculator("darwin-arm64")

	tests := []struct {
		name      string
		cpu       string
		memory    string
		nodeCount int
		shouldErr bool
	}{
		{"Valid small", "1", "2G", 3, false},
		{"Valid medium", "2", "4G", 2, false},
		{"Exceeds CPU", "4", "2G", 3, true},
		{"Exceeds memory", "1", "8G", 2, true},
		{"Exceeds node limit", "1", "2G", 10, true},
		{"Invalid CPU format", "invalid", "2G", 2, true},
		{"Invalid memory format", "2", "invalid", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := calc.ValidateResources(tt.cpu, tt.memory, tt.nodeCount)

			if tt.shouldErr && err == nil {
				t.Error("Expected error but got none")
			}

			if !tt.shouldErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestGetPlatformCapabilities_Method(t *testing.T) {
	calc := NewResourceCalculator("darwin-arm64")
	cap := calc.GetPlatformCapabilities()

	if cap.Platform != "darwin-arm64" {
		t.Errorf("Expected platform 'darwin-arm64', got '%s'", cap.Platform)
	}

	if cap.TotalCPU <= 0 {
		t.Error("TotalCPU should be greater than 0")
	}

	if cap.TotalMemoryGB <= 0 {
		t.Error("TotalMemoryGB should be greater than 0")
	}
}

func TestRaspberryPiConstraints(t *testing.T) {
	calc := NewResourceCalculator("linux-arm64-pi")
	rec, err := calc.CalculateRecommendation(ProfileDevelopment, 1)

	if err != nil {
		t.Fatalf("Failed to calculate recommendation: %v", err)
	}

	// Raspberry Pi should have max 0 workers (single node)
	if rec.MaxWorkers != 0 {
		t.Errorf("Expected MaxWorkers 0 for Pi, got %d", rec.MaxWorkers)
	}
}

func TestAutoCalculateWorkers(t *testing.T) {
	calc := NewResourceCalculator("darwin-arm64")

	// Request 0 workers to trigger auto-calculation
	rec, err := calc.CalculateRecommendation(ProfileDevelopment, 0)

	if err != nil {
		t.Fatalf("Failed to calculate recommendation: %v", err)
	}

	if rec.RecommendedWorkers == 0 {
		t.Error("Auto-calculated workers should be greater than 0")
	}

	if rec.RecommendedWorkers > rec.MaxWorkers {
		t.Errorf("Auto-calculated workers %d exceeds max %d", rec.RecommendedWorkers, rec.MaxWorkers)
	}

	t.Logf("Auto-calculated %d workers (max: %d)", rec.RecommendedWorkers, rec.MaxWorkers)
}
