## Requirements:
- IDE
- Go 1.20
- [task](https://taskfile.dev/) utility to run tasks
- docker and [k3d](https://k3d.io/) or [kind](https://kind.sigs.k8s.io/) to start local cluster
- helm and kubectl

## Taskfile

Repo contains Taskfile.dist.yaml. You can define your own tasks in [Taskfile.yaml](https://taskfile.dev/usage/#supported-file-names).

## Test in local cluster

0. Bootstrap local cluster (or reset existing one):

    ```
    task dev:cluster:reset
    ```

1. Build and run cycle:

    ```
    task dev:vmi:build dev:vmi:run
    ```

    or simply:

    ```
    task dev:converge
    ```

    `dev:vmi:run` will copy YAML manifests from apis directory into crds directory before installing Helm chart.

2. Run infinite logs watcher (restart this task after each build and run cycle):

    ```
    task dev:vmi:logs
    ```

2. Delete release:

    ```
    task dev:vmi:delete
    ```

## CRDs

- Define YAML manifest "by hands".
- Define Go structured in apis/version directory.
- Run task `gen:apis` to generate DeepCopy methods.
- Add new types into addKnownTypes method in the register.go file.
- Use type in Watch calls during controller setup.

