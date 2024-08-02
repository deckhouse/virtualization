---
title: "FAQ"
---

## How to install an operating system in a virtual machine from an iso-image?

Let's consider installing an operating system in a virtual machine from an iso-image, using Windows OS installation as an example.

To install the OS we will need an iso-image of Windows OS. We need to download it and publish it on some http-service available from the cluster.

Let's create an empty disk for OS installation:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualDisk
metadata:
  name: win-disk
  namespace: default
spec:
  persistentVolumeClaim:
    size: 100Gi
    storageClassName: local-path
```

Let's create resources with iso-images of Windows OS and virtio drivers:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: win-11-iso
spec:
  dataSource:
    type: HTTP
    http:
      url: "http://example.com/win11.iso"
```

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: ClusterVirtualImage
metadata:
  name: win-virtio-iso
spec:
  dataSource:
    type: HTTP
    http:
      url: "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso"
```

Create a virtual machine:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: win-vm
  namespace: default
  labels:
    vm: win
spec:
  virtualMachineClassName: generic
  runPolicy: Manual
  osType: Windows
  bootloader: EFI
  cpu:
    cores: 6
    coreFraction: 50%
  memory:
    size: 8Gi
  enableParavirtualization: true
  blockDeviceRefs:
    - kind: ClusterVirtualImage
      name: win-11-iso
    - kind: ClusterVirtualImage
      name: win-virtio-iso
    - kind: VirtualDisk
      name: win-disk
```

Once the resource is created, the virtual machine will be started. You need to connect to it and use the graphical wizard to add the `virtio` drivers and perform the OS installation.

```bash
d8 v vnc -n default win-vm
```

After the installation is complete, shut down the virtual machine.

Next, modify the `VirtualMachine` resource and apply the changes:

```yaml
spec:
  # ...
  runPolicy: AlwaysON
  # ...
  blockDeviceRefs:
    # remove all ClusterVirtualImage resources with iso disks from this section
    - kind: VirtualDisk
      name: win-disk
```

## How to create a virtual image for container registry

The virtual machine disk image stored in the container registry must be created in a special way.

Example Dockerfile for creating an image:

```Dockerfile
FROM scratch
COPY image-name.img /disk/image-name.img
```

Next, you need to build the image and run it in the container registry:

```bash
docker build -t docker.io/username/image:latest

docker push docker.io/username/image:latest
```

## How to redirect traffic to a virtual machine

Since the virtual machine runs in a Kubernetes cluster, the forwarding of network traffic to it is done similarly to the forwarding of traffic to the pods.

To do this, you just need to create a service with the required settings.

Suppose we have a virtual machine with http service published on port 80 and the following set of labels:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: web
  labels:
    vm: web
spec: ...
```

In order to direct network traffic to port 80 of the virtual machine - let's create a service:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: svc-1
spec:
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 80
  selector:
    app: old
```

We can change virtual machine label values on the fly, i.e. changing labels does not require restarting the virtual machine, which means that we can configure network traffic redirection from different services dynamically:

Let's imagine that we have created a new service and want to redirect traffic to our virtual machine from it:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: svc-2
spec:
  ports:
    - name: http
      port: 8080
      protocol: TCP
      targetPort: 80
  selector:
    app: new
```

By changing the labels on the virtual machine, we will redirect network traffic from the `svc-2` service to it

```yaml
metadata:
  labels:
    app: old
```

## How to provide windows answer file(Sysprep)

To provide Sysprep ability it's necessary to define in virtual machine with SysprepRef provisioning.
Set answer files (typically named unattend.xml or autounattend.xml) to secret to perform unattended installations of Windows.
You can also specify here other files in base64 format (customize.ps1, id_rsa.pub, ...) that you need to successfully execute scripts inside the answer file.

First, create sysprep secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: sysprep-config
data:
  unattend.xml: XXXx # base64 of answer file
```

Then create a virtual machine with unattended installation:

```yaml
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualMachine
metadata:
  name: win-vm
  namespace: default
  labels:
    vm: win
spec:
  virtualMachineClassName: generic
  provisioning:
    type: SysprepRef
    sysprepRef:
      kind: Secret
      name: sysprep-config
  runPolicy: AlwaysOn
  osType: Windows
  bootloader: EFI
  cpu:
    cores: 6
    coreFraction: 50%
  memory:
    size: 8Gi
  enableParavirtualization: true
  blockDeviceRefs:
    - kind: ClusterVirtualImage
      name: win-11-iso
    - kind: ClusterVirtualImage
      name: win-virtio-iso
    - kind: VirtualDisk
      name: win-disk
```

## Using virtualization in conjunction with the Admission-policy-engine module

Before starting, it is recommended to familiarize yourself with the settings of the [Admission-policy-engine](https://deckhouse.ru/documentation/v1/modules/015-admission-policy-engine/) module.  
When setting up security policies, it is recommended to follow the security policies that are installed in your company.

Let's look at the example of the enabled Baseline policy.  
Since Baseline does not allow the Pod of a virtual machine to run by default due to the elevated privileges required for the correct operation of the virtual machine, you will need to manually configure the namespaces in which they will run.

1. Exclusion of the namespace from the Baseline policy.  
```yaml
apiVersion: v1
kind: Namespace
metadata:
  labels:
    kubernetes.io/metadata.name: <namespace>
    security.deckhouse.io/pod-policy: privileged
  name: <namespace>
spec:
  finalizers:
  - kubernetes
```
2. Setting up a new security policy.  
This policy is based on Baseline and allows you to run virtual machines in a given namespace.
```yaml
---
apiVersion: deckhouse.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: virt-launcher-deny
spec:
  enforcementAction: Deny
  match:
    namespaceSelector:
      labelSelector:
        matchLabels:
          kubernetes.io/metadata.name: <namespace>
    labelSelector:
      matchLabels:
        kubevirt.internal.virtualization.deckhouse.io: virt-launcher
  policies:
    allowPrivilegeEscalation: true
    allowedCapabilities:
      - NET_BIND_SERVICE
      - SYS_NICE
    runAsUser:
      ranges:
        - max: 0
          min: 0
      rule: MustRunAs
---
apiVersion: deckhouse.io/v1alpha1
kind: SecurityPolicy
metadata:
  name: other-deny
spec:
  enforcementAction: Deny
  match:
    namespaceSelector:
      labelSelector:
        matchLabels:
          kubernetes.io/metadata.name: <namespace>
    labelSelector:
      matchExpressions:
        - key: kubevirt.internal.virtualization.deckhouse.io
          operator: NotIn
          values:
            - virt-launcher
  policies:
    allowedCapabilities:
      - AUDIT_WRITE
      - CHOWN
      - DAC_OVERRIDE
      - FOWNER
      - FSETID
      - KILL
      - MKNOD
      - NET_BIND_SERVICE
      - SETFCAP
      - SETGID
      - SETPCAP
      - SETUID
      - SYS_CHROOT
    allowedProcMount: Default
    seccompProfiles:
      allowedProfiles:
        - RuntimeDefault
        - Localhost
        - ""
        - undefined
      allowedLocalhostFiles:
        - '*'
    allowedUnsafeSysctls:
      - kernel.shm_rmid_forced
      - net.ipv4.ip_local_port_range
      - net.ipv4.ip_unprivileged_port_start
      - net.ipv4.tcp_syncookies
      - net.ipv4.ping_group_range
    allowHostNetwork: false
    allowPrivileged: false
    allowPrivilegeEscalation: false
    seLinux:
      - type: container_t
      - type: container_init_t
      - type: container_kvm_t
      - level: s0
    runAsUser:
      rule: MustRunAsNonRoot
```
