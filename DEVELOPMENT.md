## Requirements:
- IDE
- Go 1.20
- [task](https://taskfile.dev/) utility to run tasks
- docker and [kind](https://kind.sigs.k8s.io/) to start local cluster
- helm and kubectl

## Taskfile

Repo contains Taskfile.dist.yaml. You can define your own tasks in [Taskfile.yaml](https://taskfile.dev/usage/#supported-file-names).

## Test in local cluster

Build and run:

```
task vmi:build vmi:run
```

`vmi:run` will copy YAML manifests from apis directory into crds directory before
installing Helm chart.

Delete release:

```
task vmi:delete
```

## CRDs

- Define YAML manifest "by hands".
- Define Go structured in apis/version directory.
- Run task `gen:apis` to generate DeepCopy methods.
- Add new types into addKnownTypes method in the register.go file.
- Use type in Watch calls during controller setup.

