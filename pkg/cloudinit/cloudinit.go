package cloudinit

import (
	"bytes"
	"text/template"
)

// Config represents the cloud-init configuration options
type Config struct {
	Role        string // "control-plane" or "worker"
	K8sVersion  string // e.g., "1.30"
	PodCIDR     string
	ServiceCIDR string
}

// GenerateControlPlaneConfig generates cloud-init config for control plane
func GenerateControlPlaneConfig(cfg Config) (string, error) {
	tmpl := `#cloud-config

package_update: true
package_upgrade: true

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
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Control plane node initialization complete after $UPTIME seconds"
`

	t, err := template.New("control-plane").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GenerateWorkerConfig generates cloud-init config for worker nodes
func GenerateWorkerConfig(cfg Config) (string, error) {
	tmpl := `#cloud-config

package_update: true
package_upgrade: true

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
    curl -fsSL https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
    echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v{{.K8sVersion}}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
    apt-get update
    apt-get install -y kubelet kubeadm kubectl
    apt-mark hold kubelet kubeadm kubectl

  # Enable kubelet
  - systemctl enable kubelet

final_message: "Worker node initialization complete after $UPTIME seconds"
`

	t, err := template.New("worker").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}
