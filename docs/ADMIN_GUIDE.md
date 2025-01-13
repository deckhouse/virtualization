---
title: "Admin guide"
weight: 40
---

## Introduction

This guide is intended for administrators of Deckhouse Virtualization Platform and describes how to create and modify cluster resources.

The administrator also has rights to manage project resources, which are described in the [“User Guide”](./USER_GUIDE.md) document.

## Images

The `ClusterVirtualImage` resource is used to load virtual machine images into the intra-cluster storage, after which it can be used to create virtual machine disks. It is available in all cluster namespaces/projects.

The image creation process includes the following steps:

- The user creates a `ClusterVirtualImage` resource.
- Once created, the image is automatically uploaded from the source specified in the specification to the storage (DVCR).
- Once the upload is complete, the resource becomes available for disk creation.

There are different types of images:

- ISO image - an installation image used for the initial installation of an operating system. Such images are released by OS vendors and are used for installation on physical and virtual servers.
- Preinstalled disk image - contains an already installed and configured operating system ready for use after the virtual machine is created. These images are offered by several vendors and can be provided in formats such as qcow2, raw, vmdk, and others.

Example of resource for obtaining virtual machine images: https://cloud-images.ubuntu.com

Once a share is created, the image type and size are automatically determined, and this information is reflected in the share status.

Images can be downloaded from various sources, such as HTTP servers where image files are located or container registries. It is also possible to download images directly from the command line using the curl utility.

Images can be created from other images and virtual machine disks.

A full description of the ClusterVirtualImage resource configuration parameters can be found at [link](cr.html#clustervirtualimage).

Translated with DeepL.com (free version)

### Creating an image from an HTTP server

Consider creating a cluster image

Run the following command to create a `ClusterVirtualImage`:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: ubuntu-22.04
spec:
  # A source for image creation.
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img"
EOF
```

Check the result of the `ClusterVirtualImage` creation:

```bash
d8 k get clustervirtualimage ubuntu-22.04
# or shorter
d8 k get cvi ubuntu-22.04

# NAME           PHASE   CDROM   PROGRESS   AGE
# ubuntu-22.04   Ready   false   100%       23h
```

After creation, the `ClusterVirtualImage` resource can be in the following states (phases):

- `Pending` - waiting for all dependent resources required for image creation to be ready.
- `WaitForUserUpload` - waiting for the user to upload the image (this phase is present only for `type=Upload`).
- `Provisioning` - the image creation process is in progress.
- `Ready` - the image is created and ready for use.
- `Failed` - an error occurred during the image creation process.
- `Terminating` - the image is being deleted. The image may “hang” in this state if it is still connected to the virtual machine.

As long as the image has not entered the `Ready` phase, the contents of the `.spec` block can be changed. If you change it, the disk creation process will start again. After entering the `Ready` phase, the contents of the `.spec` block cannot be changed!

You can trace the image creation process by adding the `-w` key to the previous command:

```bash
d8 k get cvi ubuntu-22.04 -w

# NAME           PHASE          CDROM   PROGRESS   AGE
# ubuntu-22.04   Provisioning   false              4s
# ubuntu-22.04   Provisioning   false   0.0%       4s
# ubuntu-22.04   Provisioning   false   28.2%      6s
# ubuntu-22.04   Provisioning   false   66.5%      8s
# ubuntu-22.04   Provisioning   false   100.0%     10s
# ubuntu-22.04   Provisioning   false   100.0%     16s
# ubuntu-22.04   Ready          false   100%       18s
```

The `ClusterVirtualImage` resource description provides additional information about the downloaded image:

```bash
d8 k describe cvi ubuntu-22.04
```

### Creating an image from Container Registry

An image stored in Container Registry has a certain format. Let's look at an example:

First, download the image locally:

```bash
curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
```

Next, create a `Dockerfile` with the following contents:

```Dockerfile
FROM scratch
COPY ubuntu2204.img /disk/ubuntu2204.img
```

Build the image and load it into the container registry. The example below uses docker.io as the container registry. you need to have a service account and a customized environment to run it.

```bash
docker build -t docker.io/<username>/ubuntu2204:latest
```

where `username` is the username specified when registering with docker.io.

Load the created image into the container registry:

```bash
docker push docker.io/<username>/ubuntu2204:latest
```

To use this image, create a resource as an example:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: ubuntu-2204
spec:
  dataSource:
    type: ContainerImage
    containerImage:
      image: docker.io/<username>/ubuntu2204:latest
EOF
```

### Load the image from the command line

To load an image from the command line, first create the following resource as shown below with the `ClusterVirtualImage` example:

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: some-image
spec:
  dataSource:
    type: Upload
EOF
```

Once created, the resource will enter the `WaitForUserUpload` phase, which means it is ready for image upload.

There are two options available for uploading from a cluster node and from an arbitrary node outside the cluster:

```bash
d8 k get cvi some-image -o jsonpath="{.status.imageUploadURLs}"  | jq

# {
#   "external":"https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm",
#   "inCluster":"http://10.222.165.239/upload"
# }
```

As an example, download the Cirros image:

```bash
curl -L http://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img -o cirros.img
```

Upload the image using the following command:

```bash
curl https://virtualization.example.com/upload/g2OuLgRhdAWqlJsCMyNvcdt4o5ERIwmm --progress-bar -T cirros.img | cat
```

After the upload is complete, the image should be created and enter the `Ready` phase:

```bash
d8 k get cvi some-image
# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       1m
```

## Virtual Machine Classes

The `VirtualMachineClass` resource is designed for centralized configuration of preferred virtual machine settings. It allows you to define CPU instructions and configuration policies for CPU and memory resources for virtual machines, as well as define ratios of these resources. In addition, `VirtualMachineClass` provides management of virtual machine placement across platform nodes. This allows administrators to effectively manage virtualization platform resources and optimally place virtual machines on platform nodes.

The structure of the `VirtualMachineClass` resource is as follows:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: <vmclass-name>
spec:
  # The block describes the virtual processor parameters for virtual machines.
  # This block cannot be changed after the resource has been created.
  cpu: ...
  # Describes the rules for node placement of virtual machines.
  # When changed, it is automatically applied to all virtual machines using this VirtualMachineClass.
  nodeSelector: ...
  # Describes the sizing policy for configuring virtual machine resources.
  # When changed, it is automatically applied to all virtual machines using this VirtualMachineClass.
  sizingPolicies: ...
```

{{< alert level="warning" >}}
Warning. Since changing the `.spec.nodeSelector` parameter affects all virtual machines using this `VirtualMachineClass`, the following should be considered:

For Enterprise-edition: this may cause virtual machines to be migrated to new destination nodes if the current nodes do not meet placement requirements.
For Community edition: this may cause virtual machines to restart according to the automatic change application policy set in the `.spec.disruptions.restartApprovalMode` parameter.
{{< /alert >}}

The virtualization platform provides 3 predefined `VirtualMachineClass` resources:

```bash
d8 k get virtualmachineclass
NAME               PHASE   AGE
host               Ready   6d1h
host-passthrough   Ready   6d1h
generic            Ready   6d1h
```

- `host` - this class uses a virtual CPU that is as close as possible to the platform node's CPU in terms of instruction set. This provides high performance and functionality, as well as compatibility with live migration for nodes with similar processor types. For example, VM migration between nodes with Intel and AMD processors will not work. This is also true for different generations of processors, as their instruction set is different.
- `host-passthrough` - uses the physical CPU of the platform node directly without any modifications. When using this class, the guest VM can only be migrated to a target node that has a CPU that exactly matches the CPU of the source node.
- `generic` is a universal CPU model that uses a fairly old, but supported by most modern CPUs, Nehalem model. This allows VMs to be run on any nodes in the cluster with live migration capability.

`VirtualMachineClass` is mandatory to be specified in the virtual machine configuration, an example of how to specify a class in the VM specification:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
spec:
  virtualMachineClassName: generic # the name of VirtualMachineClass
  ...
```

{{< alert level="info" >}}
Warning. It is recommended to create at least one `VirtualMachineClass` resource in the cluster with the Discovery type immediately after all nodes are configured and added to the cluster. This will allow the virtual machines to utilize a generic CPU with the highest possible CPU performance given the CPUs on the cluster nodes, allowing the virtual machines to utilize the maximum CPU capabilities and migrate seamlessly between cluster nodes if necessary.
{{< /alert >}}

Platform administrators can create the required classes of virtual machines according to their needs, but it is recommended to create the minimum required. Consider the following example:

### VirtualMachineClass configuration example

![](./images/vmclass-examples.png)

Let's imagine that we have a cluster of four nodes. Two of these nodes labeled `group=blue` have a “CPU X” processor with three instruction sets, and the other two nodes labeled `group=green` have a newer “CPU Y” processor with four instruction sets.

To optimally utilize the resources of this cluster, it is recommended to create three additional virtual machine classes (VirtualMachineClass):

- **universal**: This class will allow virtual machines to run on all nodes in the platform and migrate between them. It will use the instruction set for the lowest CPU model to ensure the greatest compatibility.
- **cpuX**: This class will be for virtual machines that should only run on nodes with a “CPU X” processor. VMs will be able to migrate between these nodes using the available “CPU X” instruction sets.
- **cpuY**: This class is for VMs that should only run on nodes with a “CPU Y” processor. VMs will be able to migrate between these nodes using the available “CPU Y” instruction sets.

> CPU instruction sets are a list of all the instructions that a processor can execute, such as addition, subtraction, or memory operations. They determine what operations are possible, affect program compatibility and performance, and can change from one generation of processors to the next.

Sample resource configurations for a given cluster:

```yaml
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: universal
spec:
  cpu:
    discovery: {}
    type: Discovery
  sizingPolicies: { ... }
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: cpuX
spec:
  cpu:
    discovery: {}
    type: Discovery
  nodeSelector:
    matchExpressions:
      - key: group
        operator: In
        values: ["blue"]
  sizingPolicies: { ... }
---
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: cpuY
spec:
  cpu:
    discovery:
      nodeSelector:
        matchExpressions:
          - key: group
            operator: In
            values: ["green"]
    type: Discovery
  sizingPolicies: { ... }
```

### Other configuration options

Example of the `VirtualMachineClass` resource configuration:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineClass
metadata:
  name: discovery
spec:
  cpu:
    # configure a generic vCPU for a given set of nodes
    discovery:
      nodeSelector:
        matchExpressions:
          - key: node-role.kubernetes.io/control-plane
            operator: DoesNotExist
    type: Discovery
  # allow VMs with this class to run only on nodes in the worker group
  nodeSelector:
    matchExpressions:
      - key: node.deckhouse.io/group
        operator: In
        values:
          - worker
  # resource configuration policy
  sizingPolicies:
    # for a range of 1 to 4 cores, it is possible to use 1 to 8GB of RAM in 512Mi increments
    # i.e. 1GB, 1.5GB, 2GB, 2.5GB etc.
    # no dedicated cores allowed
    # and all corefraction options are available
    - cores:
        min: 1
        max: 4
      memory:
        min: 1Gi
        max: 8Gi
        step: 512Mi
      dedicatedCores: [false]
      coreFractions: [5, 10, 20, 50, 100]
    # for a range of 5 to 8 cores, it is possible to use 5 to 16GB of RAM in 1GB increments
    # i.e. 5GB, 6GB, 7GB, etc.
    # it is not allowed to use dedicated cores
    # and some corefraction options are available
    - cores:
        min: 5
        max: 8
      memory:
        min: 5Gi
        max: 16Gi
        step: 1Gi
      dedicatedCores: [false]
      coreFractions: [20, 50, 100]
    # for a range of 9 to 16 cores, it is possible to use 9 to 32GB of RAM in 1GB increments
    # it is possible to use dedicated cores (or not)
    # and some variants of the corefraction parameter are available
    - cores:
        min: 9
        max: 16
      memory:
        min: 9Gi
        max: 32Gi
        step: 1Gi
      dedicatedCores: [true, false]
      coreFractions: [50, 100]
    # for the range from 17 to 1024 cores it is possible to use from 1 to 2 GB of RAM per core
    # only dedicated cores are available for use
    # and the only parameter corefraction = 100%
    - cores:
        min: 17
        max: 1024
      memory:
        perCore:
          min: 1Gi
          max: 2Gi
      dedicatedCores: [true]
      coreFractions: [100]
```

The following are fragments of `VirtualMachineClass` configurations for different tasks:

- a class with a vCPU with the required set of processor instructions, for this we use `type: Features` to specify the required set of supported instructions for the processor:

```yaml
spec:
cpu:
features: - vmx
type: Features

```

- class c universal vCPU for a given set of nodes, for this we use `type: Discovery`:

```yaml
spec:
  cpu:
    discovery:
      nodeSelector:
        matchExpressions:
          - key: node-role.kubernetes.io/control-plane
            operator: DoesNotExist
    type: Discovery
```

- to create a vCPU of a specific CPU with a pre-defined instruction set, we use `type: Model`. In advance, to get a list of supported CPU names for the cluster node, run the command:

```bash
d8 k get nodes <node-name> -o json | jq '.metadata.labels | to_entries[] | select(.key | test(“cpu-model”)) | .key | split(“/”)[1]'' -r

# Sample output:
#
# IvyBridge
# Nehalem
# Opteron_G1
# Penryn
# SandyBridge
# Westmere
```

then specify `VirtualMachineClass` in the resource specification:

```yaml
spec:
  cpu:
    model: IvyBridge
    type: Model
```

## Reliability mechanisms

### Migration / Maintenance Mode

Virtual machine migration is an important feature in virtualized infrastructure management. It allows you to move running virtual machines from one physical host to another without shutting them down. Virtual machine migration is required for a number of tasks and scenarios:

- Load balancing: Moving virtual machines between nodes allows you to evenly distribute the load on servers, ensuring that resources are utilized in the best possible way.
- Node maintenance: Virtual machines can be moved from nodes that need to be taken out of service to perform routine maintenance or software upgrades.
- Upgrading Virtual Machine Firmware: Migration allows you to upgrade the “firmware” of virtual machines without interrupting their operation.

#### Start migration of an arbitrary machine

The following is an example of migrating a selected virtual machine:

Before starting the migration, see the current status of the virtual machine:

```bash
d8 k get vm
# NAME                                   PHASE     NODE           IPADDRESS     AGE
# linux-vm                              Running   virtlab-pt-1   10.66.10.14   79m
```

We can see that it is currently running on the `virtlab-pt-1` node.

To migrate a virtual machine from one host to another, taking into account the virtual machine placement requirements, the `VirtualMachineOperations` (`vmop`) resource with the `Evict` type is used.

```yaml
d8 k apply -f - <<EOF
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: evict-linux-vm-$(date +%s)
spec:
  # virtual machine name
  virtualMachineName: linux-vm
  # operation for migration
  type: Evict
EOF
```

Immediately after creating the `vmop` resource, run the command:

```bash
d8 k get vm -w
# NAME                                   PHASE       NODE           IPADDRESS     AGE
# linux-vm                              Running     virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Migrating   virtlab-pt-1   10.66.10.14   79m
# linux-vm                              Running     virtlab-pt-2   10.66.10.14   79m
```

#### Maintenance Mode

When performing work on nodes with running virtual machines, there is a risk of disrupting their performance. To avoid this, you can put the node into maintenance mode and migrate virtual machines to other free nodes.

To do this, run the following command:

```bash
d8 k drain <nodename> --ignore-daemonsets --delete-emptydir-dat
```

where `<nodename>` is the node on which the work is to be performed and which should be freed from all resources (including system resources).

If there is a need to push only virtual machines off the node, run the following command:

```bash
d8 k drain <nodename> --pod-selector vm.kubevirt.internal.virtualization.deckhouse.io/name --delete-emptydir-data
```

After running the `d8 k drain` command, the node will go into maintenance mode and no virtual machines will be able to start on it. To take it out of maintenance mode, run the following command:

```bash
d8 k uncordon <nodename>
```

![](./images/drain.png)

### ColdStandby

ColdStandby provides a mechanism to recover a virtual machine from a failure on the host on which it was running.

The following requirements must be met for this mechanism to work:

- The virtual machine startup policy (`.spec.runPolicy`) must be one of: `AlwaysOnUnlessStoppedManually`, `AlwaysOn`.
- On hosts running virtual machines, the [fencing](https://deckhouse.io/products/kubernetes-platform/documentation/v1/modules/040-node-manager/cr.html#nodegroup-v1-spec-fencing-mode) mechanism must be enabled.

Let's see how it works on the example:

- A cluster consists of three nodes master, workerA and workerB. The worker nodes have the Fencing mechanism enabled.
- The `linux-vm` virtual machine is running on the workerA node.
- A problem occurs on workerA node (power outage, network outage, etc.)
- The controller checks node availability and finds that workerA is unavailable.
- The controller removes node `workerA` from the cluster.
- The `linux-vm` virtual machine is started on another suitable node (workerB).

![](./images/coldstandby.png)

## Disk and image storage settings

For storing disks (`VirtualDisk`) and images (`VirtualImage`) with the `PersistentVolumeClaim` type, platform-provided storage is used.

The list of storage supported by the platform can be listed by executing the command to view storage classes (`StorageClass`)

```bash
d8 k get storageclass
```

Example of command execution:

```bash
# NAME                                       PROVISIONER                           RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
# ceph-pool-r2-csi-rbd                       rbd.csi.ceph.com                      Delete          WaitForFirstConsumer   true                   49d
# ceph-pool-r2-csi-rbd-immediate (default)   rbd.csi.ceph.com                      Delete          Immediate              true                   49d
# linstor-thin-r1                            replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   28d
# linstor-thin-r2                            replicated.csi.storage.deckhouse.io   Delete          WaitForFirstConsumer   true                   78d
# nfs-4-1-wffc                               nfs.csi.k8s.io                        Delete          WaitForFirstConsumer   true                   49d
```

The `(default)` marker next to the class name indicates that this `StorageClass` will be used by default, in case the user has not specified the class name explicitly in the resource being created.

If the default `StorageClass` is not present in the cluster, the user must specify the required `StorageClass` explicitly in the resource specification.

The virtualization module also allows you to specify individual settings for disk and image storage.

### Storage class settings for images

The storage class settings for images are defined in the `.spec.settings.virtualImages` parameter of the module settings.

Example:

```yaml.
spec:
  ...
  settings:
    virtualImages:
       allowedStorageClassNames:
       - sc-1
       - sc-2
       defaultStorageClassName: sc-1
```

`allowedStorageClassNames` - (optional) is a list of allowed `StorageClass` for creating a `VirtualImage` that can be explicitly specified in the resource specification.
`defaultStorageClassName` - (optional) is the `StorageClass` used by default when creating a `VirtualImage` if the `.spec.persistentVolumeClaim.storageClassName` parameter is not specified.

### Storage class settings for disks

The storage class settings for disks are defined in the `.spec.settings.virtualDisks` parameter of the module settings.

Example:

```yaml.
spec:
  ...
  settings:
    virtualDisks:
       allowedStorageClassNames:
       - sc-1
       - sc-2
       defaultStorageClassName: sc-1
```

`allowedStorageClassNames` - (optional) is a list of allowed `StorageClass` for creating a `VirtualDisk` that can be explicitly specified in the resource specification.

`defaultStorageClassName` - (optional) is the `StorageClass` used by default when creating a `VirtualDisk` if the `.spec.persistentVolumeClaim.storageClassName` parameter is not specified.

### Fine-tune storage classes for disks

When you create a disk, the controller will automatically select the most optimal parameters supported by the storage based on what it knows.

Prioritizes `PersistentVolumeClaim` parameter settings when creating a disk by automatically determining the characteristics of the storage:

- RWX + Block
- RWX + FileSystem
- RWO + Block
- RWO + FileSystem

If the storage is unknown and it is not possible to automatically characterize it, then RWO + FileSystem is used.
