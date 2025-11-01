use colored::*;
use regex::Regex;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::thread;
use std::time::{Duration, Instant};
use rayon::prelude::*;
use crossbeam::thread as crossbeam_thread;

// Constants
const K8S_VERSION: &str = "1.30";
const POD_NETWORK_CIDR: &str = "10.244.0.0/16";
const DEFAULT_CPUS: &str = "2";
const DEFAULT_MEMORY: &str = "4G";
const DEFAULT_DISK: &str = "20G";
const UBUNTU_RELEASE: &str = "noble"; // Ubuntu 24.04 LTS Noble Numbat
const CONTROL_PLANE_NAME: &str = "k8s-prd-c1";
const WORKER_PREFIX: &str = "k8s-prd-w";
const CONTEXT_NAME: &str = "k8s-prd";
const CLUSTER_NAME: &str = "k8s-prd";

#[derive(Debug, Clone)]
struct ClusterConfig {
    control_plane: NodeConfig,
    workers: Vec<NodeConfig>,
    pod_cidr: String,
    k8s_version: String,
    context_name: String,
    cluster_name: String,
}

#[derive(Debug, Clone)]
struct NodeConfig {
    name: String,
    cpus: String,
    memory: String,
    disk: String,
}

#[derive(Debug, Deserialize, Serialize)]
struct MultipassInfo {
    errors: Vec<serde_json::Value>,
    info: HashMap<String, NodeInfo>,
}

#[derive(Debug, Deserialize, Serialize)]
struct NodeInfo {
    ipv4: Vec<String>,
    state: String,
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "🚀 Starting Kubernetes PROD cluster installation with Multipass".green().bold());

    // Check prerequisites
    check_prerequisites()?;

    // Create cluster configuration
    let config = ClusterConfig {
        control_plane: NodeConfig {
            name: CONTROL_PLANE_NAME.to_string(),
            cpus: DEFAULT_CPUS.to_string(),
            memory: DEFAULT_MEMORY.to_string(),
            disk: DEFAULT_DISK.to_string(),
        },
        workers: vec![
            NodeConfig {
                name: format!("{}1", WORKER_PREFIX),
                cpus: DEFAULT_CPUS.to_string(),
                memory: DEFAULT_MEMORY.to_string(),
                disk: DEFAULT_DISK.to_string(),
            },
            NodeConfig {
                name: format!("{}2", WORKER_PREFIX),
                cpus: DEFAULT_CPUS.to_string(),
                memory: DEFAULT_MEMORY.to_string(),
                disk: DEFAULT_DISK.to_string(),
            },
        ],
        pod_cidr: POD_NETWORK_CIDR.to_string(),
        k8s_version: K8S_VERSION.to_string(),
        context_name: CONTEXT_NAME.to_string(),
        cluster_name: CLUSTER_NAME.to_string(),
    };

    // Create cloud-init files
    create_cloud_init_files(&config)?;

    // Launch control plane
    println!("{}", "🎯 Launching control plane node...".cyan().bold());
    launch_node(&config.control_plane, "control-plane-init.yaml")?;

    // Wait for control plane to be ready
    println!("{}", "⏳ Waiting for control plane to initialize...".cyan().bold());
    wait_for_cloud_init(&config.control_plane.name)?;

    // Initialize Kubernetes cluster
    println!("{}", "🔧 Initializing Kubernetes cluster...".cyan().bold());
    let join_command = initialize_cluster(&config)?;

    // Save kubeconfig
    println!("{}", "💾 Saving kubeconfig...".cyan().bold());
    save_kubeconfig(&config)?;

    // Install CNI (Calico)
    println!("{}", "🌐 Installing Calico CNI...".cyan().bold());
    install_calico(&config)?;

    // Launch worker nodes in parallel (ACCELERATED)
    println!("{}", "🎯 Launching worker nodes in parallel...".cyan().bold());
    let results: Vec<Result<(), String>> = config
        .workers
        .par_iter()
        .map(|worker| {
            println!("{}", format!("  • Launching {}...", worker.name).cyan());
            launch_and_wait_worker(worker, "worker-init.yaml")
                .map_err(|e| format!("Failed to launch {}: {}", worker.name, e))?;
            Ok(())
        })
        .collect();

    // Check if all launches succeeded
    for result in results {
        result.map_err(|e| e.to_string())?;
    }
    println!("{}", "  ✓ All workers launched".green());

    // Join worker nodes to cluster in parallel (ACCELERATED)
    println!("{}", "🔗 Joining workers to cluster in parallel...".cyan().bold());
    let join_results: Vec<Result<(), String>> = config
        .workers
        .par_iter()
        .map(|worker| {
            println!("{}", format!("  • Joining {}...", worker.name).cyan());
            join_worker(&worker.name, &join_command)
                .map_err(|e| format!("Failed to join {}: {}", worker.name, e))?;
            Ok(())
        })
        .collect();

    // Check if all joins succeeded
    for result in join_results {
        result.map_err(|e| e.to_string())?;
    }
    println!("{}", "  ✓ All workers joined".green());

    // Wait for all nodes to be ready with smart polling (ACCELERATED)
    println!("{}", "⏳ Waiting for all nodes to be ready (smart polling)...".cyan().bold());
    let all_node_names: Vec<String> = std::iter::once(config.control_plane.name.clone())
        .chain(config.workers.iter().map(|w| w.name.clone()))
        .collect();
    wait_for_nodes_ready(&config.control_plane.name, &all_node_names, 300)?;
    println!("{}", "  ✓ All nodes ready".green());

    // Install metrics-server, Helm, and Ingress in parallel (ACCELERATED)
    println!("{}", "📊📦🌐 Installing metrics-server, Helm, and Ingress in parallel...".cyan().bold());
    let control_plane_name = config.control_plane.name.clone();
    let control_plane_name2 = config.control_plane.name.clone();
    let control_plane_name3 = config.control_plane.name.clone();

    crossbeam_thread::scope(|s| {
        let h1 = s.spawn(move |_| {
            println!("{}", "  • Installing metrics-server...".cyan());
            install_metrics_server(&control_plane_name)
                .map_err(|e| format!("Failed to install metrics-server: {}", e))
        });

        let h2 = s.spawn(move |_| {
            println!("{}", "  • Installing Helm...".cyan());
            install_helm(&control_plane_name2)
                .map_err(|e| format!("Failed to install Helm: {}", e))
        });

        let h3 = s.spawn(move |_| {
            println!("{}", "  • Installing Ingress NGINX...".cyan());
            install_ingress_nginx(&control_plane_name3)
                .map_err(|e| format!("Failed to install Ingress NGINX: {}", e))
        });

        let r1 = h1.join().unwrap();
        let r2 = h2.join().unwrap();
        let r3 = h3.join().unwrap();

        r1.map_err(|e| e.to_string())?;
        r2.map_err(|e| e.to_string())?;
        r3.map_err(|e| e.to_string())?;

        println!("{}", "  ✓ metrics-server, Helm, and Ingress installed".green());
        Ok::<(), String>(())
    }).unwrap()?;

    // Install kube-prometheus-stack
    println!("{}", "🔭 Installing kube-prometheus-stack (Prometheus + Grafana)...".cyan().bold());
    install_kube_prometheus_stack(&config.control_plane.name)?;

    // Expose Grafana and Prometheus
    println!("{}", "🌐 Exposing Grafana and Prometheus...".cyan().bold());
    expose_grafana(&config.control_plane.name)?;

    // Get Grafana password
    let grafana_password = get_grafana_password(&config.control_plane.name)?;

    // Get control plane IP for access info
    let control_plane_ip = get_node_ip(&config.control_plane.name)?;

    // Display cluster status
    println!("\n{}", "✅ PROD Cluster installation complete!".green().bold());
    display_cluster_info(&config, &control_plane_ip, &grafana_password);

    Ok(())
}

fn check_prerequisites() -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "🔍 Checking prerequisites...".cyan().bold());

    // Check if multipass is installed
    match Command::new("which").arg("multipass").output() {
        Ok(output) if output.status.success() => {},
        _ => {
            return Err("multipass is not installed. Install it from https://multipass.run/".into());
        }
    }

    // Check if kubectl is installed
    match Command::new("which").arg("kubectl").output() {
        Ok(output) if output.status.success() => {},
        _ => {
            println!("{}", "⚠️  kubectl not found. You'll need to install it to manage the cluster.".yellow());
        }
    }

    println!("{}", "✓ Prerequisites check passed".green());
    Ok(())
}

fn create_cloud_init_files(config: &ClusterConfig) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "📝 Creating cloud-init configuration files...".cyan().bold());

    let control_plane_init = get_control_plane_cloud_init(&config.k8s_version);
    fs::write("control-plane-init.yaml", control_plane_init)?;

    let worker_init = get_worker_cloud_init(&config.k8s_version);
    fs::write("worker-init.yaml", worker_init)?;

    println!("{}", "✓ Cloud-init files created".green());
    Ok(())
}

fn get_control_plane_cloud_init(k8s_version: &str) -> String {
    let ssh_key = get_ssh_public_key();
    let ssh_key_config = if !ssh_key.is_empty() {
        format!("\nssh_authorized_keys:\n  - {}\n", ssh_key)
    } else {
        String::new()
    };

    format!(r#"#cloud-config

package_update: true
package_upgrade: true
{}
bootcmd:
  - modprobe overlay
  - modprobe br_netfilter

write_files:
  - path: /etc/modules-load.d/k8s.conf
    content: |
      overlay
      br_netfilter
  - path: /etc/sysctl.d/k8s.conf
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward                 = 1
  - path: /etc/apt/keyrings/.placeholder
    content: ""

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - socat
  - conntrack
  - ipset

runcmd:
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab

  # Apply sysctl settings
  - sysctl --system

  # Install containerd
  - |
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update
    apt-get install -y containerd.io

  # Configure containerd
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd

  # Install Kubernetes packages
  - |
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v{}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v{}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Control plane node initialization complete after $UPTIME seconds"
"#, ssh_key_config, k8s_version, k8s_version)
}

fn get_worker_cloud_init(k8s_version: &str) -> String {
    let ssh_key = get_ssh_public_key();
    let ssh_key_config = if !ssh_key.is_empty() {
        format!("\nssh_authorized_keys:\n  - {}\n", ssh_key)
    } else {
        String::new()
    };

    format!(r#"#cloud-config

package_update: true
package_upgrade: true
{}
bootcmd:
  - modprobe overlay
  - modprobe br_netfilter

write_files:
  - path: /etc/modules-load.d/k8s.conf
    content: |
      overlay
      br_netfilter
  - path: /etc/sysctl.d/k8s.conf
    content: |
      net.bridge.bridge-nf-call-iptables  = 1
      net.bridge.bridge-nf-call-ip6tables = 1
      net.ipv4.ip_forward                 = 1
  - path: /etc/apt/keyrings/.placeholder
    content: ""

packages:
  - apt-transport-https
  - ca-certificates
  - curl
  - gnupg
  - socat
  - conntrack
  - ipset

runcmd:
  # Disable swap
  - swapoff -a
  - sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab

  # Apply sysctl settings
  - sysctl --system

  # Install containerd
  - |
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null
    apt-get update
    apt-get install -y containerd.io

  # Configure containerd
  - mkdir -p /etc/containerd
  - containerd config default | tee /etc/containerd/config.toml
  - sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
  - systemctl restart containerd
  - systemctl enable containerd

  # Install Kubernetes packages
  - |
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v{}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v{}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Worker node initialization complete after $UPTIME seconds"
"#, ssh_key_config, k8s_version, k8s_version)
}

fn launch_node(node: &NodeConfig, cloud_init_file: &str) -> Result<(), Box<dyn std::error::Error>> {
    let output = Command::new("multipass")
        .args(&[
            "launch",
            "--name", &node.name,
            "--cpus", &node.cpus,
            "--memory", &node.memory,
            "--disk", &node.disk,
            "--network", "en0",
            "--cloud-init", cloud_init_file,
            UBUNTU_RELEASE,
        ])
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .output()?;

    if !output.status.success() {
        return Err(format!("Failed to launch node {}", node.name).into());
    }

    Ok(())
}

fn wait_for_cloud_init(node_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    let max_attempts = 60;
    for i in 0..max_attempts {
        let output = Command::new("multipass")
            .args(&["exec", node_name, "--", "cloud-init", "status"])
            .output();

        if let Ok(output) = output {
            let status = String::from_utf8_lossy(&output.stdout);
            if status.contains("status: done") {
                println!("{}", format!("  ✓ {} is ready", node_name).green());
                return Ok(());
            }
        }

        thread::sleep(Duration::from_secs(5));
        println!("  Attempt {}/{}...", i + 1, max_attempts);
    }

    Err(format!("Timeout waiting for cloud-init on {}", node_name).into())
}

// Smart polling function to check if a node is ready in Kubernetes
fn is_node_ready(control_plane: &str, node_name: &str) -> bool {
    let output = Command::new("multipass")
        .args(&[
            "exec",
            control_plane,
            "--",
            "kubectl",
            "get",
            "node",
            node_name,
            "-o",
            "jsonpath={.status.conditions[?(@.type=='Ready')].status}",
        ])
        .output();

    if let Ok(output) = output {
        let status = String::from_utf8_lossy(&output.stdout);
        return status.trim() == "True";
    }
    false
}

// Wait for all nodes to be ready with smart polling
fn wait_for_nodes_ready(
    control_plane: &str,
    node_names: &[String],
    timeout_secs: u64,
) -> Result<(), Box<dyn std::error::Error>> {
    let start = Instant::now();
    let timeout = Duration::from_secs(timeout_secs);

    loop {
        let all_ready = node_names
            .iter()
            .all(|name| is_node_ready(control_plane, name));

        if all_ready {
            return Ok(());
        }

        if start.elapsed() > timeout {
            return Err(format!(
                "Timeout waiting for nodes to be ready after {} seconds",
                timeout_secs
            )
            .into());
        }

        thread::sleep(Duration::from_secs(2));
    }
}

// Wrapper function to launch and wait for a worker node (for parallel execution)
fn launch_and_wait_worker(
    worker: &NodeConfig,
    init_file: &str,
) -> Result<(), Box<dyn std::error::Error>> {
    launch_node(worker, init_file)?;
    wait_for_cloud_init(&worker.name)?;
    Ok(())
}

fn get_node_ip(node_name: &str) -> Result<String, Box<dyn std::error::Error>> {
    let output = Command::new("multipass")
        .args(&["info", node_name, "--format", "json"])
        .output()?;

    if !output.status.success() {
        return Err(format!("Failed to get node info for {}", node_name).into());
    }

    let info: MultipassInfo = serde_json::from_slice(&output.stdout)?;

    let node_info = info.info.get(node_name)
        .ok_or_else(|| format!("No info found for node {}", node_name))?;

    let ip = node_info.ipv4.first()
        .ok_or_else(|| format!("No IP address found for node {}", node_name))?;

    Ok(ip.clone())
}

fn initialize_cluster(config: &ClusterConfig) -> Result<String, Box<dyn std::error::Error>> {
    // Get control plane IP
    let control_plane_ip = get_node_ip(&config.control_plane.name)?;
    println!("  Control plane IP: {}", control_plane_ip);

    // Initialize cluster
    let init_cmd = format!(
        "sudo kubeadm init --pod-network-cidr={} --apiserver-advertise-address={} --control-plane-endpoint={}",
        config.pod_cidr,
        control_plane_ip,
        control_plane_ip
    );

    let output = Command::new("multipass")
        .args(&["exec", &config.control_plane.name, "--", "bash", "-c", &init_cmd])
        .output()?;

    if !output.status.success() {
        println!("Init output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("kubeadm init failed".into());
    }

    let output_str = String::from_utf8_lossy(&output.stdout);

    // Extract join command
    let join_regex = Regex::new(r"kubeadm join [^\n]+\n[^\n]+discovery-token-ca-cert-hash[^\n]+")?;
    let join_matches = join_regex.find(&output_str)
        .ok_or("Could not find join command in kubeadm output")?;

    // Clean up join command (remove line continuations)
    let join_command = join_matches.as_str()
        .replace("\\\n", "")
        .replace("\\", "")
        .trim()
        .to_string();

    // Setup kubeconfig on control plane
    let kubeconfig_setup = r#"
        mkdir -p $HOME/.kube
        sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
        sudo chown $(id -u):$(id -g) $HOME/.kube/config
    "#;

    let output = Command::new("multipass")
        .args(&["exec", &config.control_plane.name, "--", "bash", "-c", kubeconfig_setup])
        .output()?;

    if !output.status.success() {
        return Err("Failed to setup kubeconfig".into());
    }

    println!("{}", "  ✓ Cluster initialized".green());
    Ok(join_command)
}

fn save_kubeconfig(config: &ClusterConfig) -> Result<(), Box<dyn std::error::Error>> {
    let home_dir = std::env::var("HOME")?;
    let kube_dir = PathBuf::from(&home_dir).join(".kube");
    fs::create_dir_all(&kube_dir)?;

    let kubeconfig_path = kube_dir.join("config");
    let temp_kubeconfig_path = kube_dir.join(format!("config.{}.tmp", &config.context_name));

    // Get kubeconfig from control plane
    let output = Command::new("multipass")
        .args(&["exec", &config.control_plane.name, "--", "sudo", "cat", "/etc/kubernetes/admin.conf"])
        .output()?;

    if !output.status.success() {
        return Err("Failed to retrieve kubeconfig".into());
    }

    let mut kubeconfig = String::from_utf8_lossy(&output.stdout).to_string();

    // Get control plane IP
    let control_plane_ip = get_node_ip(&config.control_plane.name)?;

    // Replace internal IP with multipass IP and update context/cluster names
    let server_regex = Regex::new(r"server: https://[^:]+:")?;
    kubeconfig = server_regex.replace_all(&kubeconfig, format!("server: https://{}:", control_plane_ip)).to_string();

    // Replace cluster name
    kubeconfig = kubeconfig.replace("name: kubernetes", &format!("name: {}", &config.cluster_name));
    kubeconfig = kubeconfig.replace("cluster: kubernetes", &format!("cluster: {}", &config.cluster_name));

    // Replace context name
    kubeconfig = kubeconfig.replace("name: kubernetes-admin@kubernetes", &format!("name: {}", &config.context_name));
    kubeconfig = kubeconfig.replace("current-context: kubernetes-admin@kubernetes", &format!("current-context: {}", &config.context_name));

    // Write temporary kubeconfig
    fs::write(&temp_kubeconfig_path, &kubeconfig)?;

    // Merge with existing kubeconfig if it exists
    if kubeconfig_path.exists() {
        // Existing kubeconfig exists, merge them
        println!("{}", "  📦 Merging with existing kubeconfig...".cyan());

        // Backup existing kubeconfig
        let timestamp = chrono::Local::now().format("%Y%m%d-%H%M%S");
        let backup_path = kubeconfig_path.with_extension(format!("backup.{}", timestamp));
        match fs::copy(&kubeconfig_path, &backup_path) {
            Ok(_) => println!("{}", format!("  📦 Existing kubeconfig backed up to: {}", backup_path.display()).green()),
            Err(e) => println!("{}", format!("⚠️  Failed to backup existing kubeconfig: {}", e).yellow()),
        }

        // Merge kubeconfigs using kubectl config view --flatten
        let merge_script = format!(
            "KUBECONFIG={}:{} kubectl config view --flatten > {}.merged && mv {}.merged {}",
            kubeconfig_path.display(),
            temp_kubeconfig_path.display(),
            kubeconfig_path.display(),
            kubeconfig_path.display(),
            kubeconfig_path.display()
        );

        let output = Command::new("bash")
            .args(&["-c", &merge_script])
            .output()?;

        if !output.status.success() {
            println!("Merge error: {}", String::from_utf8_lossy(&output.stderr));
            return Err("Failed to merge kubeconfigs".into());
        }

        // Remove temporary file
        let _ = fs::remove_file(&temp_kubeconfig_path);

        println!("{}", format!("  ✓ Kubeconfig merged with context: {}", &config.context_name).green());
    } else {
        // No existing kubeconfig, just rename temp to config
        fs::rename(&temp_kubeconfig_path, &kubeconfig_path)?;
        println!("{}", format!("  ✓ Kubeconfig saved to: {}", kubeconfig_path.display()).green());
    }

    // Set current context to the new cluster
    let set_context_output = Command::new("kubectl")
        .args(&["config", "use-context", &config.context_name])
        .output();

    match set_context_output {
        Ok(output) if output.status.success() => {
            println!("{}", format!("  ✓ Current context set to: {}", &config.context_name).green());
        }
        _ => {
            println!("{}", format!("⚠️  Failed to set current context to {}", &config.context_name).yellow());
        }
    }

    Ok(())
}

fn install_calico(config: &ClusterConfig) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Installing Calico CNI...");

    let install_script = format!(
        r#"
        kubectl create -f https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/tigera-operator.yaml
        sleep 10
        curl -sS -O https://raw.githubusercontent.com/projectcalico/calico/v3.28.0/manifests/custom-resources.yaml
        sed -i 's#cidr: 192\.168\.0\.0/16#cidr: {}#g' custom-resources.yaml
        kubectl create -f custom-resources.yaml
        "#,
        config.pod_cidr
    );

    let output = Command::new("multipass")
        .args(&["exec", &config.control_plane.name, "--", "bash", "-c", &install_script])
        .output()?;

    if !output.status.success() {
        println!("Calico installation output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to install Calico".into());
    }

    println!("  Waiting for Calico to be ready...");
    thread::sleep(Duration::from_secs(30));

    println!("{}", "  ✓ Calico CNI installed".green());
    Ok(())
}

fn install_metrics_server(control_plane_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Installing metrics-server...");

    let install_script = r#"
        kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
        sleep 5
        kubectl patch deployment metrics-server -n kube-system --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}]'
        sleep 10
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", install_script])
        .output()?;

    if !output.status.success() {
        println!("metrics-server installation output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to install metrics-server".into());
    }

    println!("  Waiting for metrics-server to be ready...");
    thread::sleep(Duration::from_secs(20));

    println!("{}", "  ✓ metrics-server installed".green());
    Ok(())
}

fn install_helm(control_plane_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Installing Helm...");

    let install_script = r#"
        curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", install_script])
        .output()?;

    if !output.status.success() {
        println!("Helm installation output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to install Helm".into());
    }

    println!("{}", "  ✓ Helm installed".green());
    Ok(())
}

fn install_kube_prometheus_stack(control_plane_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Installing kube-prometheus-stack...");

    let install_script = r#"
        helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
        helm repo update
        kubectl create namespace monitoring
        helm install kube-prometheus-stack prometheus-community/kube-prometheus-stack \
            --namespace monitoring \
            --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
        sleep 30
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", install_script])
        .output()?;

    if !output.status.success() {
        println!("kube-prometheus-stack installation output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to install kube-prometheus-stack".into());
    }

    println!("  Waiting for monitoring stack to be ready...");
    thread::sleep(Duration::from_secs(30));

    println!("{}", "  ✓ kube-prometheus-stack installed".green());
    Ok(())
}

fn expose_grafana(control_plane_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Exposing Grafana as NodePort...");

    let expose_script = r#"
        kubectl patch svc kube-prometheus-stack-grafana -n monitoring -p '{"spec":{"type":"NodePort","ports":[{"port":80,"nodePort":30080}]}}'
        kubectl patch svc kube-prometheus-stack-prometheus -n monitoring -p '{"spec":{"type":"NodePort","ports":[{"port":9090,"nodePort":30090}]}}'
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", expose_script])
        .output()?;

    if !output.status.success() {
        println!("Grafana exposure output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to expose Grafana".into());
    }

    println!("{}", "  ✓ Grafana exposed on port 30080".green());
    println!("{}", "  ✓ Prometheus exposed on port 30090".green());
    Ok(())
}

fn get_grafana_password(control_plane_name: &str) -> Result<String, Box<dyn std::error::Error>> {
    let get_password_script = r#"
        kubectl --namespace monitoring get secrets kube-prometheus-stack-grafana -o jsonpath="{.data.admin-password}" | base64 -d
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", get_password_script])
        .output()?;

    if !output.status.success() {
        return Err("Failed to get Grafana password".into());
    }

    Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
}

fn install_ingress_nginx(control_plane_name: &str) -> Result<(), Box<dyn std::error::Error>> {
    println!("  Installing Ingress NGINX...");

    let install_script = r#"
        kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.9.4/deploy/static/provider/cloud/deploy.yaml
        sleep 10
    "#;

    let output = Command::new("multipass")
        .args(&["exec", control_plane_name, "--", "bash", "-c", install_script])
        .output()?;

    if !output.status.success() {
        println!("Ingress NGINX installation output: {}", String::from_utf8_lossy(&output.stdout));
        return Err("Failed to install Ingress NGINX".into());
    }

    println!("  Waiting for ingress controller to be ready...");
    thread::sleep(Duration::from_secs(30));

    println!("{}", "  ✓ Ingress NGINX installed".green());
    Ok(())
}

fn join_worker(worker_name: &str, join_command: &str) -> Result<(), Box<dyn std::error::Error>> {
    let output = Command::new("multipass")
        .args(&["exec", worker_name, "--", "sudo", "bash", "-c", join_command])
        .output()?;

    if !output.status.success() {
        println!("Join output: {}", String::from_utf8_lossy(&output.stdout));
        return Err(format!("Failed to join worker {}", worker_name).into());
    }

    println!("{}", format!("  ✓ {} joined successfully", worker_name).green());
    Ok(())
}

fn get_ssh_public_key() -> String {
    let home_dir = match std::env::var("HOME") {
        Ok(dir) => dir,
        Err(_) => return String::new(),
    };

    // Try Ed25519 key first (more modern)
    let ed25519_path = PathBuf::from(&home_dir).join(".ssh/id_ed25519.pub");
    if let Ok(key) = fs::read_to_string(&ed25519_path) {
        return key.trim().to_string();
    }

    // Fall back to RSA key
    let rsa_path = PathBuf::from(&home_dir).join(".ssh/id_rsa.pub");
    if let Ok(key) = fs::read_to_string(&rsa_path) {
        return key.trim().to_string();
    }

    String::new()
}

fn display_cluster_info(config: &ClusterConfig, control_plane_ip: &str, grafana_password: &str) {
    println!("\n{}", "=".repeat(60));
    println!("{}", "🎉 Kubernetes PROD Cluster Ready!".green().bold());
    println!("{}", "=".repeat(60));

    println!("\n📦 Cluster Configuration:");
    println!("  • Environment:        PROD");
    println!("  • Context Name:       {}", &config.context_name);
    println!("  • Cluster Name:       {}", &config.cluster_name);
    println!("  • Kubernetes Version: {}", config.k8s_version);
    println!("  • Pod Network CIDR:   {}", config.pod_cidr);
    println!("  • Ubuntu Release:     24.04 LTS (Noble Numbat)");
    println!("  • Container Runtime:  containerd");
    println!("  • CNI Plugin:         Calico");
    println!("  • Monitoring:         kube-prometheus-stack");
    println!("  • Ingress:            NGINX Ingress Controller");

    println!("\n🖥️  Cluster Nodes:");
    println!("  • Control Plane: {}", config.control_plane.name);
    for worker in &config.workers {
        println!("  • Worker: {}", worker.name);
    }

    println!("\n📊 Monitoring Stack:");
    println!("  • Prometheus:  http://{}:30090", control_plane_ip);
    println!("  • Grafana:     http://{}:30080", control_plane_ip);
    println!("  • Username:    admin");
    println!("  • Password:    {}", grafana_password);
    println!("\n  {} Pre-configured Grafana dashboards including:", "28".cyan().bold());
    println!("  • Kubernetes / Compute Resources / Cluster");
    println!("  • Kubernetes / Networking / Cluster");
    println!("  • Node Exporter / Nodes");
    println!("  • And many more...");

    println!("\n🔧 Multi-Cluster Management:");
    println!("  • Switch to PROD:  kubectl config use-context {}", &config.context_name);
    println!("  • List contexts:   kubectl config get-contexts");
    println!("  • Current context: kubectl config current-context");

    println!("\n📋 Useful Commands:");
    println!("  • Check nodes:       kubectl get nodes");
    println!("  • Check pods:        kubectl get pods -A");
    println!("  • View metrics:      kubectl top nodes");
    println!("  • Shell to control:  multipass shell {}", config.control_plane.name);
    println!("  • Stop cluster:      multipass stop {} {} {}", config.control_plane.name, config.workers[0].name, config.workers[1].name);
    println!("  • Start cluster:     multipass start {} {} {}", config.control_plane.name, config.workers[0].name, config.workers[1].name);
    println!("  • Delete cluster:    multipass delete {} {} {} --purge", config.control_plane.name, config.workers[0].name, config.workers[1].name);

    println!("\n📁 Kubeconfig: ~/.kube/config");
    println!("{}\n", "=".repeat(60));

    // Display actual node status
    println!("📊 Current cluster status:");
    let _ = Command::new("kubectl")
        .args(&["get", "nodes", "-o", "wide"])
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .output();
}
