## Requirements:
- IDE
- Go 1.20
- [task](https://taskfile.dev/) utility to run tasks
- [golangci-lint](https://golangci-lint.run/usage/install/) >=1.52.1 (earlier versions has poor performance with Go 1.20)
- docker and [k3d](https://k3d.io/) to start local cluster
- helm and kubectl

## Taskfile

Repo contains Taskfile.dist.yaml. You can define your own tasks in [Taskfile.yaml](https://taskfile.dev/usage/#supported-file-names).

## Test in local cluster

### Prepare

- Change dvcr settings in local/virtualization-controller/values.yaml
- Create secret with ghcr.io token:

```
kubectl create secret docker-registry ghcr-io-auth --docker-username=GITHUB_USERNAME --docker-password=GITHUB_TOKEN --docker-server=ghcr.io --dry-run=client -o yaml > local/virtualization-controller/templates/auth-secret.yaml
```

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

### Useful tasks

- `task dev:delete` — delete Helm release with virtualization-controller
- `task dev:down` — remove k3d cluster and registry
- `task dev:reset` — recreate k3d cluster and registry
- `task build:cache:reset` — recreate build cache for virtualization-controller (requires after significant changes in go.mod)
- `task kctl` — shortcut for `kubectl -n virtualization-controller`
- `task gen:api` — run k8s code-generator to generate DeepCopy methods
- `task lint` — run Go linters
- `task dev:update:crds` — apply all manifests from `api` directory
- `task dev:update:<short-name>` — apply CRD manifest from `api` directory. Short names are: cvmi, vmi, vmd, vmds, vm.

## CRDs

- Define YAML manifest "by hands" in `api` directory.
- Define Go structures in `api/<version>` directory.
- Run `task gen:api` to generate DeepCopy methods.
- Add new types into addKnownTypes method in the `api/<versin>/register.go` file.
- Use type in Watch calls during controller setup.

### Notes

`task dev:converge` will copy YAML manifests from the `api` directory into `local/virtualization-controller/crds` directory before installing Helm chart.

Helm only install new CRDs. It will not update CRDs on `helm update`, as it is dangerous to automate (see [Helm docs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#some-caveats-and-explanations)). After making changes of YAML manifests, use `task dev:update:crds` to apply all manifests in the `api` directory or use CRD short name to update individual CRD, e.g. run `task dev:update:cvmi` to update `customresourcedefinition.apiextensions.k8s.io/ClusterVirtualMachineImage`.
