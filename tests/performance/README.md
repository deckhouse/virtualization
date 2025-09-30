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
    - [Bootstrapper](#bootstrapper)
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
- **Bootstrapper**: Main script for creating/deleting test resources
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
│   └── ...
├── shatal/             # VM migration tool via node drain
├── statistic/          # Statistics collection
├── status-access-vms/  # VM access and monitoring
├── bootstrapper.sh     # Main script
├── values.yaml         # Configuration
└── Taskfile.yaml       # Task automation
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

### Bootstrapper

Main script for managing test resources.

```bash
# Create resources
./bootstrapper.sh apply --count=10 --namespace=perf --storage-class=ceph-pool-r2-csi-rbd

# Remove resources
./bootstrapper.sh destroy --namespace=perf --resources=all
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
cd shatal
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
cd statistic
task run
```

## 📊 Monitoring

### Grafana Dashboards

- **Virtualization Dashboard**: General virtualization statistics
- **Virtual Machine Dashboard**: Detailed VM statistics
- **Ceph Dashboard**: Storage monitoring

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
cd statistic && task run

# Start wobbling tool
cd shatal && task run
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
cd status-access-vms/ansible
task run

# Start load testing
cd status-access-vms/tank
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
cd shatal
go build -o shatal cmd/shatal/main.go

# Build statistic
cd statistic
go build -o stat cmd/stat/main.go
```

### Adding New Tests

1. Create a new template in `templates/`
2. Add configuration to `values.yaml`
3. Update `bootstrapper.sh` if necessary
4. Add tasks to `Taskfile.yaml`

## 📝 Usage Examples

### Creating Test Environment

```bash
# 1. Create namespace
kubectl create namespace perf

# 2. Create 50 VMs with disks
task apply COUNT=50 NAMESPACE=perf

# 3. Start monitoring
cd statistic && task run

# 4. Start stress testing
cd shatal && task run
```

### Resource Cleanup

```bash
# Remove all resources from namespace
task destroy NAMESPACE=perf
```

## 🤝 Contributing

1. Fork the repository
2. Create a branch for new feature
3. Make changes
4. Add tests
5. Create Pull Request

## 📄 License

Copyright 2024 Flant JSC. Licensed under the Apache License, Version 2.0.