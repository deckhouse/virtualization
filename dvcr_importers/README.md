## Requirements:
- IDE
- Go 1.19
- [task](https://taskfile.dev/) utility to run tasks
- [golangci-lint](https://golangci-lint.run/usage/install/) >=1.52.1 (earlier versions has poor performance with Go 1.20)
- docker to push images to registry
- [k3d](https://k3d.io/) to push images to local k3d registry

## Push to local cluster

### Build and push

- `task importer:push` — build importer image and push it to local k3d registry
- `task uploader:push` — build uploader image and push it to local k3d registry

## Push to dev deckhouse registry

### Configure dev deckhouse registry

Login to `dev-registry.deckhouse.io` registry to push images:

```
docker login dev-registry.deckhouse.io
```

### Build and push

- `REGISTRY=dev-registry.deckhouse.io/virt task importer:push` — build importer image and push it to dev deckhouse registry
- `REGISTRY=dev-registry.deckhouse.io/virt task uploader:push` — build uploader image and push it to dev deckhouse registry

### Useful tasks

- `lint` — run Go linters
