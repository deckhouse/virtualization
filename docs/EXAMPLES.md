---
title: "Configuration examples"
---

## Quick start

Example of creating a virtual machine with Ubuntu 22.04.

Let's create namespace where we will create virtual machines:

```bash
kubectl create ns vms
```

Let's create a virtual machine disk from an external source:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: linux-disk
  namespace: vms
spec:
  persistentVolumeClaim:
    size: 10Gi
    storageClassName: local-path
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

After creating `VirtualMachineDiks` in the namespace vms, the pod `importer-*` will start, which will perform the download of the given image.

Let's look at the current status of the resource:

```bash
kubectl -n vms get virtualmachinedisk -o wide

# NAME            PHASE   CAPACITY    PROGRESS   TARGET PVC                                               AGE
# linux-disk      Ready   10Gi        100%       vmd-vmd-blank-001-10c7616b-ba9c-4531-9874-ebcb3a2d83ad   1m
```

Next, let's create a virtual machine from the following specification:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
  namespace: vms
  labels:
    vm: linux
spec:
  runPolicy: AlwaysOn # the virtual machine should always be on
  enableParavirtualization: true # use paravirtualization (virtio)
  osType: Generic
  bootloader: BIOS
  cpu:
    cores: 1
    coreFraction: 10% # request 10% of the CPU time of one core
  memory:
    size: 1Gi
  provisioning: # example cloud-init script to create a cloud user with cloud password
    type: UserData
    userData: |
      #cloud-config
      users:
      - name: cloud
        passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        chpasswd: { expire: False }
        lock_passwd: false
        ssh_authorized_keys:
          - ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQDTXjTmx3hq2EPDQHWSJN7By1VNFZ8colI5tEeZDBVYAe9Oxq4FZsKCb1aGIskDaiAHTxrbd2efoJTcPQLBSBM79dcELtqfKj9dtjy4S1W0mydvWb2oWLnvOaZX/H6pqjz8jrJAKXwXj2pWCOzXerwk9oSI4fCE7VbqsfT4bBfv27FN4/Vqa6iWiCc71oJopL9DldtuIYDVUgOZOa+t2J4hPCCSqEJK/r+ToHQbOWxbC5/OAufXDw2W1vkVeaZUur5xwwAxIb3wM3WoS3BbwNlDYg9UB2D8+EZgNz1CCCpSy1ELIn7q8RnrTp0+H8V9LoWHSgh3VCWeW8C/MnTW90IR
  blockDevices:
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: linux-disk
```

Let's check that the virtual machine is created and running:

```bash
kubectl -n default get virtualmachine

# NAME       PHASE     NODENAME   IPADDRESS    AGE
# linux-vm   Running   virtlab-1  10.66.10.1   5m
```

Let's connect to the virtual machine using the console (press `Ctrl+]` to exit the console):

```bash
./dvp-connect -n vms --vm linux-vm
```

Let's connect to the machine using VNC:

```bash
./dvp-connect -n vms --vm linux-vm -c vnc
```

After running the command, the default VNC client will start. An alternative way to connect is to use the `--proxy-only` parameter to forward the VNC port to a local machine:

# Images

`VirtualMachineImage` and `ClusterVirtualMachineImage` are intended to store virtual machine disk images or installation images in `iso` format to create and replicate virtual machine disks in the same way. When connected to a virtual machine, these images are read-only and the `iso` format installation image will be attached as a cdrom device.

The `VirtualMachineImage` resource is only available in the namespace in which it was created, while `ClusterVirtualMachineImage` is available for all namespaces within the cluster.

Depending on the configuration, the `VirtualMachineImage` resource can store data in `DVCR` or use platform-provided disk storage (PV). On the other hand, `ClusterVirtualMachineImage` stores data only in `DVCR`, providing a single access to all images for all namespaces in the cluster.

Let's look at the creation of these resources with examples:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineImage
metadata:
  name: ubuntu-img
  namespace: vms
spec:
  storage: ContainerRegistry
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

Let's see what happens:

```bash
kubectl -n vms get virtualmachineimage

# NAME         PHASE   CDROM   PROGRESS   AGE
# ubuntu-img   Ready   false   100%       10m
```

to store the image in disk storage provided by the platform, the `storage` settings will be as follows:

```yaml
spec:
  storage: Kubernetes
  persistentVolumeClaim:
    storageClassName: "your-storage-class-name"
```

where `your-storage-class-name` is the name of the storageClass to be used.

To view the list of available storage classes, run the following command:

```bash
kubectl get storageclass

# Пример вывода команды:
# NAME                          PROVISIONER              RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
# linstor-thin-r1               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
# linstor-thin-r2               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
# linstor-thin-r3               linstor.csi.linbit.com   Delete          WaitForFirstConsumer   true                   20d
```

The `ClusterVirtualMachineImage` resource is created similarly, but does not require the `storage` settings to be specified:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: ubuntu-img
spec:
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

Let's look at the status of `ClusterVirtualMachineImage`:

```bash
kubectl get clustervirtualmachineimage

# NAME         PHASE   CDROM   PROGRESS   AGE
# ubuntu-img   Ready   false   100%       11m
```

Images can be created from a variety of external sources, such as an HTTP server where the image files are hosted or a container registry where images are stored and available for download. It is also possible to download images directly from the command line using the curl utility. Let's take a closer look at each of these options.

### Create and use an image from the container registry

The first thing to do is to generate the image itself for storage in the container registry.

As an example, let's consider creating a docker image with the ubuntu 22.04 disk:

Load the image locally:

```bash
curl -L https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img -o ubuntu2204.img
```

Create a Dockerfile with the following contents:

```Dockerfile
FROM scratch
COPY ubuntu2204.img /disk/ubuntu2204.img
```

Let's build an image and push it into the container registry. We will use docker.io as container registry, for this you need to have a service account and a configured environment.

```bash
docker build -t docker.io/username/ubuntu2204:latest
```

where, `username` is your username specified when registering with docker.io

Upload the created image to the container registry:

```bash
docker push docker.io/username/ubuntu2204:latest
```

To use this image, let's create the `ClusterVirtualMachineImage` resource as an example:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: ubuntu-2204
spec:
  dataSource:
    type: ContainerImage
    containerImage:
      image: docker.io/username/ubuntu2204:latest
```

To look at a resource and its status, run the command:

```bash
kubectl get clustervirtalmachineimage
```

### Uploading an image from the command line

To upload an image from the command line, we first need to create the following resource, consider `ClusterVirtualMachineImage` as an example:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualMachineImage
metadata:
  name: some-image
spec:
  dataSource:
    type: Upload
```

Once the resource is created, let's look at its status:

```bash
kubectl get clustervirtualmachineimages some-image -o json | jq .status.uploadCommand -r

> uploadCommand: curl https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc
    -T example.iso
```

> It is worth noting that CVMI with the Upload type waits 15 minutes after the image is created for the upload to begin. After this timeout expires, the resource will enter the Failed state.

Let's download the Cirros image for an example and boot it:

```bash
curl -L http://download.cirros-cloud.net/0.5.1/cirros-0.5.1-x86_64-disk.img -o cirros.img
https://virtualization.example.com/upload/dSJSQW0fSOerjH5ziJo4PEWbnZ4q6ffc -T cirros.img
```

After the `curl` command completes, the image should be created.

You can verify that everything was successful by checking the status of the created image:

```bash
kubectl get clustervirtualmachineimages

# NAME         PHASE   CDROM   PROGRESS   AGE
# some-image   Ready   false   100%       10m
```

## Disks

Disks are used in virtual machines to write and store data. The storage provided by the platform is used to store disks.

To see the available options, run the command:

```bash
kubectl get storageclass
```

Let's look at the options of what disks we can create:

### Creating a blank disk

The first thing to note is that we can create empty disks!

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: vmd-blank
spec:
  persistentVolumeClaim:
    storageClassName: "your-storage-class-name"
    size: 100M
```

Once the disk is created, we can use it to connect to the virtual machine.

You can view the status of the created resource with the command:

```bash
kubectl get virtualmachinedisk

# NAME        PHASE  CAPACITY   AGE
# vmd-blank   Ready  100Mi      1m
```

### Creating a disk from an image

We can create disks using existing disk images as well as external sources like images.

When creating a disk resource, we can specify the desired size. If no size is specified, a disk will be created with the size corresponding to the original disk image stored in the `VirtualMachineImage` or `ClusterVirtualMachineImage` resource. If you want to create a larger disk, you must explicitly specify this.

As an example, we will use a previously created `ClusterVirtualMachineImage` named `ubuntu-2204`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: ubuntu-root
spec:
  persistentVolumeClaim:
    size: 10Gi
    storageClassName: "your-storage-class-name"
  dataSource:
    type: ClusterVirtualMachineImage
    clusterVirtualMachineImage:
      name: ubuntu-img
```

### Changing disk size

Disks can be resized (only upwards for now) even if they are attached to a virtual machine, by editing the `spec.persistentVolumeClame.size` field:

```yaml
kubectl patch ubuntu-root --type merge -p '{"spec":{"persistentVolumeClaim":{"size":"11Gi"}}}'
```

### Connecting disks to running virtual machines

Disks can be attached "live" to an already running virtual machine by using the `VirtualMachineBlockDeviceAttachment` resource, for example:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineBlockDeviceAttachment
metadata:
  name: vmd-blank-attachment
spec:
  virtualMachineName: linux-vm # имя виртуальной машины, к которой будет подключен диск
  blockDevice:
    type: VirtualMachineDisk
    virtualMachineDisk:
      name: vmd-blank # имя подключаемого диска
```

If you change the machine name in this resource to another machine name, the disk will be reconnected from one virtual machine to another.

If you delete the `VirtualMachineBlockDeviceAttachment` resource - the disk will be disconnected from the virtual machine.

To see the list of live connected disks, run the command:

```bash
kubectl get virtualmachineblockdeviceattachments
```

## Virtual Machines

So, now we have disks and images, let's move on to the most important thing - creating a virtual machine.

To create a virtual machine, the `VirtualMachine` resource is used, its parameters allow you to configure:

- the resources required for the virtual machine (processor, memory, disks and images);
- rules of virtual machine placement on cluster nodes;
- boot loader settings and optimal parameters for the guest OS;
- virtual machine startup policy and policy for applying changes;
- initial configuration scenarios (cloud-init).

Let's create a virtual machine and configure it step by step:

### 0. Creating a disk for the virtual machine

The first thing we need to do before creating a virtual machine resource is to create a disk with the installed OS.

Let's create a disk for the virtual machine:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineDisk
metadata:
  name: ubuntu-2204-root
spec:
  persistentVolumeClaim:
    size: 10Gi
  dataSource:
    type: HTTP
    http:
      url: "https://cloud-images.ubuntu.com/minimal/releases/jammy/release-20230615/ubuntu-22.04-minimal-cloudimg-amd64.img"
```

### 1. Creating a virtual machine

Below is an example of a simple virtual machine configuration running Ubuntu 22.04. The example uses the cloud-init script, which installs the nginx package and creates the user `cloud`, with the password `cloud`:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: linux-vm
  namespace: default
  labels:
    vm: linux
spec:
  runPolicy: AlwaysOn
  provisioning:
    type: UserData
    userData: |
      #cloud-config
      package_update: true
      packages:
        - nginx
      run_cmd:
        - systemctl daemon-relaod
        - systemctl enable --now nginx
      users:
      - name: cloud
        # password: cloud
        passwd: $6$rounds=4096$vln/.aPHBOI7BMYR$bBMkqQvuGs5Gyd/1H5DP4m9HjQSy.kgrxpaGEHwkX7KEFV8BS.HZWPitAtZ2Vd8ZqIZRqmlykRCagTgPejt1i.
        shell: /bin/bash
        sudo: ALL=(ALL) NOPASSWD:ALL
        chpasswd: { expire: False }
        lock_passwd: false
  cpu:
    cores: 1
  memory:
    size: 2Gi
  blockDevices:
    # the order of disks and images in this block determines the boot priority
    - type: VirtualMachineDisk
      virtualMachineDisk:
        name: ubuntu-2204-root
```

If there is some private data, the initial initial initialization script of the virtual machine can be created in secret.

Example of a secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: linux-vm-cloud-init
  namespace: default
data:
  userData: # here cloud-init config in base64
type: Opaque
```

What it would look like in a virtual machine specification:

```yaml
spec:
  provisioning:
    type: UserDataSecret
    userDataSercertRef:
      name: linux-vm-cloud-init
```

Let's create the virtual machine from the manifest above.

After startup, the virtual machine must be in `Ready` status.

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME      IPADDRESS     AGE
# linux-vm   Running   node-name-x   10.66.10.1    5m
```

After creation, the virtual machine will automatically obtain an IP address from the range specified in the module settings (`vmCIDRs` block).

If we want to bind a specific IP address for the machine before it is started, the following steps must be performed:

1. Create a `VirtualMachineIPAddressClaim` resource in which to bind the desired ip address of the virtual machine:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineIPAddressClaim
metadata:
  name: <claim-name>
  namespace: <namespace>
spec:
  address: "W.X.Y.Z"
```

2. Commit the changes to the virtual machine specification accordingly:

```yaml
spec:
  virtualMachineIPAddressClaimName: <claim-name>
```

### 2. Configuring virtual machine placement rules

Let's assume that we need the virtual machine to run on a given set of nodes, for example on the `system` node group, the following configuration fragment will help us to do this:

```yaml
spec:
  tolerations:
    - key: "node-role.kubernetes.io/system"
      operator: Exists
      effect: NoSchedule
  nodeSelector:
    node-role.kubernetes.io/system: ""
```

Make changes to the previously created virtual machine specification.

### 3. Customize how the changes are applied

After making changes to the machine configuration, nothing will happen because the `Manual` change application policy is applied by default, which means that the changes need to be validated.

How can we figure this out?

Let's look at the status of the VM:

```bash
kubectl get linux-vm -o jsonpath='{.status}'
```

In the `.status.pendingChanges` field, we will see the changes that need to be applied. In the `.status.message` field, we will see a message that a restart of the virtual machine is required to apply the required changes.

Let's create and apply the following resource, which is responsible for the declarative way of managing the state of the virtual machine:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachineOperation
metadata:
  name: restart
spec:
  virtualMachineName: linux-vm
  type: Restart
EOF
```

Let's look at the status of the resource that has been created:

```bash
kubectl get vmops restart

# NAME       PHASE       VMNAME     AGE
# restart    Completed   linux-vm   1m
```

Once it goes to the `Completed` state, the virtual machine reboot is complete and the new virtual machine configuration settings are applied.

What if we want the changes required to reboot the virtual machine to be applied automatically? To do this, we need to configure the change application policy as follows:

```yaml
spec:
  disruptions:
    approvalMode: Automatic
```

### 4. Virtual Machine Startup Policy

Let's connect to the virtual machine using the serial console:

```bash
./dvp-connect -n default --vm linux-vm
```

terminate the virtual machine:

```bash
cloud@linux-vm$ sudo poweroff
```

then look at the status of the virtual machine

```bash
kubectl get virtualmachine

# NAME       PHASE     NODENAME       IPADDRESS   AGE
# linux-vm   Running   node-name-x    10.66.10.1  5m
```

the virtual machine is up and running again! But why did this happen?

Unlike classic virtualization systems, to determine the state of a virtual machine, we use a run policy that defines the desired state of the virtual machine at any given time.

When creating the virtual machine, we specified the `runPolicy: AlwaysOn` parameter, which means that the virtual machine should be started even if for some reason it is shut down, restarted or crashed.

To shut down the machine, change the policy value to `AlwaysOff` and the virtual machine will be shut down correctly.
