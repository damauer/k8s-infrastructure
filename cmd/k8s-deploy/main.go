package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s-infrastructure/pkg/addons"
	"k8s-infrastructure/pkg/binaries"
	"k8s-infrastructure/pkg/cloudinit"
	"k8s-infrastructure/pkg/config"
	"k8s-infrastructure/pkg/images"
	"k8s-infrastructure/pkg/network"
	"k8s-infrastructure/pkg/platform"
	"k8s-infrastructure/pkg/provider"
	"k8s-infrastructure/pkg/provider/multipass"
	"k8s-infrastructure/pkg/provider/native"
	"k8s-infrastructure/pkg/resources"
)

const (
	version = "0.1.0"
)

type CLI struct {
	config   *config.Config
	platform *platform.Platform
	provider provider.Provider
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "create":
		handleCreate()
	case "destroy":
		handleDestroy()
	case "list":
		handleList()
	case "status":
		handleStatus()
	case "version":
		fmt.Printf("k8s-deploy version %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleCreate() {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	env := fs.String("env", "dev", "Environment to deploy (dev, prd)")
	platformOverride := fs.String("platform", "auto", "Platform override (auto, darwin-arm64, linux-arm64-pi, etc.)")
	k8sVersion := fs.String("k8s-version", "1.30.0", "Kubernetes version")
	cniType := fs.String("cni", "flannel", "CNI type (calico, flannel)")
	cniVersion := fs.String("cni-version", "latest", "CNI version")
	nodes := fs.Int("nodes", 0, "Number of worker nodes (0 for auto-calculation)")
	cpus := fs.String("cpus", "", "CPUs per node (empty for auto-calculation)")
	memory := fs.String("memory", "", "Memory per node (empty for auto-calculation)")
	disk := fs.String("disk", "", "Disk per node (empty for auto-calculation)")
	profile := fs.String("profile", "development", "Workload profile (development, testing, production)")
	autoSize := fs.Bool("auto-size", false, "Automatically calculate optimal resource sizes")

	fs.Parse(os.Args[2:])

	cli, err := initializeCLI(*platformOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	// Initialize resource calculator
	calc := resources.NewResourceCalculator(cli.platform.String())
	caps := calc.GetPlatformCapabilities()

	// Parse workload profile
	var workloadProfile resources.WorkloadProfile
	switch *profile {
	case "development":
		workloadProfile = resources.ProfileDevelopment
	case "testing":
		workloadProfile = resources.ProfileTesting
	case "production":
		workloadProfile = resources.ProfileProduction
	default:
		fmt.Fprintf(os.Stderr, "Invalid profile: %s. Must be development, testing, or production\n", *profile)
		os.Exit(1)
	}

	// Get resource recommendations
	rec, err := calc.CalculateRecommendation(workloadProfile, *nodes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to calculate resource recommendations: %v\n", err)
		os.Exit(1)
	}

	// Determine final resource values (use recommendations if auto-size or not specified)
	finalCPUs := *cpus
	finalMemory := *memory
	finalDisk := *disk
	finalNodes := *nodes

	if *autoSize || *cpus == "" || *memory == "" || *disk == "" || *nodes == 0 {
		// Use recommendations
		if finalCPUs == "" {
			finalCPUs = rec.WorkerCPU
		}
		if finalMemory == "" {
			finalMemory = rec.WorkerMemory
		}
		if finalDisk == "" {
			finalDisk = rec.WorkerDisk
		}
		if finalNodes == 0 {
			finalNodes = rec.RecommendedWorkers
		}
	}

	// Multipass requires integer CPU values, convert millicores to whole CPUs
	if cli.platform.DeployMethod == "multipass" {
		// Parse and round up fractional CPUs for control plane
		cpCPU, err := resources.ParseCPU(rec.ControlPlaneCPU)
		if err == nil && cpCPU < 1 {
			rec.ControlPlaneCPU = "1"
		} else if err == nil && cpCPU != float64(int(cpCPU)) {
			rec.ControlPlaneCPU = fmt.Sprintf("%d", int(cpCPU)+1)
		}

		// Parse and round up fractional CPUs for workers
		workerCPU, err := resources.ParseCPU(finalCPUs)
		if err == nil && workerCPU < 1 {
			finalCPUs = "1"
		} else if err == nil && workerCPU != float64(int(workerCPU)) {
			finalCPUs = fmt.Sprintf("%d", int(workerCPU)+1)
		}
	}

	// Validate resources against platform limits
	totalNodes := finalNodes + 1 // workers + control plane
	if err := calc.ValidateResources(finalCPUs, finalMemory, totalNodes); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Resource validation failed: %v\n", err)
		fmt.Printf("\n💡 Platform Capabilities (%s):\n", caps.Platform)
		fmt.Printf("   Total CPU: %d cores (%.1f available after OS reservation)\n",
			caps.TotalCPU, float64(caps.TotalCPU)-caps.ReservedCPU)
		fmt.Printf("   Total Memory: %.1fGB (%.1fGB available after OS reservation)\n",
			caps.TotalMemoryGB, caps.TotalMemoryGB-caps.ReservedMemGB)
		fmt.Printf("   Max Nodes: %d\n", caps.MaxNodesLimit)
		os.Exit(1)
	}

	fmt.Printf("🚀 Creating Kubernetes cluster\n")
	fmt.Printf("   Environment: %s\n", *env)
	fmt.Printf("   Platform: %s\n", cli.platform.String())
	fmt.Printf("   Profile: %s\n", *profile)
	fmt.Printf("   Kubernetes: v%s\n", *k8sVersion)
	fmt.Printf("   CNI: %s v%s\n", *cniType, *cniVersion)
	fmt.Printf("   Nodes: 1 control-plane + %d workers\n\n", finalNodes)

	fmt.Printf("📊 Resource Allocation:\n")
	fmt.Printf("   Control Plane: %s CPU, %s Memory, %s Disk\n",
		rec.ControlPlaneCPU, rec.ControlPlaneMemory, rec.ControlPlaneDisk)
	fmt.Printf("   Workers: %s CPU, %s Memory, %s Disk (each)\n",
		finalCPUs, finalMemory, finalDisk)
	if *autoSize || *cpus == "" {
		fmt.Printf("   ℹ️  Using optimized resources for %s workload\n", *profile)
	}
	fmt.Println()

	ctx := context.Background()

	// Setup network configuration first
	netManager := network.NewNetworkManager(cli.platform.String(), cli.platform.DeployMethod)
	netConfig := netManager.GetDefaultNetworkConfig(*env)

	if err := netManager.ValidateNetworkConfig(netConfig); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid network configuration: %v\n", err)
		os.Exit(1)
	}

	// Create cluster configuration
	clusterConfig := provider.ClusterConfig{
		Name:        fmt.Sprintf("k8sv1-%s", *env),
		Environment: *env,
		Nodes:       []provider.NodeConfig{},
		PodCIDR:     netConfig.PodCIDR,
		ServiceCIDR: netConfig.ServiceCIDR,
		CNI:         *cniType,
		CNIVersion:  *cniVersion,
	}

	// Generate cloud-init configs
	k8sMajorMinor := strings.Join(strings.Split(*k8sVersion, ".")[:2], ".")
	ciConfig := cloudinit.Config{
		K8sVersion:  k8sMajorMinor,
		PodCIDR:     netConfig.PodCIDR,
		ServiceCIDR: netConfig.ServiceCIDR,
	}

	// Generate control plane cloud-init
	ciConfig.Role = "control-plane"
	controlPlaneInit, err := cloudinit.GenerateControlPlaneConfig(ciConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to generate control plane cloud-init: %v\n", err)
		os.Exit(1)
	}

	// Write control plane cloud-init to temp file
	controlPlaneTmpFile, err := os.CreateTemp("", "k8s-cp-init-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(controlPlaneTmpFile.Name())

	if _, err := controlPlaneTmpFile.WriteString(controlPlaneInit); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to write cloud-init: %v\n", err)
		os.Exit(1)
	}
	controlPlaneTmpFile.Close()

	// Generate worker cloud-init
	ciConfig.Role = "worker"
	workerInit, err := cloudinit.GenerateWorkerConfig(ciConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to generate worker cloud-init: %v\n", err)
		os.Exit(1)
	}

	// Write worker cloud-init to temp file
	workerTmpFile, err := os.CreateTemp("", "k8s-worker-init-*.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(workerTmpFile.Name())

	if _, err := workerTmpFile.WriteString(workerInit); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to write cloud-init: %v\n", err)
		os.Exit(1)
	}
	workerTmpFile.Close()

	// Add control plane node
	clusterConfig.Nodes = append(clusterConfig.Nodes, provider.NodeConfig{
		Name:          fmt.Sprintf("k8sv1-%s-c1", *env),
		Role:          "control-plane",
		CPUs:          rec.ControlPlaneCPU,
		Memory:        rec.ControlPlaneMemory,
		Disk:          rec.ControlPlaneDisk,
		Environment:   *env,
		KubeVersion:   fmt.Sprintf("v%s", *k8sVersion),
		CloudInitPath: controlPlaneTmpFile.Name(),
	})

	// Add worker nodes
	for i := 0; i < finalNodes; i++ {
		clusterConfig.Nodes = append(clusterConfig.Nodes, provider.NodeConfig{
			Name:          fmt.Sprintf("k8sv1-%s-w%d", *env, i+1),
			Role:          "worker",
			CPUs:          finalCPUs,
			Memory:        finalMemory,
			Disk:          finalDisk,
			Environment:   *env,
			KubeVersion:   fmt.Sprintf("v%s", *k8sVersion),
			CloudInitPath: workerTmpFile.Name(),
		})
	}

	fmt.Printf("📊 Network Configuration:\n")
	fmt.Printf("   Pod CIDR: %s\n", netConfig.PodCIDR)
	fmt.Printf("   Service CIDR: %s\n", netConfig.ServiceCIDR)
	fmt.Printf("   DNS Service IP: %s\n", netConfig.DNSServiceIP)
	fmt.Printf("   MTU: %d\n\n", netConfig.MTU)

	// Display image and binary information
	imgManager := images.NewImageManager(cli.platform.Arch, "")
	binManager := binaries.NewBinaryManager(cli.platform.OS, cli.platform.Arch, "")

	k8sImages := imgManager.GetKubernetesImages(*k8sVersion)
	k8sBinaries := binManager.GetKubernetesBinaries(fmt.Sprintf("v%s", *k8sVersion))

	fmt.Printf("📦 Required Images: %d\n", len(k8sImages))
	fmt.Printf("📦 Required Binaries: %d\n\n", len(k8sBinaries))

	// Deploy cluster
	fmt.Printf("⏳ Deploying cluster...\n")
	if err := cli.provider.DeployCluster(ctx, clusterConfig); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to deploy cluster: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Cluster created successfully!\n\n")

	// Install add-ons
	controlPlaneName := fmt.Sprintf("k8sv1-%s-c1", *env)
	addonMgr := addons.NewAddonManager(cli.provider.Name(), controlPlaneName)

	if err := addonMgr.InstallAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to install add-ons: %v\n", err)
		os.Exit(1)
	}

	// Get control plane IP for displaying URLs
	controlPlaneStatus, err := cli.provider.GetNodeStatus(ctx, controlPlaneName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not get control plane IP: %v\n", err)
	} else {
		// Get access information
		accessInfo, err := addonMgr.GetAccessInfo(ctx, controlPlaneStatus.IP)
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  Warning: Could not get access information: %v\n", err)
		} else {
			// Display access information
			fmt.Printf("\n📊 Monitoring & GitOps:\n")
			fmt.Printf("  • Grafana:     %s\n", accessInfo["grafana_url"])
			fmt.Printf("    Username:    %s\n", accessInfo["grafana_username"])
			fmt.Printf("    Password:    %s\n", accessInfo["grafana_password"])
			fmt.Printf("\n  • Prometheus:  %s\n", accessInfo["prometheus_url"])
			fmt.Printf("\n  • ArgoCD:      %s (HTTPS: %s)\n", accessInfo["argocd_url_http"], accessInfo["argocd_url_https"])
			fmt.Printf("    Username:    %s\n", accessInfo["argocd_username"])
			fmt.Printf("    Password:    %s\n", accessInfo["argocd_password"])
		}
	}

	fmt.Printf("\n✅ All components installed successfully!\n")
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  • View cluster status: k8s-deploy status --env %s\n", *env)
	fmt.Printf("  • List all nodes: k8s-deploy list\n")
}

func handleDestroy() {
	fs := flag.NewFlagSet("destroy", flag.ExitOnError)
	env := fs.String("env", "dev", "Environment to destroy (dev, prd)")
	platformOverride := fs.String("platform", "auto", "Platform override")
	confirm := fs.Bool("yes", false, "Skip confirmation prompt")

	fs.Parse(os.Args[2:])

	if !*confirm {
		fmt.Printf("⚠️  This will destroy the %s cluster and all its resources.\n", *env)
		fmt.Print("Are you sure? (yes/no): ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	cli, err := initializeCLI(*platformOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	clusterName := fmt.Sprintf("k8sv1-%s", *env)

	fmt.Printf("🗑️  Destroying cluster: %s\n", clusterName)

	if err := cli.provider.DestroyCluster(ctx, clusterName); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to destroy cluster: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Cluster destroyed successfully!\n")
}

func handleList() {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	platformOverride := fs.String("platform", "auto", "Platform override")

	fs.Parse(os.Args[2:])

	cli, err := initializeCLI(*platformOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	nodes, err := cli.provider.ListNodes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to list nodes: %v\n", err)
		os.Exit(1)
	}

	if len(nodes) == 0 {
		fmt.Println("No nodes found.")
		return
	}

	fmt.Printf("📋 Nodes (%d):\n\n", len(nodes))
	fmt.Printf("%-30s %-15s %-12s %-15s\n", "NAME", "STATE", "ROLE", "IP")
	fmt.Println("─────────────────────────────────────────────────────────────────────────")

	for _, node := range nodes {
		role := node.Role
		if role == "" {
			role = "unknown"
		}
		ip := node.IP
		if ip == "" {
			ip = "N/A"
		}
		fmt.Printf("%-30s %-15s %-12s %-15s\n", node.Name, node.State, role, ip)
	}
}

func handleStatus() {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	env := fs.String("env", "dev", "Environment to check")
	platformOverride := fs.String("platform", "auto", "Platform override")

	fs.Parse(os.Args[2:])

	cli, err := initializeCLI(*platformOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	clusterName := fmt.Sprintf("k8sv1-%s", *env)

	fmt.Printf("🔍 Cluster Status: %s\n\n", clusterName)

	// Get all nodes for this cluster
	allNodes, err := cli.provider.ListNodes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to list nodes: %v\n", err)
		os.Exit(1)
	}

	// Filter nodes by cluster name prefix
	var clusterNodes []provider.NodeStatus
	for _, node := range allNodes {
		if len(node.Name) >= len(clusterName) && node.Name[:len(clusterName)] == clusterName {
			clusterNodes = append(clusterNodes, node)
		}
	}

	if len(clusterNodes) == 0 {
		fmt.Printf("❌ No nodes found for cluster: %s\n", clusterName)
		return
	}

	// Display cluster information
	var controlPlane, workers []provider.NodeStatus
	for _, node := range clusterNodes {
		if node.Role == "control-plane" {
			controlPlane = append(controlPlane, node)
		} else {
			workers = append(workers, node)
		}
	}

	fmt.Printf("Control Plane Nodes: %d\n", len(controlPlane))
	for _, node := range controlPlane {
		status := "🟢"
		if node.State != "Running" {
			status = "🔴"
		}
		fmt.Printf("  %s %-30s State: %-10s IP: %s\n", status, node.Name, node.State, node.IP)
	}

	fmt.Printf("\nWorker Nodes: %d\n", len(workers))
	for _, node := range workers {
		status := "🟢"
		if node.State != "Running" {
			status = "🔴"
		}
		fmt.Printf("  %s %-30s State: %-10s IP: %s\n", status, node.Name, node.State, node.IP)
	}

	// Overall cluster health
	allRunning := true
	for _, node := range clusterNodes {
		if node.State != "Running" {
			allRunning = false
			break
		}
	}

	fmt.Println()
	if allRunning {
		fmt.Println("✅ Cluster is healthy - all nodes running")
	} else {
		fmt.Println("⚠️  Cluster has issues - some nodes not running")
	}
}

func initializeCLI(platformOverride string) (*CLI, error) {
	// Detect or use override platform
	var p *platform.Platform
	var err error

	if platformOverride != "auto" {
		// Parse override (format: os-arch or os-arch-variant)
		p = &platform.Platform{}
		// Simple parsing - in production would be more robust
		p.DeployMethod = "multipass" // default
		fmt.Printf("Platform override: %s\n", platformOverride)
	} else {
		p, err = platform.Detect()
		if err != nil {
			return nil, fmt.Errorf("failed to detect platform: %w", err)
		}
	}

	// Load configuration
	configPath := filepath.Join("config", "platform-config.yaml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize provider based on deployment method
	var prov provider.Provider
	ctx := context.Background()

	switch p.DeployMethod {
	case "multipass":
		prov = multipass.NewMultipassProvider()
		if err := prov.Initialize(ctx, cfg.GetPlatformConfig(p.String())); err != nil {
			return nil, fmt.Errorf("failed to initialize multipass provider: %w", err)
		}
	case "native":
		prov = native.NewNativeProvider()
		if err := prov.Initialize(ctx, cfg.GetPlatformConfig(p.String())); err != nil {
			return nil, fmt.Errorf("failed to initialize native provider: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported deployment method: %s", p.DeployMethod)
	}

	return &CLI{
		config:   cfg,
		platform: p,
		provider: prov,
	}, nil
}

func printUsage() {
	fmt.Printf(`k8s-deploy - Cross-platform Kubernetes cluster deployment tool

Usage:
  k8s-deploy <command> [flags]

Commands:
  create      Create a new Kubernetes cluster
  destroy     Destroy an existing cluster
  list        List all nodes across clusters
  status      Show cluster status and health
  version     Show version information
  help        Show this help message

Create Flags:
  --env string          Environment to deploy (dev, prd) (default "dev")
  --platform string     Platform override (default "auto")
  --k8s-version string  Kubernetes version (default "1.30.0")
  --cni string          CNI type (calico, flannel) (default "calico")
  --cni-version string  CNI version (default "3.28.0")
  --profile string      Workload profile: development, testing, production (default "development")
  --nodes int           Number of worker nodes (0 for auto-calculation) (default 0)
  --cpus string         CPUs per node (empty for auto-calculation)
  --memory string       Memory per node (empty for auto-calculation)
  --disk string         Disk per node (empty for auto-calculation)
  --auto-size           Automatically calculate optimal resource sizes

Destroy Flags:
  --env string          Environment to destroy (dev, prd) (default "dev")
  --platform string     Platform override (default "auto")
  --yes                 Skip confirmation prompt

Status Flags:
  --env string          Environment to check (dev, prd) (default "dev")
  --platform string     Platform override (default "auto")

Examples:
  # Create a dev cluster with auto-detected platform and optimized resources
  k8s-deploy create --env dev

  # Create a production cluster with auto-sized resources
  k8s-deploy create --env prd --profile production --auto-size

  # Create a cluster with specific resources
  k8s-deploy create --env dev --nodes 3 --cpus 2 --memory 4G --disk 30G

  # Create a testing cluster with recommended resources for the platform
  k8s-deploy create --env dev --profile testing

  # List all nodes
  k8s-deploy list

  # Check cluster status
  k8s-deploy status --env dev

  # Destroy a cluster
  k8s-deploy destroy --env dev

  # Destroy without confirmation
  k8s-deploy destroy --env prd --yes

For more information, visit: https://github.com/yourusername/k8s-infrastructure
`)
}
