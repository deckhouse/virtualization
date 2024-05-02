## Requirements:
- IDE
- Go 1.19
- [task](https://taskfile.dev/) utility to run tasks
- [golangci-lint](https://golangci-lint.run/usage/install/) >=1.52.1 (earlier versions has poor performance with Go 1.20)
- docker and [k3d](https://k3d.io/) to start local cluster
- helm and kubectl
- kubeconfig switcher, e.g. [kubeswitch](https://github.com/danielfoehrKn/kubeswitch)

### Configure dev deckhouse registry

Login to `dev-registry.deckhouse.io` registry to pull virtualization-importer and test OS images:

```
docker login dev-registry.deckhouse.io
```

## Test in local cluster

### Run

0. Bootstrap local cluster:

    ```
    task dev:up
    ```

1. Build and run with new changes:

    ```
    task dev:converge
    ```

2. Run infinite logs watcher (restart this task after each build and run cycle):

    ```
    task dev:logs
    ```

3. Tasks to test CVI/VI

Create ClusterVirtualImage to import ubuntu ISO into DVCR:

    ```
    task cvi:recreate
    ```

Watch CVI resource:
    ```
    task cvi:watch
    ```


### Local cluster components

- Cluster registry `k3d-registry.virtualization-controller.test:5000`. It is used to deliver dev images of virtualization-controller, importer and uploader.
- Kubevirt and CDI. Kubevirt not starting VMs on MacOS.
- Caddy. A local server to test HTTP data sources.
- DVCR. TLS and basic auth enabled container registry to store all images.

### Caddy
 
UI can be accessed with port-forwarding:

```
kubectl port-forward -n caddy service/caddy 8081:80
http://localhost:8081/caddy/images/
```

A local-path-provisioner is used for Caddy in local cluster to save disk space on local machine.
A full image is used for remote cluster to start Caddy without PVC on remote cluster.

### DVCR

A simple registry implementation is started to use as DVCR. It has TLS certificate and a basic authentication.

It uses local-path-provisioner for loca cluster and a linstor storage for remote cluster.

## Test in remote cluster

### Prepare

Setup local access to your remote cluster with ssh tunnel or by publishing Kubernetes API.

Set default context or use kubeswitch to switch to remote cluster context.

### Run

0. Install caddy and dvcr into remote cluster:

   ```
   task dev:up:remote
   ``` 

1. Build and run with new changes:

    ```
    task dev:converge
    ```

2. Run infinite logs watcher (restart this task after each 'build and run' cycle):

    ```
    task dev:logs
    ```

3. Run VM

Create ClusterVirtualImage with alpine ISO from Caddy, VirtualImage with 10Gb capacity and a VirtualMachine with 4 CPUs:

    ```
    kubectl create ns vm
    kubectl -n vm apply -f config/samples/linux_vm_from_iso_image.yaml
    ```

4. Status

Ensure everything is up after some time:

```
kubectl -n vm get virtualization
NAME                                                  PHASE
virtualmachine.virtualization.deckhouse.io/linux-vm   Running

NAME                                                        CAPACITY   PHASE   PROGRESS
virtualdisk.virtualization.deckhouse.io/linux-disk   10Gi       Ready

NAME                                                               CDROM   PHASE   PROGRESS
clustervirtualimage.virtualization.deckhouse.io/linux-iso   false   Ready
```

5. Access VM

Use virtctl to enter a VM:

```
virtctl console -n vm linux-vm
Successfully connected to linux-vm console. The escape sequence is ^]

Welcome to Alpine Linux 3.18
Kernel 6.1.34-0-lts on an x86_64 (/dev/ttyS0)

localhost login: root
Welcome to Alpine!

The Alpine Wiki contains a large amount of how-to guides and general
information about administrating Alpine systems.
See <https://wiki.alpinelinux.org/>.

You can setup the system with the command: setup-alpine

You may change this message by editing /etc/motd.

localhost:~# 
```

## Taskfile

Repo contains Taskfile.dist.yaml. You can define your own tasks in [Taskfile.my.yaml](https://taskfile.dev/usage/#supported-file-names) and use them with `my:` prefix, e.g. `task my:build`. Use Taskfile.my.yaml.example as inspiration.

### Useful tasks

- `task dev:delete` — delete Helm release with virtualization-controller
- `task dev:down` — remove k3d cluster and registry
- `task dev:reset` — recreate k3d cluster and registry
- `task build:cache:reset` — recreate build cache for virtualization-controller (requires after significant changes in go.mod)
- `task kctl` — shortcut for `kubectl -n virtualization-controller`
- `task task api:generate` — run k8s code-generator to generate DeepCopy methods
- `task lint` — run Go linters
- `task dev:update:crds` — apply all manifests from `api` directory
- `task dev:update:<short-name>` — apply CRD manifest from `api` directory. Short names are: cvmi, vmi, vmd, vmds, vm.


## CRDs

- Define YAML manifest "by hands" in `api` directory.
- Define Go structures in `api/<version>` directory.
- Run `task api:generate` to generate DeepCopy methods.
- Add new types into addKnownTypes method in the `api/<versin>/register.go` file.
- Use type in Watch calls during controller setup.

### Notes

`task dev:converge` will copy YAML manifests from the `api` directory into `local/virtualization-controller/crds` directory before installing Helm chart.

Helm only install new CRDs. It will not update CRDs on `helm update`, as it is dangerous to automate (see [Helm docs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#some-caveats-and-explanations)). After making changes of YAML manifests, use `task dev:update:crds` to apply all manifests in the `api` directory or use CRD short name to update individual CRD, e.g. run `task dev:update:cvmi` to update `customresourcedefinition.apiextensions.k8s.io/ClusterVirtualImage`.
