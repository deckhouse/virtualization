- [Performance Testing Framework](#performance-testing-framework)
  - [🏗️ Architecture](#️-architecture)
    - [Core Components](#core-components)
    - [Directory Structure](#directory-structure)
  - [🚀 Quick Start](#-quick-start)
    - [Prerequisites](#prerequisites)
    - [Install Dependencies](#install-dependencies)
    - [Create Test Resources](#create-test-resources)
    - [Remove Resources](#remove-resources)
  - [🛠️ Tools](#️-tools)
    - [bootstrap](#bootstrap)
    - [Taskfile Integration](#taskfile-integration)
    - [Shatal - VM Wobbling Tool](#shatal---vm-wobbling-tool)
    - [Evicter - Migration Tool](#evicter---migration-tool)
    - [Statistics - Statistics Collection](#statistics---statistics-collection)
  - [📊 Monitoring](#-monitoring)
    - [Grafana Dashboards](#grafana-dashboards)
    - [Prometheus Rules](#prometheus-rules)
  - [⚙️ Configuration](#️-configuration)
    - [values.yaml](#valuesyaml)
    - [Resource Types](#resource-types)
  - [🎯 Testing Scenarios](#-testing-scenarios)
    - [1. Basic Performance Testing](#1-basic-performance-testing)
    - [2. Migration Testing](#2-migration-testing)
    - [3. VM Access Testing](#3-vm-access-testing)
  - [📈 Metrics and Monitoring](#-metrics-and-monitoring)
    - [Key Metrics](#key-metrics)
    - [Dashboards](#dashboards)
  - [🔧 Development](#-development)
    - [Building Tools](#building-tools)
    - [Adding New Tests](#adding-new-tests)
  - [📝 Usage Examples](#-usage-examples)
    - [Creating Test Environment](#creating-test-environment)
    - [Resource Cleanup](#resource-cleanup)
  - [🤝 Contributing](#-contributing)
  - [📄 License](#-license)

# Performance Testing Framework

A comprehensive framework for virtualization performance testing, including tools for creating, migrating, and monitoring virtual machines in Kubernetes.

## 🏗️ Architecture

### Core Components

- **Helm Chart**: Resource management through Helm
- **bootstrap**: Main script for creating/deleting test resources
- **Shatal**: Virtual machine "wobbling" tool
- **Evicter**: Continuous VM migration tool
- **Statistics**: Performance statistics collection
- **Monitoring**: Grafana dashboards and Prometheus rules

### Directory Structure

```
performance/
├── templates/           # Kubernetes manifests
├── tools/              # Testing tools
│   ├── evicter/        # VM migration
│   ├── shatal/         # VM migration tool via node drain
│   ├── statistic/       # Statistics collection
│   └── status-access-vms/  # VM access and monitoring
├── monitoring/         # Grafana dashboards
├── ssh/               # SSH keys for VM access
├── bootstrap.sh      # Main script
├── values.yaml        # Configuration
└── Taskfile.yaml      # Task automation
```

## 🚀 Quick Start

### Prerequisites

- Kubernetes cluster with virtualization support
- Helm 3
- kubectl
- Go (for building tools)

### Install Dependencies

```bash
task check_or_install_software
```

### Create Test Resources

```bash
# Create 10 virtual machines
task apply COUNT=10

# Create only disks
task apply:disks COUNT=5

# Create only VMs
task apply:vms COUNT=5
```

### Remove Resources

```bash
# Remove all resources
task destroy

# Remove only VMs
task destroy:vms

# Remove only disks
task destroy:disks
```

## 🛠️ Tools

### bootstrap

Main script for managing test resources.

**Available Flags:**
- `--count, -c`: Number of virtual machines to create (required for apply)
- `--namespace, -n`: Namespace for resources (default: current context namespace)
- `--storage-class, -s`: Storage class for VM disks
- `--name, -r`: Release name (default: performance)
- `--resources, -R`: Resources to manage - 'vds', 'vms', or 'all' (default: all)
- `--resources-prefix, -p`: Prefix for resource names (default: performance)

```bash
# Create resources (using long flags)
./bootstrap.sh apply --count=10 --namespace=perf --storage-class=ceph-pool-r2-csi-rbd

# Create resources (using short flags)
./bootstrap.sh apply -c 10 -n perf -s ceph-pool-r2-csi-rbd

# Create only disks
./bootstrap.sh apply -c 5 -n perf -R vds -r performance-disks

# Create only VMs (assuming disks exist)
./bootstrap.sh apply -c 5 -n perf -R vms -r performance-vms

# Remove resources
./bootstrap.sh destroy --namespace=perf --resources=all
# or using short flags
./bootstrap.sh destroy -n perf -R all

# Remove specific resources
./bootstrap.sh destroy -n perf -R vms -r performance-vms
```

### Taskfile Integration

The framework includes comprehensive Taskfile integration for easy automation:

**Available Tasks:**
```bash
# Basic operations
task apply COUNT=10                    # Create 10 VMs
task destroy                           # Remove all resources
task apply:disks COUNT=5               # Create only disks
task apply:vms COUNT=5                 # Create only VMs
task destroy:disks                     # Remove only disks
task destroy:vms                       # Remove only VMs

# Two-step deployment
task apply:all COUNT=30                # Create disks first, then VMs
task destroy:all                       # Remove VMs first, then disks

# Utility tasks
task render                           # Preview Helm templates
task help                             # Show bootstrap.sh help
task check_or_install_software       # Install dependencies
```

**Environment Variables:**
```bash
# Set custom values
COUNT=50 NAMESPACE=test STORAGE_CLASS=ceph-pool-r2-csi-rbd task apply
```

### Shatal - VM Wobbling Tool

Tool for continuous stress testing of virtual machines.

**Features:**
- Node draining with VM migration
- CPU core fraction changes (10% ↔ 25%)
- VM creation/deletion
- Configurable operation weights

**Usage:**
```bash
cd tools/shatal
KUBECONFIG=$(cat ~/.kube/config | base64 -w 0)
KUBECONFIG_BASE64=$KUBECONFIG task run
```

### Evicter - Migration Tool

Continuous migration of a specified percentage of virtual machines.

```bash
# Migrate 20% of VMs in namespace 'perf' for 1 hour
./evicter --target=20 --duration=1h --ns=perf
```

### Statistics - Statistics Collection

```bash
cd tools/statistic
task run
```

## 📊 Monitoring

### Grafana Dashboards

The monitoring directory contains pre-configured Grafana dashboards:

- **virtualization-dashboard.yaml**: General virtualization statistics
- **virtual-machine-dashboard.yaml**: Detailed VM statistics  
- **ceph-dashboard.yaml**: Storage monitoring

### SSH Access

The `ssh/` directory contains SSH keys for VM access:
- `id_ed`: Private SSH key
- `id_ed.pub`: Public SSH key

### Prometheus Rules

Configured rules for performance monitoring and alerts.

## ⚙️ Configuration

### values.yaml

Main configuration parameters:

```yaml
# Number of resources
count: 1

# Resources to create
resources:
  default: all  # all, vms, vds, vi
  prefix: "performance"
  storageClassName: "ceph-pool-r2-csi-rbd"
  
  # VM configuration
  vm:
    runPolicy: AlwaysOnUnlessStoppedManually
    restartApprovalMode: Dynamic
    spec:
      cpu:
        cores: 1
        coreFraction: 10%
      memory:
        size: 256Mi
        
  # Virtual disk configuration
  vd:
    spec:
      type: vd  # vi or vd
      diskSize: 300Mi
      
  # Virtual image configuration
  vi:
    spec:
      type: vi  # vi or pvc
      baseImage:
        name: alpine
        url: "https://example.com/alpine.qcow2"
```

### Resource Types

**VirtualDisk (vd.spec.type):**
- `vi`: creates VMs with VirtualImage in blockDeviceRefs
- `vd`: creates VMs with corresponding VirtualDisk

**VirtualImage (vi.spec.type):**
- `vi`: creates image through ContainerRegistry
- `pvc`: creates image through PersistentVolumeClaim

## 🎯 Testing Scenarios

### 1. Basic Performance Testing

```bash
# Create 100 VMs for load testing
task apply COUNT=100

# Start statistics collection
cd tools/statistic && task run

# Start wobbling tool
cd tools/shatal && task run
```

### 2. Migration Testing

```bash
# Start continuous migration of 30% VMs
cd tools/evicter
go run cmd/main.go --target=30 --duration=2h
```

### 3. VM Access Testing

```bash
# Configure VM access through Ansible
cd tools/status-access-vms/ansible
task run

# Start load testing
cd tools/status-access-vms/tank
task run
```

## 📈 Metrics and Monitoring

### Key Metrics

- VM creation time
- VM migration time
- Resource usage (CPU, memory, disk)
- VM availability
- Storage performance

### Dashboards

All dashboards are automatically deployed when creating resources and are available in Grafana.

## 🔧 Development

### Building Tools

```bash
# Build evicter
cd tools/evicter
go build -o evicter cmd/main.go

# Build shatal
cd tools/shatal
go build -o shatal cmd/shatal/main.go

# Build statistic
cd tools/statistic
go build -o stat cmd/stat/main.go
```

### Adding New Tests

1. Create a new template in `templates/`
2. Add configuration to `values.yaml`
3. Update `bootstrap.sh` if necessary
4. Add tasks to `Taskfile.yaml`

## 📝 Usage Examples

### Creating Test Environment

```bash
# 1. Create namespace
kubectl create namespace perf

# 2. Create 50 VMs with disks
task apply COUNT=50 NAMESPACE=perf

# 3. Start monitoring
cd tools/statistic && task run

# 4. Start stress testing
cd tools/shatal && task run
```

### Resource Cleanup

```bash
# Remove all resources from namespace
task destroy NAMESPACE=perf
```

## 🔧 Troubleshooting

### Common Issues

**1. Helm Template Errors**
```bash
# If you get template errors, check the values structure
helm template test . --values values.yaml

# Debug with verbose output
task apply COUNT=1 --verbose
```

**2. Resource Conflicts**
```bash
# If resources are stuck in terminating state
kubectl delete virtualmachines --all -n perf --force --grace-period=0
kubectl delete virtualdisks --all -n perf --force --grace-period=0

# Clean up secrets
kubectl delete secrets --all -n perf
```

**3. Namespace Issues**
```bash
# Check current namespace
kubectl config view --minify -o jsonpath='{..namespace}'

# Switch to correct namespace
kubectl config set-context --current --namespace=perf
```

**4. Storage Class Issues**
```bash
# List available storage classes
kubectl get storageclass

# Use correct storage class
task apply COUNT=5 STORAGE_CLASS=ceph-pool-r2-csi-rbd
```

### Debug Commands

```bash
# Check Helm releases
helm list -n perf

# Check resource status
kubectl get all -n perf
kubectl get virtualmachines -n perf
kubectl get virtualdisks -n perf

# Check logs
kubectl logs -n perf -l app=performance
```

## 🤝 Contributing

1. Fork the repository
2. Create a branch for new feature
3. Make changes
4. Add tests
5. Create Pull Request

## 📄 License

Copyright 2024 Flant JSC. Licensed under the Apache License, Version 2.0.