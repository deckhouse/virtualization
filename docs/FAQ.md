---
title: "FAQ"
weight: 70
---

## How to install an operating system in a virtual machine from an iso-image?

Let's consider installing an operating system in a virtual machine from an iso-image, using Windows OS installation as an example.

To install the OS we will need an iso-image of Windows OS. We need to download it and publish it on some http-service available from the cluster.

1. Let's create an empty disk for OS installation:

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

1. Let's create resources with iso-images of Windows OS and virtio drivers:

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

1. Create a virtual machine:

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

1. Once the resource is created, the virtual machine will be started. You need to connect to it and use the graphical wizard to add the `virtio` drivers and perform the OS installation.

    ```bash
    d8 v vnc -n default win-vm
    ```

1. After the installation is complete, shut down the virtual machine.

1. Next, modify the `VirtualMachine` resource and apply the changes:

    ```yaml
    spec:
      # ...
      runPolicy: AlwaysOn
      # ...
      blockDeviceRefs:
        # remove all ClusterVirtualImage resources with iso disks from this section
        - kind: VirtualDisk
          name: win-disk
    ```

1. After the changes, the virtual machine will start. To continue working with it, use the following command:

   ```bash
   d8 v vnc -n default win-vm
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
type: "provisioning.virtualization.deckhouse.io/sysprep"
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

## How to redirect traffic to a virtual machine

Since the virtual machine runs in a Kubernetes cluster, the forwarding of network traffic to it is done similarly to the forwarding of traffic to the pods.

To do this, you just need to create a service with the required settings.

1. Suppose we have a virtual machine with http service published on port 80 and the following set of labels:

    ```yaml
    apiVersion: virtualization.deckhouse.io/v1alpha2
    kind: VirtualMachine
    metadata:
      name: web
      labels:
        vm: web
    spec: ...
    ```

1. In order to direct network traffic to port 80 of the virtual machine - let's create a service:

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

## How to increase the DVCR size

To increase the disk size for DVCR, you must set a larger size in the `virtualization` module configuration than the current size.

1. Check the current dvcr size:

    ```shell
    kubectl get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}''
    #Output
    {“size”:“58G”,“storageClass”:“linstor-thick-data-r1”}
    ```

1. Set the size:

    ```shell
    kubectl patch mc virtualization \.
      --type merge -p '{“spec”: { “settings”: { “dvcr”: { “storage”: { “persistentVolumeClaim”: {“size”: “59G”}}}}}}''
    
    #Output
    moduleconfig.deckhouse.io/virtualization patched
    ```

1. Check the resizing:

    ```shell
    kubectl get mc virtualization -o jsonpath='{.spec.settings.dvcr.storage.persistentVolumeClaim}'
    #Output
    {“size”:“59G”,“storageClass”:“linstor-thick-data-r1”}
    
    kubectl get pvc dvcr -n d8-virtualization
    #Output
    NAME STATUS VOLUME                                    CAPACITY    ACCESS MODES   STORAGECLASS           AGE
    dvcr Bound  pvc-6a6cedb8-1292-4440-b789-5cc9d15bbc6b  57617188Ki  RWO            linstor-thick-data-r1  7d
    ```
